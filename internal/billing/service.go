package billing

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"nexium.ai/api/internal/mailer"
	"nexium.ai/api/pkg/adullam"
	"nexium.ai/api/pkg/apierr"
	"nexium.ai/api/pkg/saspay"
)

type SetQuotaFn    func(ctx context.Context, userID uuid.UUID, quotaBytes *int64) error
type GetUserInfoFn func(ctx context.Context, userID uuid.UUID) (name, email string, err error)

type Service struct {
	store       Store
	adullam     *adullam.Client
	saspay      *saspay.Client
	setQuota    SetQuotaFn
	getUserInfo GetUserInfoFn
	mail        *mailer.Mailer
}

func NewService(store Store, adullamClient *adullam.Client, saspayClient *saspay.Client, setQuota SetQuotaFn, getUserInfo GetUserInfoFn, mail *mailer.Mailer) *Service {
	return &Service{
		store:       store,
		adullam:     adullamClient,
		saspay:      saspayClient,
		setQuota:    setQuota,
		getUserInfo: getUserInfo,
		mail:        mail,
	}
}

func (s *Service) ListCountries(ctx context.Context) ([]Country, error) {
	return s.store.ListActiveCountries(ctx)
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
		if sub.CurrentPeriodEnd != nil && time.Now().After(*sub.CurrentPeriodEnd) {
			if expErr := s.expireUser(ctx, userID); expErr != nil {
				log.Printf("[billing] expireUser error user=%s: %v", userID, expErr)
			}
			sub = nil
		} else {
			return sub, sub.Plan, nil
		}
	} else {
		sub = nil
	}
	free, err := s.store.GetPlanBySlug(ctx, "free")
	if err != nil {
		return nil, nil, err
	}
	return nil, free, nil
}

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
	if req.CountryCode == "" {
		req.CountryCode = "CI"
	}
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
	channelOK, provider, err := s.store.GetChannelInfo(ctx, req.Channel)
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
		ID:          paymentID,
		UserID:      userID,
		PlanID:      plan.ID,
		AmountXOF:   plan.PriceXOF,
		Status:      "pending",
		Channel:     req.Channel,
		Phone:       req.Phone,
		Provider:    provider,
		CountryCode: req.CountryCode,
	}
	if err := s.store.CreatePayment(ctx, p); err != nil {
		return nil, fmt.Errorf("create payment record: %w", err)
	}

	desc := fmt.Sprintf("NEXIUM %s - 1 month", plan.Name)
	if len(desc) > 48 {
		desc = desc[:48]
	}

	if provider == "adullam" {
		remote, err := s.adullam.CreatePayment(ctx, paymentID.String(), adullam.CreatePaymentInput{
			Amount:        plan.PriceXOF,
			Currency:      "xof",
			CustomerPhone: req.Phone,
			Channel:       adullamChannel(req.Channel),
			Description:   desc,
		})
		if err != nil {
			_ = s.store.UpdatePaymentStatus(ctx, paymentID, "failed", "")
			return nil, apierr.New(502, fmt.Sprintf("payment gateway error: %s", err.Error()))
		}
		_ = s.store.UpdatePaymentStatus(ctx, paymentID, remote.Status, remote.ID)
		p.AdullamID = remote.ID
		p.Status = remote.Status
		p.RedirectURL = remote.RedirectURL
		p.Plan = plan
		if remote.RedirectURL != "" {
			_ = s.store.SetPaymentRedirectURL(ctx, paymentID, remote.RedirectURL)
		}
	} else {
		// SasPay
		name, email, err := s.getUserInfo(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("getUserInfo: %w", err)
		}
		firstName, lastName := splitName(name)
		remote, err := s.saspay.CreatePayment(ctx, paymentID.String(), saspay.CreateSoftpayInput{
			Amount:      fmt.Sprintf("%.2f", float64(plan.PriceXOF)),
			Currency:    currencyForCountry(req.CountryCode),
			Country:     req.CountryCode,
			Customer:    saspay.Customer{Email: email, FirstName: firstName, LastName: lastName, Phone: req.Phone},
			Network:     req.Channel,
			Description: desc,
		})
		if err != nil {
			_ = s.store.UpdatePaymentStatus(ctx, paymentID, "failed", "")
			return nil, apierr.New(502, fmt.Sprintf("payment gateway error: %s", err.Error()))
		}
		normalised := saspay.NormaliseStatus(remote.Status)
		_ = s.store.UpdatePaymentStatus(ctx, paymentID, normalised, remote.ID)
		p.AdullamID = remote.ID
		p.Status = normalised
		p.RedirectURL = remote.CheckoutURL
		p.Plan = plan
		if remote.CheckoutURL != "" {
			_ = s.store.SetPaymentRedirectURL(ctx, paymentID, remote.CheckoutURL)
		}
	}
	return p, nil
}

