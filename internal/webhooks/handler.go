package webhooks

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"nexium.ai/api/pkg/apierr"
	"nexium.ai/api/pkg/response"
)

type Handler struct{ svc Service }

func NewHandler(svc Service) *Handler { return &Handler{svc} }

// Routes mounts under /api/v1/projects/{projectID}/webhooks (JWT auth)
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Get("/{webhookID}", h.get)
	r.Patch("/{webhookID}", h.update)
	r.Delete("/{webhookID}", h.delete)
	r.Get("/{webhookID}/deliveries", h.listDeliveries)
	return r
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	projectID, _ := projectID(r)
	hooks, err := h.svc.List(r.Context(), projectID)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, hooks)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	projectID, _ := projectID(r)
	var req struct {
		URL    string   `json:"url"`
		Events []string `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		response.Error(w, apierr.ErrBadRequest("url and events are required"))
		return
	}
	hook, secret, err := h.svc.Create(r.Context(), projectID, req.URL, req.Events)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.Created(w, map[string]any{"webhook": hook, "secret": secret})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	projectID, _ := projectID(r)
	id, err := uuid.Parse(chi.URLParam(r, "webhookID"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid webhook id"))
		return
	}
	hook, err := h.svc.Get(r.Context(), id, projectID)
	if err != nil {
		response.Error(w, err)
		return
	}
	if hook == nil {
		response.Error(w, apierr.ErrNotFound)
		return
	}
	response.OK(w, hook)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	projectID, _ := projectID(r)
	id, err := uuid.Parse(chi.URLParam(r, "webhookID"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid webhook id"))
		return
	}
	var req struct {
		URL      string   `json:"url"`
		Events   []string `json:"events"`
		IsActive *bool    `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid JSON"))
		return
	}
	existing, err := h.svc.Get(r.Context(), id, projectID)
	if err != nil || existing == nil {
		response.Error(w, apierr.ErrNotFound)
		return
	}
	url := req.URL
	if url == "" {
		url = existing.URL
	}
	events := req.Events
	if len(events) == 0 {
		events = existing.Events
	}
	isActive := existing.IsActive
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	hook, err := h.svc.Update(r.Context(), id, projectID, url, events, isActive)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, hook)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	projectID, _ := projectID(r)
	id, err := uuid.Parse(chi.URLParam(r, "webhookID"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid webhook id"))
		return
	}
	if err := h.svc.Delete(r.Context(), id, projectID); err != nil {
		response.Error(w, err)
		return
	}
	response.NoContent(w)
}

func (h *Handler) listDeliveries(w http.ResponseWriter, r *http.Request) {
	projectID, _ := projectID(r)
	id, err := uuid.Parse(chi.URLParam(r, "webhookID"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid webhook id"))
		return
	}
	deliveries, err := h.svc.ListDeliveries(r.Context(), id, projectID)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, deliveries)
}

func projectID(r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "projectID"))
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}
