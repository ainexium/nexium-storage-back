package billing

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	ListCountries(ctx context.Context) ([]Country, error)
	ListActiveCountries(ctx context.Context) ([]Country, error)
	UpdateCountry(ctx context.Context, code string, isActive bool, localPerXOF float64) error
	ListPlans(ctx context.Context) ([]Plan, error)
	GetPlanByID(ctx context.Context, id uuid.UUID) (*Plan, error)
	GetPlanBySlug(ctx context.Context, slug string) (*Plan, error)
	GetSubscription(ctx context.Context, userID uuid.UUID) (*Subscription, error)
	UpsertSubscription(ctx context.Context, userID, planID uuid.UUID, status string, periodEnd *time.Time) error
	ExpireSubscription(ctx context.Context, userID uuid.UUID) error
	CreatePayment(ctx context.Context, p *BillingPayment) error
	GetPayment(ctx context.Context, id uuid.UUID) (*BillingPayment, error)
	GetPaymentByAdullamID(ctx context.Context, adullamID string) (*BillingPayment, error)
	GetPaymentByGatewayID(ctx context.Context, gatewayID, provider string) (*BillingPayment, error)
	UpdatePaymentStatus(ctx context.Context, id uuid.UUID, status, adullamID string) error
	SetPaymentRedirectURL(ctx context.Context, id uuid.UUID, url string) error
	ListUserPayments(ctx context.Context, userID uuid.UUID) ([]BillingPayment, error)
	GetChannelInfo(ctx context.Context, slug string) (active bool, provider string, err error)
	// Add-ons
	CreateAddon(ctx context.Context, a *StorageAddon) error
	GetAddon(ctx context.Context, id uuid.UUID) (*StorageAddon, error)
	GetAddonByAdullamID(ctx context.Context, adullamID string) (*StorageAddon, error)
	GetAddonByGatewayID(ctx context.Context, gatewayID, provider string) (*StorageAddon, error)
	UpdateAddonStatus(ctx context.Context, id uuid.UUID, status, adullamID string) error
	ListUserAddons(ctx context.Context, userID uuid.UUID) ([]StorageAddon, error)
	SumCompletedAddonBytes(ctx context.Context, userID uuid.UUID) (int64, error)
}

type pgStore struct{ db *pgxpool.Pool }

func NewStore(db *pgxpool.Pool) Store { return &pgStore{db} }

// ── Countries ────────────────────────────────────────────────────────────────

