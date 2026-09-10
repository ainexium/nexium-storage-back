package files

import (
	"time"

	"github.com/google/uuid"
)

type File struct {
	ID             uuid.UUID `json:"id"`
	BucketID       uuid.UUID `json:"bucket_id"`
	ObjectKey      string    `json:"object_key"`
	Filename       string    `json:"filename"`
	MimeType       string    `json:"mime_type"`
	SizeBytes      int64     `json:"size_bytes"`
	URL            string    `json:"url,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	BucketIsPublic bool      `json:"-"` // transient from join
}

type UploadResponse struct {
	File *File `json:"file"`
}

type DownloadResponse struct {
	URL string `json:"url"`
}

type PresignData struct {
	FileID    uuid.UUID `json:"file_id"`
	ObjectKey string    `json:"object_key"`
	UploadURL string    `json:"upload_url"`
	ExpiresIn int       `json:"expires_in"` // secondes
}

var AllowedMIMETypes = map[string]bool{
	// Images
	"image/jpeg":    true,
	"image/png":     true,
	"image/gif":     true,
	"image/webp":    true,
	"image/svg+xml": true,
	"image/heic":    true,
	"image/heif":    true,
	// Vidéos
	"video/mp4":        true,
	"video/webm":       true,
	"video/quicktime":  true, // .mov (iPhone / Mac)
	"video/x-msvideo":  true, // .avi
	"video/mpeg":       true,
	"video/3gpp":       true, // .3gp (mobile)
	"video/x-matroska": true, // .mkv
	// Documents
	"application/pdf":  true,
	"text/plain":       true,
	"text/csv":         true,
	"application/zip":  true,
	"application/json": true,
	"application/xml":  true,
	"text/xml":         true,
}
