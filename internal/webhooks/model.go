package webhooks

import (
	"time"

	"github.com/google/uuid"
)

type Webhook struct {
	ID        uuid.UUID `json:"id"`
	ProjectID uuid.UUID `json:"project_id"`
	URL       string    `json:"url"`
	Events    []string  `json:"events"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Delivery struct {
	ID          uuid.UUID  `json:"id"`
	WebhookID   uuid.UUID  `json:"webhook_id"`
	Event       string     `json:"event"`
	StatusCode  *int       `json:"status_code"`
	Success     bool       `json:"success"`
	Attempts    int        `json:"attempts"`
	Error       *string    `json:"error,omitempty"`
	DeliveredAt *time.Time `json:"delivered_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// Supported events
const (
	EventFileCreated = "file.created"
	EventFileDeleted = "file.deleted"
	EventFileRenamed = "file.renamed"
)

var AllEvents = []string{EventFileCreated, EventFileDeleted, EventFileRenamed}