func (s *Service) GetPaymentStatus(ctx context.Context, userID, paymentID uuid.UUID) (*BillingPayment, error) {
	p, err := s.store.GetPayment(ctx, paymentID)
	if err != nil {
		return nil, err
	}
	if p == nil || p.UserID != userID {
		return nil, apierr.ErrNotFound
	}
	if isTerminal(p.Status) || p.AdullamID == "" {
		return p, nil
	}

	if p.Provider == "saspay" {
		remote, err := s.saspay.GetPayment(ctx, p.AdullamID)
		if err != nil {
			log.Printf("[billing] saspay poll error payment=%s: %v", p.ID, err)
			return p, nil
		}
		normalised := saspay.NormaliseStatus(remote.Status)
		if normalised != p.Status {
			_ = s.store.UpdatePaymentStatus(ctx, paymentID, normalised, remote.ID)
			p.Status = normalised
		}
		if normalised == "failed" && remote.FailureReason != "" {
			p.FailureReason = remote.FailureReason
		}
		if p.Status == "completed" {
			if err := s.activateSubscription(ctx, p.UserID, p.Plan, p.FeeXOF); err != nil {
				return nil, err
			}
		}
	} else {
		remote, err := s.adullam.GetPayment(ctx, p.AdullamID)
		if err != nil {
			log.Printf("[billing] adullam poll error payment=%s: %v", p.ID, err)
			return p, nil
		}
		if remote.Status != p.Status {
			_ = s.store.UpdatePaymentStatus(ctx, paymentID, remote.Status, remote.ID)
			p.Status = remote.Status
		}
		if remote.Status == "failed" && remote.FailedReason != "" {
			p.FailureReason = remote.FailedReason
		}
		if p.Status == "completed" {
			if err := s.activateSubscription(ctx, p.UserID, p.Plan, p.FeeXOF); err != nil {
				return nil, err
			}
		}
	}
	return p, nil
}

// HandleWebhook processes an inbound Adullam payment event.
func (s *Service) HandleWebhook(ctx context.Context, adullamPayment *adullam.Payment) error {
	if adullamPayment.Status != "completed" {
		return nil
	}
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
		return s.activateSubscription(ctx, p.UserID, p.Plan, p.FeeXOF)
	}
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

// HandleSaspayWebhook processes an inbound SasPay payment event.
func (s *Service) HandleSaspayWebhook(ctx context.Context, gatewayPaymentID string) error {
	p, err := s.store.GetPaymentByGatewayID(ctx, gatewayPaymentID, "saspay")
	if err != nil {
		return err
	}
	if p != nil {
		if p.Status == "completed" {
			return nil
		}
		if err := s.store.UpdatePaymentStatus(ctx, p.ID, "completed", gatewayPaymentID); err != nil {
			return err
		}
		return s.activateSubscription(ctx, p.UserID, p.Plan, p.FeeXOF)
	}
	addon, err := s.store.GetAddonByGatewayID(ctx, gatewayPaymentID, "saspay")
	if err != nil {
		return err
	}
	if addon != nil {
		if addon.Status == "completed" {
			return nil
		}
		if err := s.store.UpdateAddonStatus(ctx, addon.ID, "completed", gatewayPaymentID); err != nil {
			return err
		}
		addon.Status = "completed"
		return s.activateAddon(ctx, addon)
	}
	log.Printf("[billing] saspay webhook: unknown payment %s", gatewayPaymentID)
	return nil
}

func (s *Service) ListPayments(ctx context.Context, userID uuid.UUID) ([]BillingPayment, error) {
	return s.store.ListUserPayments(ctx, userID)
}

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
		return 104857600, nil
	}
	return free.MaxFileBytes, nil
}

func (s *Service) activateSubscription(ctx context.Context, userID uuid.UUID, plan *Plan, feeXOF int) error {
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
		go s.sendActivationEmail(userID, plan, periodEnd, feeXOF)
	}
	return nil
}

