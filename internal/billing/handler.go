package billing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"nexium.ai/api/pkg/adullam"
	"nexium.ai/api/pkg/apierr"
	mw "nexium.ai/api/pkg/middleware"
	"nexium.ai/api/pkg/response"
)

type Handler struct {
	svc           *Service
	webhookSecret string
	db            *pgxpool.Pool
}

func NewHandler(svc *Service, webhookSecret string, db *pgxpool.Pool) *Handler {
	return &Handler{svc: svc, webhookSecret: webhookSecret, db: db}
}

// AuthRoutes are mounted under the JWT-authenticated group.
func (h *Handler) AuthRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/plans", h.listPlans)
	r.Get("/subscription", h.getSubscription)
	r.Post("/checkout", h.checkout)
	r.Get("/payments", h.listPayments)
	r.Get("/payments/{paymentID}", h.getPaymentStatus)
	// Add-ons
	r.Get("/addons/packages", h.listAddonPackages)
	r.Post("/addons/checkout", h.addonCheckout)
	r.Get("/addons", h.listAddons)
	r.Get("/addons/{addonID}", h.getAddonStatus)
	return r
}

// ChannelRoutes are public — no auth required.
func (h *Handler) ChannelRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.listChannels)
	return r
}

func (h *Handler) listChannels(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(r.Context(),
		`SELECT id, name, slug, COALESCE(logo_url,''), is_active, COALESCE(maintenance_note,''), display_order
		 FROM payment_channels ORDER BY display_order ASC, name ASC`)
	if err != nil {
		response.Error(w, err)
		return
	}
	defer rows.Close()
	type Channel struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		Slug            string `json:"slug"`
		LogoURL         string `json:"logo_url"`
		IsActive        bool   `json:"is_active"`
		MaintenanceNote string `json:"maintenance_note,omitempty"`
		DisplayOrder    int    `json:"display_order"`
	}
	var list []Channel
	for rows.Next() {
		var c Channel
		if err := rows.Scan(&c.ID, &c.Name, &c.Slug, &c.LogoURL, &c.IsActive, &c.MaintenanceNote, &c.DisplayOrder); err != nil {
			response.Error(w, err)
			return
		}
		list = append(list, c)
	}
	if list == nil {
		list = []Channel{}
	}
	response.OK(w, list)
}

// WebhookRoutes are mounted without JWT auth — verified via HMAC signature.
func (h *Handler) WebhookRoutes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.webhook)
	return r
}

func (h *Handler) listPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := h.svc.ListPlans(r.Context())
	if err != nil {
		response.Error(w, err)
		return
	}
	if plans == nil {
		plans = []Plan{}
	}
	response.OK(w, plans)
}

func (h *Handler) getSubscription(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	sub, plan, err := h.svc.GetSubscription(r.Context(), userID)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, map[string]any{
		"subscription": sub,
		"plan":         plan,
		"is_free_plan": sub == nil,
	})
}

func (h *Handler) checkout(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	var req CheckoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid JSON"))
		return
	}
	payment, err := h.svc.InitiateCheckout(r.Context(), userID, req)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.Created(w, payment)
}

func (h *Handler) getPaymentStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	paymentID, err := uuid.Parse(chi.URLParam(r, "paymentID"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid payment id"))
		return
	}
	payment, err := h.svc.GetPaymentStatus(r.Context(), userID, paymentID)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, payment)
}

func (h *Handler) listPayments(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	payments, err := h.svc.ListPayments(r.Context(), userID)
	if err != nil {
		response.Error(w, err)
		return
	}
	if payments == nil {
		payments = []BillingPayment{}
	}
	response.OK(w, payments)
}

// webhook handles inbound events from Adullam.
// Adullam signs requests with HMAC-SHA256 of the raw body using the webhook secret.
// Header: X-Adullam-Signature: sha256=<hex>
func (h *Handler) webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if h.webhookSecret == "" || !h.verifySignature(body, r.Header.Get("X-Adullam-Signature")) {
		log.Printf("[billing] webhook: invalid signature")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var event struct {
		Event string          `json:"event"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if event.Event != "payment" {
		w.WriteHeader(http.StatusOK) // ignore non-payment events
		return
	}

	var payment adullam.Payment
	if err := json.Unmarshal(event.Data, &payment); err != nil {
		log.Printf("[billing] webhook: failed to decode payment data: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := h.svc.HandleWebhook(r.Context(), &payment); err != nil {
		log.Printf("[billing] webhook: handler error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// ── Add-on handlers ──────────────────────────────────────────────────────────

func (h *Handler) listAddonPackages(w http.ResponseWriter, r *http.Request) {
	response.OK(w, h.svc.GetAddonPackages())
}

func (h *Handler) addonCheckout(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	var req AddonCheckoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid JSON"))
		return
	}
	addon, err := h.svc.InitiateAddonCheckout(r.Context(), userID, req)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.Created(w, addon)
}

func (h *Handler) getAddonStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	addonID, err := uuid.Parse(chi.URLParam(r, "addonID"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid addon id"))
		return
	}
	addon, err := h.svc.GetAddonStatus(r.Context(), userID, addonID)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, addon)
}

func (h *Handler) listAddons(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	addons, err := h.svc.ListAddons(r.Context(), userID)
	if err != nil {
		response.Error(w, err)
		return
	}
	if addons == nil {
		addons = []StorageAddon{}
	}
	response.OK(w, addons)
}

func (h *Handler) verifySignature(body []byte, sigHeader string) bool {
	if !strings.HasPrefix(sigHeader, "sha256=") {
		return false
	}
	expected := strings.TrimPrefix(sigHeader, "sha256=")
	mac := hmac.New(sha256.New, []byte(h.webhookSecret))
	mac.Write(body)
	actual := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(actual), []byte(expected))
}
