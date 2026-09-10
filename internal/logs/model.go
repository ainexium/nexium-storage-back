package logs

import (
	"time"

	"github.com/google/uuid"
)

type Entry struct {
	ID           uuid.UUID `json:"id"`
	UserID       uuid.UUID `json:"user_id"`
	UserName     string    `json:"user_name,omitempty"`
	UserEmail    string    `json:"user_email,omitempty"`
	IsSuperAdmin bool      `json:"is_super_admin"`
	IsAdmin      bool      `json:"is_admin"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type,omitempty"`
	ResourceID   string    `json:"resource_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type Filter struct {
	UserID *uuid.UUID
	Action string
	Limit  int
}
