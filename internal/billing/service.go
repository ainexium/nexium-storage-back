package billing

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"nexium.ai/api/internal/mailer"
	"nexium.ai/api/pkg/adullam"
	"nexium.ai/api/pkg/apierr"
)

type SetQuotaFn    func(ctx context.Context, userID uuid.UUID, quotaBytes *int64) error
type GetUserInfoFn func(ctx context.Context, userID uuid.UUID) (name, email string, err error)

type Service struct {
	store       Store
	adullam     *adullam.Client
	setQuota    SetQuotaFn
	getUserInfo GetUserInfoFn
	mail        *mailer.Mailer
}

func NewService(store Store, client *adullam.Client, setQuota SetQuotaFn, getUserInfo GetUserInfoFn, mail *mailer.Mailer) *Service {
	return &Service{store: store, adullam: client, setQuota: setQuota, getUserInfo: getUserInfo, mail: mail}
}

func (s *Service) ListPlans(ctx context.Context) ([]Plan, error) {
	return s.store.ListPlans(ctx)
}

func (s *Service) GetSubscription(ctx context.Context, userID uuid.UUID) (sub *Subscription, plan *Plan, err error) {
	sub, err = s.store.GetSubscription(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	if sub != nil && sub.Status == "active" {
		// Real-time expiry: expire immediately if period ended without the daily job having run yet
		if sub.CurrentPeriodEnd != nil && time.Now().After(*sub.CurrentPeriodEnd) {
			if expErr := s.expireUser(ctx, userID); expErr != nil {
				log.Printf("[billing] expireUser error user=%s: %v", userID, expErr)
			}
			sub = nil
		} else {
			return sub, sub.Plan, nil
		}
	} else {
		sub = nil // treat expired/cancelled as no subscription
	}
	// No active subscription → free plan
	free, err := s.store.GetPlanBySlug(ctx, "free")
	if err != nil {
		return nil, nil, err
	}
	return nil, free, nil
}

// expireUser marque l'abonnement comme expiré et remet le quota au niveau Free.
// Les add-ons restent en base et se réactivent automatiquement au renouvellement.
func (s *Service) expireUser(ctx context.Context, userID uuid.UUID) error {
	if err := s.store.ExpireSubscription(ctx, userID); err != nil {
		return err
	}
	free, err := s.store.GetPlanBySlug(ctx, "free")
	if err != nil || free == nil {
		return err
	}
	return s.setQuota(ctx, userID, &free.StorageBytes)
}

func (s *Service) InitiateCheckout(ctx context.Context, userID uuid.UUID, req CheckoutRequest) (*BillingPayment, error) {
	planID, err := uuid.Parse(req.PlanID)
	if err != nil {
		return nil, apierr.ErrBadRequest("invalid plan_id")
	}
	plan, err := s.store.GetPlanByID(ctx, planID)
	if err != nil || plan == nil {
		return nil, apierr.ErrBadRequest("plan not found")
	}
	if plan.PriceXOF == 0 {
		return nil, apierr.ErrBadRequest("free plan requires no payment")
	}
	channelOK, err := s.store.IsActiveChannel(ctx, req.Channel)
	if err != nil {
		return nil, err
	}
	if !channelOK {
		return nil, apierr.ErrBadRequest("canal de paiement invalide ou indisponible")
	}
	if req.Phone == "" {
		return nil, apierr.ErrBadRequest("phone is required")
	}

	paymentID := uuid.New()
	p := &BillingPayment{
		ID:        paymentID,
		UserID:    userID,
		PlanID:    plan.ID,
		AmountXOF: plan.PriceXOF,
		Status:    "pending",
		Channel:   req.Channel,
		Phone:     req.Phone,
	}
	if err := s.store.CreatePayment(ctx, p); err != nil {
		return nil, fmt.Errorf("create payment record: %w", err)
	}

	desc := fmt.Sprintf("NEXIUM %s - 1 month", plan.Name)
	if len(desc) > 48 {
		desc = desc[:48]
	}

	remote, err := s.adullam.CreatePayment(ctx, paymentID.String(), adullam.CreatePaymentInput{
		Amount:        plan.PriceXOF,
		Currency:      "xof",
		CustomerPhone: req.Phone,
		Channel:       req.Channel,
		Description:   desc,
	})
	if err != nil {
		_ = s.store.UpdatePaymentStatus(ctx, paymentID, "failed", "")
		return nil, apierr.New(502, fmt.Sprintf("payment gateway error: %s", err.Error()))
	}

	if err := s.store.UpdatePaymentStatus(ctx, paymentID, remote.Status, remote.ID); err != nil {
		return nil, err
	}
	p.AdullamID = remote.ID
	p.Status = remote.Status
	p.RedirectURL = remote.RedirectURL
	p.Plan = plan
	if remote.RedirectURL != "" {
		if err := s.store.SetPaymentRedirectURL(ctx, paymentID, remote.RedirectURL); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// GetPaymentStatus syncs with Adullam (fallback polling) and activates subscription on completion.
func (s *Service) GetPaymentStatus(ctx context.Context, userID, paymentID uuid.UUID) (*BillingPayment, error) {
	p, err := s.store.GetPayment(ctx, paymentID)
	if err != nil {
		return nil, err
	}
	if p == nil || p.UserID != userID {
		return nil, apierr.ErrNotFound
	}

	// Terminal or no adullam id yet — return as-is
	if isTerminal(p.Status) || p.AdullamID == "" {
		return p, nil
	}

	remote, err := s.adullam.GetPayment(ctx, p.AdullamID)
	if err != nil {
		log.Printf("[billing] adullam poll error payment=%s: %v", p.ID, err)
		return p, nil // return cached status on transient error
	}

	if remote.Status != p.Status {
		if err := s.store.UpdatePaymentStatus(ctx, paymentID, remote.Status, remote.ID); err != nil {
			return nil, err
		}
		p.Status = remote.Status
	}

	if p.Status == "completed" {
		if err := s.activateSubscription(ctx, p.UserID, p.Plan); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// HandleWebhook processes an inbound Adullam payment event.
// Checks both billing_payments (subscriptions) and storage_addons.
func (s *Service) HandleWebhook(ctx context.Context, adullamPayment *adullam.Payment) error {
	if adullamPayment.Status != "completed" {
		return nil
	}

	// 1. Subscription payment?
	p, err := s.store.GetPaymentByAdullamID(ctx, adullamPayment.ID)
	if err != nil {
		return err
	}
	if p != nil {
		if p.Status == "completed" {
			return nil
		}
		if err := s.store.UpdatePaymentStatus(ctx, p.ID, "completed", adullamPayment.ID); err != nil {
			return err
		}
		return s.activateSubscription(ctx, p.UserID, p.Plan)
	}

	// 2. Storage add-on payment?
	addon, err := s.store.GetAddonByAdullamID(ctx, adullamPayment.ID)
	if err != nil {
		return err
	}
	if addon != nil {
		if addon.Status == "completed" {
			return nil
		}
		if err := s.store.UpdateAddonStatus(ctx, addon.ID, "completed", adullamPayment.ID); err != nil {
			return err
		}
		addon.Status = "completed"
		return s.activateAddon(ctx, addon)
	}

	log.Printf("[billing] webhook: unknown adullam payment %s", adullamPayment.ID)
	return nil
}

func (s *Service) ListPayments(ctx context.Context, userID uuid.UUID) ([]BillingPayment, error) {
	return s.store.ListUserPayments(ctx, userID)
}

// GetUserFileSizeLimit retourne la taille max par fichier selon le plan actif de l'utilisateur.
// Retourne le max_file_bytes du plan Free si aucune subscription active.
func (s *Service) GetUserFileSizeLimit(ctx context.Context, userID uuid.UUID) (int64, error) {
	sub, err := s.store.GetSubscription(ctx, userID)
	if err != nil {
		return 0, err
	}
	if sub != nil && sub.Status == "active" && sub.Plan != nil {
		return sub.Plan.MaxFileBytes, nil
	}
	free, err := s.store.GetPlanBySlug(ctx, "free")
	if err != nil || free == nil {
		return 104857600, nil // 100 MB fallback si plans pas encore seeded
	}
	return free.MaxFileBytes, nil
}

func (s *Service) activateSubscription(ctx context.Context, userID uuid.UUID, plan *Plan) error {
	periodEnd := time.Now().AddDate(0, 1, 0)
	if err := s.store.UpsertSubscription(ctx, userID, plan.ID, "active", &periodEnd); err != nil {
		return err
	}
	addonBytes, _ := s.store.SumCompletedAddonBytes(ctx, userID)
	total := plan.StorageBytes + addonBytes
	if err := s.setQuota(ctx, userID, &total); err != nil {
		return err
	}
	if s.mail != nil && s.getUserInfo != nil {
		go s.sendActivationEmail(userID, plan, periodEnd)
	}
	return nil
}

func (s *Service) sendActivationEmail(userID uuid.UUID, plan *Plan, periodEnd time.Time) {
	ctx := context.Background()
	name, email, err := s.getUserInfo(ctx, userID)
	if err != nil || email == "" {
		log.Printf("[billing] sendActivationEmail: getUserInfo error user=%s: %v", userID, err)
		return
	}
	d := receiptData{
		UserName:  name,
		UserEmail: email,
		Plan:      plan,
		PeriodEnd: periodEnd,
		PaymentAt: time.Now(),
	}
	html := confirmationEmailHTML(name, email, plan, periodEnd)
	pdf := generateReceiptPDF(d)
	filename := fmt.Sprintf("recu-nexium-%s.pdf", time.Now().Format("2006-01"))
	if err := s.mail.SendWithAttachment(ctx, email, name,
		fmt.Sprintf("Votre abonnement %s est actif — NEXIUM Storage", plan.Name),
		html, filename, "application/pdf", pdf,
	); err != nil {
		log.Printf("[billing] sendActivationEmail: send error user=%s: %v", userID, err)
	}
}

// ── Add-ons ──────────────────────────────────────────────────────────────────

func (s *Service) GetAddonPackages() []AddonPackage { return AddonPackages }

func (s *Service) InitiateAddonCheckout(ctx context.Context, userID uuid.UUID, req AddonCheckoutRequest) (*StorageAddon, error) {
	sub, err := s.store.GetSubscription(ctx, userID)
	if err != nil {
		return nil, err
	}
	if sub == nil || sub.Plan == nil || !sub.Plan.AddonsEnabled {
		return nil, apierr.ErrBadRequest("les add-ons ne sont pas disponibles sur votre plan actuel")
	}

	var pkg *AddonPackage
	for i, p := range AddonPackages {
		if p.ID == req.PackageID {
			pkg = &AddonPackages[i]
			break
		}
	}
	if pkg == nil {
		return nil, apierr.ErrBadRequest("package inconnu")
	}
	channelOK, err := s.store.IsActiveChannel(ctx, req.Channel)
	if err != nil {
		return nil, err
	}
	if !channelOK {
		return nil, apierr.ErrBadRequest("canal de paiement invalide ou indisponible")
	}
	if req.Phone == "" {
		return nil, apierr.ErrBadRequest("phone est requis")
	}

	addon := &StorageAddon{
		ID:        uuid.New(),
		UserID:    userID,
		PackageID: pkg.ID,
		Bytes:     pkg.Bytes,
		PriceXOF:  pkg.PriceXOF,
		Status:    "pending",
		Channel:   req.Channel,
		Phone:     req.Phone,
	}
	if err := s.store.CreateAddon(ctx, addon); err != nil {
		return nil, fmt.Errorf("create addon record: %w", err)
	}

	desc := fmt.Sprintf("NEXIUM Storage %s", pkg.Label)
	if len(desc) > 48 {
		desc = desc[:48]
	}
	remote, err := s.adullam.CreatePayment(ctx, addon.ID.String(), adullam.CreatePaymentInput{
		Amount:        pkg.PriceXOF,
		Currency:      "xof",
		CustomerPhone: req.Phone,
		Channel:       req.Channel,
		Description:   desc,
	})
	if err != nil {
		_ = s.store.UpdateAddonStatus(ctx, addon.ID, "failed", "")
		return nil, apierr.New(502, fmt.Sprintf("payment gateway error: %s", err.Error()))
	}

	_ = s.store.UpdateAddonStatus(ctx, addon.ID, remote.Status, remote.ID)
	addon.AdullamID = remote.ID
	addon.Status = remote.Status
	return addon, nil
}

func (s *Service) GetAddonStatus(ctx context.Context, userID, addonID uuid.UUID) (*StorageAddon, error) {
	a, err := s.store.GetAddon(ctx, addonID)
	if err != nil {
		return nil, err
	}
	if a == nil || a.UserID != userID {
		return nil, apierr.ErrNotFound
	}

	if isTerminal(a.Status) || a.AdullamID == "" {
		return a, nil
	}

	remote, err := s.adullam.GetPayment(ctx, a.AdullamID)
	if err != nil {
		log.Printf("[billing] addon poll error id=%s: %v", a.ID, err)
		return a, nil
	}

	if remote.Status != a.Status {
		_ = s.store.UpdateAddonStatus(ctx, addonID, remote.Status, remote.ID)
		a.Status = remote.Status
	}

	if a.Status == "completed" {
		if err := s.activateAddon(ctx, a); err != nil {
			return nil, err
		}
	}
	return a, nil
}

func (s *Service) ListAddons(ctx context.Context, userID uuid.UUID) ([]StorageAddon, error) {
	return s.store.ListUserAddons(ctx, userID)
}

func (s *Service) activateAddon(ctx context.Context, addon *StorageAddon) error {
	sub, err := s.store.GetSubscription(ctx, addon.UserID)
	if err != nil {
		return err
	}
	var baseBytes int64
	if sub != nil && sub.Plan != nil {
		baseBytes = sub.Plan.StorageBytes
	} else {
		if free, err := s.store.GetPlanBySlug(ctx, "free"); err == nil && free != nil {
			baseBytes = free.StorageBytes
		}
	}
	// SumCompletedAddonBytes includes the just-activated addon (already marked completed)
	addonTotal, err := s.store.SumCompletedAddonBytes(ctx, addon.UserID)
	if err != nil {
		return err
	}
	total := baseBytes + addonTotal
	return s.setQuota(ctx, addon.UserID, &total)
}

func isTerminal(status string) bool {
	switch status {
	case "completed", "failed", "expired":
		return true
	}
	return false
}