func (s *Service) sendActivationEmail(userID uuid.UUID, plan *Plan, periodEnd time.Time, feeXOF int) {
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
		FeeXOF:    feeXOF,
	}
	html := confirmationEmailHTML(name, email, plan, periodEnd, feeXOF)
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
	if req.CountryCode == "" {
		req.CountryCode = "CI"
	}
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
	channelOK, provider, err := s.store.GetChannelInfo(ctx, req.Channel)
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
		ID:          uuid.New(),
		UserID:      userID,
		PackageID:   pkg.ID,
		Bytes:       pkg.Bytes,
		PriceXOF:    pkg.PriceXOF,
		Status:      "pending",
		Channel:     req.Channel,
		Phone:       req.Phone,
		Provider:    provider,
		CountryCode: req.CountryCode,
	}
	if err := s.store.CreateAddon(ctx, addon); err != nil {
		return nil, fmt.Errorf("create addon record: %w", err)
	}

	desc := fmt.Sprintf("NEXIUM Storage %s", pkg.Label)
	if len(desc) > 48 {
		desc = desc[:48]
	}

	if provider == "adullam" {
		remote, err := s.adullam.CreatePayment(ctx, addon.ID.String(), adullam.CreatePaymentInput{
			Amount:        pkg.PriceXOF,
			Currency:      "xof",
			CustomerPhone: req.Phone,
			Channel:       adullamChannel(req.Channel),
			Description:   desc,
		})
		if err != nil {
			_ = s.store.UpdateAddonStatus(ctx, addon.ID, "failed", "")
			return nil, apierr.New(502, fmt.Sprintf("payment gateway error: %s", err.Error()))
		}
		_ = s.store.UpdateAddonStatus(ctx, addon.ID, remote.Status, remote.ID)
		addon.AdullamID = remote.ID
		addon.Status = remote.Status
	} else {
		name, email, err := s.getUserInfo(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("getUserInfo: %w", err)
		}
		firstName, lastName := splitName(name)
		remote, err := s.saspay.CreatePayment(ctx, addon.ID.String(), saspay.CreateSoftpayInput{
			Amount:      fmt.Sprintf("%.2f", float64(pkg.PriceXOF)),
			Currency:    currencyForCountry(req.CountryCode),
			Country:     req.CountryCode,
			Customer:    saspay.Customer{Email: email, FirstName: firstName, LastName: lastName, Phone: req.Phone},
			Network:     req.Channel,
			Description: desc,
		})
		if err != nil {
			_ = s.store.UpdateAddonStatus(ctx, addon.ID, "failed", "")
			return nil, apierr.New(502, fmt.Sprintf("payment gateway error: %s", err.Error()))
		}
		normalised := saspay.NormaliseStatus(remote.Status)
		_ = s.store.UpdateAddonStatus(ctx, addon.ID, normalised, remote.ID)
		addon.AdullamID = remote.ID
		addon.Status = normalised
	}
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

	if a.Provider == "saspay" {
		remote, err := s.saspay.GetPayment(ctx, a.AdullamID)
		if err != nil {
			log.Printf("[billing] saspay addon poll error id=%s: %v", a.ID, err)
			return a, nil
		}
		normalised := saspay.NormaliseStatus(remote.Status)
		if normalised != a.Status {
			_ = s.store.UpdateAddonStatus(ctx, addonID, normalised, remote.ID)
			a.Status = normalised
		}
		if normalised == "failed" && remote.FailureReason != "" {
			a.FailureReason = remote.FailureReason
		}
		if a.Status == "completed" {
			if err := s.activateAddon(ctx, a); err != nil {
				return nil, err
			}
		}
	} else {
		remote, err := s.adullam.GetPayment(ctx, a.AdullamID)
		if err != nil {
			log.Printf("[billing] addon poll error id=%s: %v", a.ID, err)
			return a, nil
		}
		if remote.Status != a.Status {
			_ = s.store.UpdateAddonStatus(ctx, addonID, remote.Status, remote.ID)
			a.Status = remote.Status
		}
		if remote.Status == "failed" && remote.FailedReason != "" {
			a.FailureReason = remote.FailedReason
		}
		if a.Status == "completed" {
			if err := s.activateAddon(ctx, a); err != nil {
				return nil, err
			}
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
	addonTotal, err := s.store.SumCompletedAddonBytes(ctx, addon.UserID)
	if err != nil {
		return err
	}
	total := baseBytes + addonTotal
	if err := s.setQuota(ctx, addon.UserID, &total); err != nil {
		return err
	}
	if s.mail != nil && s.getUserInfo != nil {
		go s.sendAddonEmail(addon)
	}
	return nil
}

func (s *Service) sendAddonEmail(addon *StorageAddon) {
	ctx := context.Background()
	name, email, err := s.getUserInfo(ctx, addon.UserID)
	if err != nil || email == "" {
		log.Printf("[billing] sendAddonEmail: getUserInfo error user=%s: %v", addon.UserID, err)
		return
	}
	label := addon.PackageID
	for _, p := range AddonPackages {
		if p.ID == addon.PackageID {
			label = p.Label
			break
		}
	}
	html := addonConfirmationHTML(name, email, addon)
	if err := s.mail.Send(ctx, email, name,
		fmt.Sprintf("Votre stockage a été augmenté de %s — NEXIUM Storage", label),
		html,
	); err != nil {
		log.Printf("[billing] sendAddonEmail: send error user=%s: %v", addon.UserID, err)
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func isTerminal(status string) bool {
	switch status {
	case "completed", "failed", "expired":
		return true
	}
	return false
}

// adullamChannel translates our normalised snake_case slug to the code Adullam
// expects. Adullam CI channels use the old camelCase+uppercase-country format
// (waveCI, mtnCI, ...) while all other country slugs are already accepted as-is.
var adullamChannelMap = map[string]string{
	"mtn_ci":    "mtnCI",
	"wave_ci":   "waveCI",
	"moov_ci":   "moovCI",
	"orange_ci": "orangeCI",
}

func adullamChannel(slug string) string {
	if code, ok := adullamChannelMap[slug]; ok {
		return code
	}
	return slug
}

func currencyForCountry(code string) string {
	switch code {
	case "CM", "GA", "CG", "CF", "TD", "GQ":
		return "XAF"
	case "GN":
		return "GNF"
	default:
		return "XOF"
	}
}

func splitName(name string) (firstName, lastName string) {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return "User", "User"
	}
	if len(parts) == 1 {
		return parts[0], parts[0]
	}
	return parts[0], strings.Join(parts[1:], " ")
}
