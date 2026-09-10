package projects

import (
	"time"

	"github.com/google/uuid"
)

type Project struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateRequest struct {
	Name string `json:"name"`
}

type UpdateRequest struct {
	Name string `json:"name"`
}
