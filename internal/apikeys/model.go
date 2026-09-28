package apikeys

import (
	"time"

	"github.com/google/uuid"
)

type APIKey struct {
	ID               uuid.UUID   `json:"id"`
	ProjectID        uuid.UUID   `json:"project_id"`
	Name             string      `json:"name"`
	Prefix           string      `json:"prefix"`
	AllowedBucketIDs []uuid.UUID `json:"allowed_bucket_ids"` // nil = all buckets
	LastUsedAt       *time.Time  `json:"last_used_at"`
	RevokedAt        *time.Time  `json:"revoked_at"`
	CreatedAt        time.Time   `json:"created_at"`
}

type CreateRequest struct {
	Name             string      `json:"name"`
	AllowedBucketIDs []uuid.UUID `json:"allowed_bucket_ids"` // nil/empty = all buckets
}

// CreateResponse includes the full key, shown only once.
type CreateResponse struct {
	APIKey  *APIKey `json:"api_key"`
	FullKey string  `json:"key"`
}
