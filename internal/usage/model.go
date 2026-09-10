package usage

import "github.com/google/uuid"

type Summary struct {
	ProjectID    uuid.UUID `json:"project_id"`
	StorageBytes int64     `json:"storage_bytes"`
	FileCount    int64     `json:"file_count"`
	BucketCount  int64     `json:"bucket_count"`
}