func (s *pgStore) ListCountries(ctx context.Context) ([]Country, error) {
	rows, err := s.db.Query(ctx,
		`SELECT code, name, currency_code, currency_symbol, CAST(local_per_xof AS float8), flag_emoji, is_active
		 FROM billing_countries ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Country
	for rows.Next() {
		var c Country
		if err := rows.Scan(&c.Code, &c.Name, &c.CurrencyCode, &c.CurrencySymbol, &c.LocalPerXOF, &c.FlagEmoji, &c.IsActive); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (s *pgStore) ListActiveCountries(ctx context.Context) ([]Country, error) {
	rows, err := s.db.Query(ctx,
		`SELECT code, name, currency_code, currency_symbol, CAST(local_per_xof AS float8), flag_emoji, is_active
		 FROM billing_countries WHERE is_active = true AND code != 'INT' ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Country
	for rows.Next() {
		var c Country
		if err := rows.Scan(&c.Code, &c.Name, &c.CurrencyCode, &c.CurrencySymbol, &c.LocalPerXOF, &c.FlagEmoji, &c.IsActive); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (s *pgStore) UpdateCountry(ctx context.Context, code string, isActive bool, localPerXOF float64) error {
	_, err := s.db.Exec(ctx,
		`UPDATE billing_countries SET is_active=$2, local_per_xof=$3 WHERE code=$1`,
		code, isActive, localPerXOF)
	return err
}

// ── Plans ────────────────────────────────────────────────────────────────────

func (s *pgStore) ListPlans(ctx context.Context) ([]Plan, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, name, slug, storage_bytes, price_xof, max_projects, max_file_bytes, addons_enabled, is_active
		 FROM plans WHERE is_active = true ORDER BY price_xof ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var plans []Plan
	for rows.Next() {
		var p Plan
		if err := rows.Scan(&p.ID, &p.Name, &p.Slug, &p.StorageBytes, &p.PriceXOF, &p.MaxProjects, &p.MaxFileBytes, &p.AddonsEnabled, &p.IsActive); err != nil {
			return nil, err
		}
		plans = append(plans, p)
	}
	return plans, rows.Err()
}

func (s *pgStore) GetPlanByID(ctx context.Context, id uuid.UUID) (*Plan, error) {
	p := &Plan{}
	err := s.db.QueryRow(ctx,
		`SELECT id, name, slug, storage_bytes, price_xof, max_projects, max_file_bytes, addons_enabled, is_active FROM plans WHERE id = $1`, id,
	).Scan(&p.ID, &p.Name, &p.Slug, &p.StorageBytes, &p.PriceXOF, &p.MaxProjects, &p.MaxFileBytes, &p.AddonsEnabled, &p.IsActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return p, err
}

func (s *pgStore) GetPlanBySlug(ctx context.Context, slug string) (*Plan, error) {
	p := &Plan{}
	err := s.db.QueryRow(ctx,
		`SELECT id, name, slug, storage_bytes, price_xof, max_projects, max_file_bytes, addons_enabled, is_active FROM plans WHERE slug = $1`, slug,
	).Scan(&p.ID, &p.Name, &p.Slug, &p.StorageBytes, &p.PriceXOF, &p.MaxProjects, &p.MaxFileBytes, &p.AddonsEnabled, &p.IsActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return p, err
}

// ── Subscriptions ────────────────────────────────────────────────────────────

func (s *pgStore) GetSubscription(ctx context.Context, userID uuid.UUID) (*Subscription, error) {
	sub := &Subscription{}
	plan := &Plan{}
	err := s.db.QueryRow(ctx,
		`SELECT s.id, s.user_id, s.plan_id, s.status,
		        s.current_period_start, s.current_period_end, s.created_at, s.updated_at,
		        p.id, p.name, p.slug, p.storage_bytes, p.price_xof, p.max_projects, p.max_file_bytes, p.addons_enabled, p.is_active
		 FROM subscriptions s JOIN plans p ON p.id = s.plan_id
		 WHERE s.user_id = $1`, userID,
	).Scan(
		&sub.ID, &sub.UserID, &sub.PlanID, &sub.Status,
		&sub.CurrentPeriodStart, &sub.CurrentPeriodEnd, &sub.CreatedAt, &sub.UpdatedAt,
		&plan.ID, &plan.Name, &plan.Slug, &plan.StorageBytes, &plan.PriceXOF, &plan.MaxProjects, &plan.MaxFileBytes, &plan.AddonsEnabled, &plan.IsActive,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sub.Plan = plan
	return sub, nil
}

func (s *pgStore) ExpireSubscription(ctx context.Context, userID uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`UPDATE subscriptions
		 SET status = 'expired', data_grace_end = now() + interval '30 days', updated_at = now()
		 WHERE user_id = $1 AND status = 'active'`,
		userID,
	)
	return err
}

func (s *pgStore) UpsertSubscription(ctx context.Context, userID, planID uuid.UUID, status string, periodEnd *time.Time) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO subscriptions (id, user_id, plan_id, status, current_period_start, current_period_end, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, now(), $5, now(), now())
		 ON CONFLICT (user_id) DO UPDATE SET
		    plan_id               = EXCLUDED.plan_id,
		    status                = EXCLUDED.status,
		    current_period_start  = now(),
		    current_period_end    = EXCLUDED.current_period_end,
		    data_grace_end        = NULL,
		    grace_reminded_15d_at = NULL,
		    grace_reminded_7d_at  = NULL,
		    grace_reminded_1d_at  = NULL,
		    updated_at            = now()`,
		uuid.New(), userID, planID, status, periodEnd,
	)
	return err
}

// ── Payments ─────────────────────────────────────────────────────────────────

func (s *pgStore) CreatePayment(ctx context.Context, p *BillingPayment) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO billing_payments (id, user_id, plan_id, adullam_id, amount_xof, fee_xof, status, channel, phone, redirect_url, provider, country_code, created_at, updated_at)
		 VALUES ($1, $2, $3, NULL, $4, $5, $6, $7, $8, NULLIF($9,''), $10, $11, now(), now())`,
		p.ID, p.UserID, p.PlanID, p.AmountXOF, p.FeeXOF, p.Status, p.Channel, p.Phone, p.RedirectURL, p.Provider, p.CountryCode,
	)
	return err
}

func scanPayment(row pgx.Row) (*BillingPayment, error) {
	p := &BillingPayment{}
	plan := &Plan{}
	err := row.Scan(
		&p.ID, &p.UserID, &p.PlanID, &p.AdullamID,
		&p.AmountXOF, &p.FeeXOF, &p.Status, &p.Channel, &p.Phone, &p.RedirectURL, &p.Provider, &p.CountryCode,
		&p.CreatedAt, &p.UpdatedAt,
		&plan.ID, &plan.Name, &plan.Slug, &plan.StorageBytes, &plan.PriceXOF, &plan.MaxProjects, &plan.MaxFileBytes, &plan.AddonsEnabled, &plan.IsActive,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.Plan = plan
	return p, nil
}

const paymentSelectJoin = `
	SELECT bp.id, bp.user_id, bp.plan_id, COALESCE(bp.adullam_id, ''),
	       bp.amount_xof, COALESCE(bp.fee_xof, 0), bp.status, bp.channel, bp.phone, COALESCE(bp.redirect_url, ''),
	       COALESCE(bp.provider,'adullam'), COALESCE(bp.country_code,'CI'),
	       bp.created_at, bp.updated_at,
	       pl.id, pl.name, pl.slug, pl.storage_bytes, pl.price_xof, pl.max_projects, pl.max_file_bytes, pl.addons_enabled, pl.is_active
	FROM billing_payments bp JOIN plans pl ON pl.id = bp.plan_id`

func (s *pgStore) GetPayment(ctx context.Context, id uuid.UUID) (*BillingPayment, error) {
	return scanPayment(s.db.QueryRow(ctx, paymentSelectJoin+` WHERE bp.id = $1`, id))
}

func (s *pgStore) GetPaymentByAdullamID(ctx context.Context, adullamID string) (*BillingPayment, error) {
	return scanPayment(s.db.QueryRow(ctx, paymentSelectJoin+` WHERE bp.adullam_id = $1 AND COALESCE(bp.provider,'adullam') = 'adullam'`, adullamID))
}

func (s *pgStore) GetPaymentByGatewayID(ctx context.Context, gatewayID, provider string) (*BillingPayment, error) {
	return scanPayment(s.db.QueryRow(ctx, paymentSelectJoin+` WHERE bp.adullam_id = $1 AND COALESCE(bp.provider,'adullam') = $2`, gatewayID, provider))
}

func (s *pgStore) UpdatePaymentStatus(ctx context.Context, id uuid.UUID, status, adullamID string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE billing_payments SET status = $2, adullam_id = NULLIF($3, ''), updated_at = now() WHERE id = $1`,
		id, status, adullamID,
	)
	return err
}

func (s *pgStore) SetPaymentRedirectURL(ctx context.Context, id uuid.UUID, url string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE billing_payments SET redirect_url = $2, updated_at = now() WHERE id = $1`,
		id, url,
	)
	return err
}

func (s *pgStore) ListUserPayments(ctx context.Context, userID uuid.UUID) ([]BillingPayment, error) {
	rows, err := s.db.Query(ctx,
		paymentSelectJoin+` WHERE bp.user_id = $1 ORDER BY bp.created_at DESC LIMIT 20`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var payments []BillingPayment
	for rows.Next() {
		var p BillingPayment
		var plan Plan
		if err := rows.Scan(
			&p.ID, &p.UserID, &p.PlanID, &p.AdullamID,
			&p.AmountXOF, &p.FeeXOF, &p.Status, &p.Channel, &p.Phone, &p.RedirectURL, &p.Provider, &p.CountryCode,
			&p.CreatedAt, &p.UpdatedAt,
			&plan.ID, &plan.Name, &plan.Slug, &plan.StorageBytes, &plan.PriceXOF, &plan.MaxProjects, &plan.MaxFileBytes, &plan.AddonsEnabled, &plan.IsActive,
		); err != nil {
			return nil, err
		}
		p.Plan = &plan
		payments = append(payments, p)
	}
	return payments, rows.Err()
}

func (s *pgStore) GetChannelInfo(ctx context.Context, slug string) (bool, string, error) {
	var active bool
	var provider string
	err := s.db.QueryRow(ctx,
		`SELECT is_active, provider FROM payment_channels WHERE slug = $1`, slug,
	).Scan(&active, &provider)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, "", nil
	}
	return active, provider, err
}

// ── Add-on implementations ───────────────────────────────────────────────────

func (s *pgStore) CreateAddon(ctx context.Context, a *StorageAddon) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO storage_addons (id, user_id, package_id, bytes, price_xof, fee_xof, adullam_id, status, channel, phone, provider, country_code, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, NULL, $7, $8, $9, $10, $11, now(), now())`,
		a.ID, a.UserID, a.PackageID, a.Bytes, a.PriceXOF, a.FeeXOF, a.Status, a.Channel, a.Phone, a.Provider, a.CountryCode,
	)
	return err
}

func (s *pgStore) GetAddon(ctx context.Context, id uuid.UUID) (*StorageAddon, error) {
	a := &StorageAddon{}
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, package_id, bytes, price_xof, COALESCE(fee_xof,0), COALESCE(adullam_id,''), status, channel, phone,
		        COALESCE(provider,'adullam'), COALESCE(country_code,'CI'), created_at, updated_at
		 FROM storage_addons WHERE id = $1`, id,
	).Scan(&a.ID, &a.UserID, &a.PackageID, &a.Bytes, &a.PriceXOF, &a.FeeXOF, &a.AdullamID, &a.Status, &a.Channel, &a.Phone,
		&a.Provider, &a.CountryCode, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

func (s *pgStore) GetAddonByAdullamID(ctx context.Context, adullamID string) (*StorageAddon, error) {
	a := &StorageAddon{}
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, package_id, bytes, price_xof, COALESCE(fee_xof,0), COALESCE(adullam_id,''), status, channel, phone,
		        COALESCE(provider,'adullam'), COALESCE(country_code,'CI'), created_at, updated_at
		 FROM storage_addons WHERE adullam_id = $1 AND COALESCE(provider,'adullam') = 'adullam'`, adullamID,
	).Scan(&a.ID, &a.UserID, &a.PackageID, &a.Bytes, &a.PriceXOF, &a.FeeXOF, &a.AdullamID, &a.Status, &a.Channel, &a.Phone,
		&a.Provider, &a.CountryCode, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

func (s *pgStore) GetAddonByGatewayID(ctx context.Context, gatewayID, provider string) (*StorageAddon, error) {
	a := &StorageAddon{}
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, package_id, bytes, price_xof, COALESCE(fee_xof,0), COALESCE(adullam_id,''), status, channel, phone,
		        COALESCE(provider,'adullam'), COALESCE(country_code,'CI'), created_at, updated_at
		 FROM storage_addons WHERE adullam_id = $1 AND COALESCE(provider,'adullam') = $2`, gatewayID, provider,
	).Scan(&a.ID, &a.UserID, &a.PackageID, &a.Bytes, &a.PriceXOF, &a.FeeXOF, &a.AdullamID, &a.Status, &a.Channel, &a.Phone,
		&a.Provider, &a.CountryCode, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

func (s *pgStore) UpdateAddonStatus(ctx context.Context, id uuid.UUID, status, adullamID string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE storage_addons SET status = $2, adullam_id = NULLIF($3,''), updated_at = now() WHERE id = $1`,
		id, status, adullamID,
	)
	return err
}

func (s *pgStore) ListUserAddons(ctx context.Context, userID uuid.UUID) ([]StorageAddon, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, user_id, package_id, bytes, price_xof, COALESCE(fee_xof,0), COALESCE(adullam_id,''), status, channel, phone,
		        COALESCE(provider,'adullam'), COALESCE(country_code,'CI'), created_at, updated_at
		 FROM storage_addons WHERE user_id = $1 ORDER BY created_at DESC LIMIT 50`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var addons []StorageAddon
	for rows.Next() {
		var a StorageAddon
		if err := rows.Scan(&a.ID, &a.UserID, &a.PackageID, &a.Bytes, &a.PriceXOF, &a.FeeXOF, &a.AdullamID, &a.Status, &a.Channel, &a.Phone,
			&a.Provider, &a.CountryCode, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		addons = append(addons, a)
	}
	return addons, rows.Err()
}

func (s *pgStore) SumCompletedAddonBytes(ctx context.Context, userID uuid.UUID) (int64, error) {
	var total int64
	err := s.db.QueryRow(ctx,
		`SELECT COALESCE(SUM(bytes), 0) FROM storage_addons WHERE user_id = $1 AND status = 'completed'`,
		userID,
	).Scan(&total)
	return total, err
}
