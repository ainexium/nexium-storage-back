package buckets

import (
	"time"

	"github.com/google/uuid"
)

type Bucket struct {
	ID        uuid.UUID `json:"id"`
	ProjectID uuid.UUID `json:"project_id"`
	Name      string    `json:"name"`
	IsPublic  bool      `json:"is_public"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateRequest struct {
	Name     string `json:"name"`
	IsPublic *bool  `json:"is_public"` // nil → defaults to true
}

type UpdateRequest struct {
	Name     string `json:"name"`
	IsPublic *bool  `json:"is_public"`
}
