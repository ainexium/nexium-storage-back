package billing

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Plan struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	StorageBytes   int64     `json:"storage_bytes"`
	PriceXOF       int       `json:"price_xof"`
	MaxProjects    int       `json:"max_projects"`
	MaxFileBytes   int64     `json:"max_file_bytes"`
	AddonsEnabled  bool      `json:"addons_enabled"`
	IsActive       bool      `json:"is_active"`
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
	AdullamID   string    `json:"adullam_id"`
	AmountXOF   int       `json:"amount_xof"`
	Status      string    `json:"status"` // pending, processing, completed, failed, expired
	Channel     string    `json:"channel"`
	Phone       string    `json:"phone"`
	RedirectURL string    `json:"redirect_url,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Plan        *Plan     `json:"plan,omitempty"`
}

type CheckoutRequest struct {
	PlanID  string `json:"plan_id"`
	Channel string `json:"channel"`
	Phone   string `json:"phone"`
}

// WebhookEvent is the payload sent by Adullam to our webhook endpoint.
type WebhookEvent struct {
	Event string          `json:"event"` // "payment" or "transfer"
	Data  json.RawMessage `json:"data"`
}

// ── Storage add-ons ─────────────────────────────────────────────────────────

type AddonPackage struct {
	ID       string `json:"id"`
	Bytes    int64  `json:"bytes"`
	PriceXOF int    `json:"price_xof"`
	Label    string `json:"label"`
}

// AddonPackages are the available top-up options (Pro/Business only).
var AddonPackages = []AddonPackage{
	{ID: "addon-50gb",  Bytes: 53687091200,  PriceXOF: 1500,  Label: "+50 GB"},
	{ID: "addon-100gb", Bytes: 107374182400, PriceXOF: 2800,  Label: "+100 GB"},
	{ID: "addon-250gb", Bytes: 268435456000, PriceXOF: 6500,  Label: "+250 GB"},
	{ID: "addon-500gb", Bytes: 536870912000, PriceXOF: 12000, Label: "+500 GB"},
}

type StorageAddon struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	PackageID string    `json:"package_id"`
	Bytes     int64     `json:"bytes"`
	PriceXOF  int       `json:"price_xof"`
	AdullamID string    `json:"adullam_id"`
	Status    string    `json:"status"` // pending, processing, completed, failed, expired
	Channel   string    `json:"channel"`
	Phone     string    `json:"phone"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AddonCheckoutRequest struct {
	PackageID string `json:"package_id"`
	Channel   string `json:"channel"`
	Phone     string `json:"phone"`
}
