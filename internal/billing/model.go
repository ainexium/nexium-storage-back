package billing

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Country struct {
	Code           string  `json:"code"`
	Name           string  `json:"name"`
	CurrencyCode   string  `json:"currency_code"`
	CurrencySymbol string  `json:"currency_symbol"`
	LocalPerXOF    float64 `json:"local_per_xof"`
	FlagEmoji      string  `json:"flag_emoji"`
	IsActive       bool    `json:"is_active"`
}

type Plan struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	Slug          string    `json:"slug"`
	StorageBytes  int64     `json:"storage_bytes"`
	PriceXOF      int       `json:"price_xof"`
	MaxProjects   int       `json:"max_projects"`
	MaxFileBytes  int64     `json:"max_file_bytes"`
	AddonsEnabled bool      `json:"addons_enabled"`
	IsActive      bool      `json:"is_active"`
}

type Subscription struct {
	ID                 uuid.UUID  `json:"id"`
	UserID             uuid.UUID  `json:"user_id"`
	PlanID             uuid.UUID  `json:"plan_id"`
	Status             string     `json:"status"` // active, expired, cancelled
	CurrentPeriodStart time.Time  `json:"current_period_start"`
	CurrentPeriodEnd   *time.Time `json:"current_period_end"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	Plan               *Plan      `json:"plan,omitempty"`
}

type BillingPayment struct {
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	PlanID      uuid.UUID `json:"plan_id"`
	AdullamID   string    `json:"adullam_id"` // holds gateway payment ID (adullam or saspay)
	AmountXOF   int       `json:"amount_xof"`
	FeeXOF      int       `json:"fee_xof"`     // network fee charged to customer (0 for SasPay where fee is unknown)
	Status      string    `json:"status"`      // pending, processing, completed, failed, expired
	Channel     string    `json:"channel"`
	Phone       string    `json:"phone"`
	Provider    string    `json:"provider"`     // "adullam" | "saspay"
	CountryCode string    `json:"country_code"` // e.g. "CI", "BJ"
	RedirectURL   string    `json:"redirect_url,omitempty"`
	FailureReason string    `json:"failure_reason,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	Plan          *Plan     `json:"plan,omitempty"`
}

type CheckoutRequest struct {
	PlanID      string `json:"plan_id"`
	Channel     string `json:"channel"`
	Phone       string `json:"phone"`
	CountryCode string `json:"country_code"` // defaults to "CI" if empty
}

// WebhookEvent is the payload sent by Adullam to our webhook endpoint.
type WebhookEvent struct {
	Event string          `json:"event"` // "payment" or "transfer"
	Data  json.RawMessage `json:"data"`
}

// SaspayWebhookEvent is the payload sent by SasPay.
type SaspayWebhookEvent struct {
	EventType string          `json:"event_type"` // "transaction.success", etc.
	Data      json.RawMessage `json:"data"`
}

type SaspayWebhookPayment struct {
	ID     string `json:"id"`
	Status string `json:"status"` // SUCCESS, FAILED, EXPIRED, ...
}

// ── Storage add-ons ─────────────────────────────────────────────────────────

type AddonPackage struct {
	ID       string `json:"id"`
	Bytes    int64  `json:"bytes"`
	PriceXOF int    `json:"price_xof"`
	Label    string `json:"label"`
}

var AddonPackages = []AddonPackage{
	{ID: "addon-50gb",  Bytes: 53687091200,  PriceXOF: 1500,  Label: "+50 GB"},
	{ID: "addon-100gb", Bytes: 107374182400, PriceXOF: 2800,  Label: "+100 GB"},
	{ID: "addon-250gb", Bytes: 268435456000, PriceXOF: 6500,  Label: "+250 GB"},
	{ID: "addon-500gb", Bytes: 536870912000, PriceXOF: 12000, Label: "+500 GB"},
}

type StorageAddon struct {
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	PackageID   string    `json:"package_id"`
	Bytes       int64     `json:"bytes"`
	PriceXOF    int       `json:"price_xof"`
	FeeXOF      int       `json:"fee_xof"` // network fee charged to customer
	AdullamID   string    `json:"adullam_id"` // holds gateway payment ID
	Status      string    `json:"status"`      // pending, processing, completed, failed, expired
	Channel     string    `json:"channel"`
	Phone       string    `json:"phone"`
	Provider    string    `json:"provider"`
	CountryCode   string    `json:"country_code"`
	FailureReason string    `json:"failure_reason,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type PaymentsPage struct {
	Payments   []BillingPayment `json:"payments"`
	HasMore    bool             `json:"has_more"`
	NextCursor string           `json:"next_cursor"`
}

type AddonCheckoutRequest struct {
	PackageID   string `json:"package_id"`
	Channel     string `json:"channel"`
	Phone       string `json:"phone"`
	CountryCode string `json:"country_code"`
}
