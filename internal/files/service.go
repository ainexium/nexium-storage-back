package files

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"nexium.ai/api/internal/storage"
	"nexium.ai/api/pkg/apierr"
)

// CheckBucket vérifie l'ownership et retourne la visibilité du bucket.
type CheckBucket func(ctx context.Context, bucketID, userID uuid.UUID) (exists, isPublic bool, err error)

// GetUserQuota retourne le quota en bytes de l'user. -1 = pas de quota custom (utiliser le défaut plateforme).
type GetUserQuota func(ctx context.Context, userID uuid.UUID) (int64, error)

// GetUserFileSizeLimit retourne la taille maximale par fichier selon le plan de l'utilisateur.
type GetUserFileSizeLimit func(ctx context.Context, userID uuid.UUID) (int64, error)

// IsPlatformLocked retourne true si le super admin a verrouillé tous les uploads.
type IsPlatformLocked func(ctx context.Context) (bool, error)

type Service interface {
	Upload(ctx context.Context, userID, bucketID uuid.UUID, filename, mimeType string, size int64, body io.Reader) (*File, error)
	List(ctx context.Context, userID, bucketID uuid.UUID, search string, page, perPage int) (*PagedFiles, error)
	GetDownloadURL(ctx context.Context, userID, id uuid.UUID) (string, error)
	GetPublicDownloadURL(ctx context.Context, id uuid.UUID) (string, error)
	StreamPublicFile(ctx context.Context, id uuid.UUID) (io.ReadCloser, string, int64, error)
	StreamFile(ctx context.Context, userID, id uuid.UUID) (io.ReadCloser, string, int64, error)
	Rename(ctx context.Context, userID, id uuid.UUID, filename string) (*File, error)
	Delete(ctx context.Context, userID, id uuid.UUID) error
	DeleteAllInBucket(ctx context.Context, bucketID uuid.UUID) error
	PresignUpload(ctx context.Context, userID, bucketID uuid.UUID, filename, mimeType string) (*PresignData, error)
	ConfirmUpload(ctx context.Context, userID, bucketID, fileID uuid.UUID, objectKey, filename, mimeType string, sizeBytes int64) (*File, error)
}

type service struct {
	store               Store
	storage             *storage.R2Client
	checkBucket         CheckBucket
	defaultQuota        int64
	getUserQuota        GetUserQuota
	getUserFileSizeLimit GetUserFileSizeLimit
	isPlatformLocked    IsPlatformLocked
	appURL              string // base URL de l'API, pour construire les URLs publiques stables
}

func NewService(
	store Store,
	r2 *storage.R2Client,
	checkBucket CheckBucket,
	defaultQuota int64,
	getUserQuota GetUserQuota,
	getUserFileSizeLimit GetUserFileSizeLimit,
	isPlatformLocked IsPlatformLocked,
	appURL string,
) Service {
	return &service{
		store:                store,
		storage:              r2,
		checkBucket:          checkBucket,
		defaultQuota:         defaultQuota,
		getUserQuota:         getUserQuota,
		getUserFileSizeLimit: getUserFileSizeLimit,
		isPlatformLocked:     isPlatformLocked,
		appURL:               strings.TrimRight(appURL, "/"),
	}
}

// publicFileURL construit l'URL publique stable d'un fichier via l'endpoint NEXIUM.
// Contrairement à R2_PUBLIC_URL, cet endpoint vérifie is_public avant de servir.
func (s *service) publicFileURL(fileID uuid.UUID) string {
	return s.appURL + "/api/v1/public/files/" + fileID.String()
}

func (s *service) checkQuota(ctx context.Context, userID uuid.UUID, incomingBytes int64) error {
	// 1. Verrou plateforme — super admin peut couper tous les uploads
	if locked, err := s.isPlatformLocked(ctx); err == nil && locked {
		return apierr.ErrBadRequest("les uploads sont temporairement désactivés — contactez le support")
	}

	// 2. Quota par user
	quota, err := s.getUserQuota(ctx, userID)
	if err != nil {
		return err
	}
	if quota < 0 {
		// Pas de quota custom → utiliser le défaut plateforme
		quota = s.defaultQuota
	}
	if quota == 0 {
		// 0 = illimité (réservé aux comptes spéciaux)
		return nil
	}

	used, err := s.store.TotalStorageBytesByUser(ctx, userID)
	if err != nil {
		return err
	}
	if used+incomingBytes > quota {
		return apierr.ErrBadRequest("quota de stockage dépassé — passez à un plan supérieur pour continuer")
	}
	return nil
}

func (s *service) checkFileSize(ctx context.Context, userID uuid.UUID, size int64) error {
	if s.getUserFileSizeLimit == nil {
		return nil
	}
	limit, err := s.getUserFileSizeLimit(ctx, userID)
	if err != nil || limit <= 0 {
		return nil // pas de limite connue → laisser passer
	}
	if size > limit {
		return apierr.ErrBadRequest(fmt.Sprintf("fichier trop volumineux pour votre plan (%d MB max) — passez à un plan supérieur", limit>>20))
	}
	return nil
}

func (s *service) Upload(ctx context.Context, userID, bucketID uuid.UUID, filename, mimeType string, size int64, body io.Reader) (*File, error) {
	if !AllowedMIMETypes[mimeType] {
		return nil, apierr.ErrBadRequest(fmt.Sprintf("mime type %q is not allowed", mimeType))
	}
	if err := s.checkFileSize(ctx, userID, size); err != nil {
		return nil, err
	}
	if err := s.checkQuota(ctx, userID, size); err != nil {
		return nil, err
	}
	ok, isPublic, err := s.checkBucket(ctx, bucketID, userID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apierr.ErrNotFound
	}

	fileID := uuid.New()
	objectKey := fmt.Sprintf("%s/%s/%s_%s", bucketID, fileID, fileID, filename)

	if err := s.storage.Put(ctx, objectKey, mimeType, body, size); err != nil {
		return nil, fmt.Errorf("upload to R2: %w", err)
	}

	f := &File{
		ID:        fileID,
		BucketID:  bucketID,
		ObjectKey: objectKey,
		Filename:  filename,
		MimeType:  mimeType,
		SizeBytes: size,
		CreatedAt: time.Now().UTC(),
	}
	if isPublic {
		f.URL = s.publicFileURL(fileID)
	}
	if err := s.store.Create(ctx, f); err != nil {
		_ = s.storage.Delete(ctx, objectKey)
		return nil, err
	}
	return f, nil
}

type PagedFiles struct {
	Files   []*File `json:"files"`
	Total   int     `json:"total"`
	Page    int     `json:"page"`
	PerPage int     `json:"per_page"`
}

func (s *service) List(ctx context.Context, userID, bucketID uuid.UUID, search string, page, perPage int) (*PagedFiles, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 24
	}
	offset := (page - 1) * perPage

	total, err := s.store.CountByBucket(ctx, bucketID, userID, search)
	if err != nil {
		return nil, err
	}

	fileList, err := s.store.ListByBucket(ctx, bucketID, userID, search, perPage, offset)
	if err != nil {
		return nil, err
	}
	if fileList == nil {
		fileList = []*File{}
	}
	for _, f := range fileList {
		if f.BucketIsPublic {
			f.URL = s.publicFileURL(f.ID)
		}
	}
	return &PagedFiles{Files: fileList, Total: total, Page: page, PerPage: perPage}, nil
}

func (s *service) Rename(ctx context.Context, userID, id uuid.UUID, filename string) (*File, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return nil, apierr.ErrBadRequest("filename is required")
	}
	f, err := s.store.GetByID(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, apierr.ErrNotFound
	}
	if err := s.store.Rename(ctx, id, userID, filename); err != nil {
		return nil, err
	}
	f.Filename = filename
	if f.BucketIsPublic {
		f.URL = s.publicFileURL(f.ID)
	}
	return f, nil
}

func (s *service) DeleteAllInBucket(ctx context.Context, bucketID uuid.UUID) error {
	return s.storage.DeleteByPrefix(ctx, bucketID.String()+"/")
}

func (s *service) GetDownloadURL(ctx context.Context, userID, id uuid.UUID) (string, error) {
	f, err := s.store.GetByID(ctx, id, userID)
	if err != nil {
		return "", err
	}
	if f == nil {
		return "", apierr.ErrNotFound
	}
	// Toujours utiliser une URL presignée — R2 reste entièrement privé
	url, err := s.storage.PresignGetURL(ctx, f.ObjectKey, time.Hour)
	if err != nil {
		return "", err
	}
	return url, nil
}

// GetPublicDownloadURL génère une URL presignée pour un fichier public (utilisé pour le téléchargement direct).
func (s *service) GetPublicDownloadURL(ctx context.Context, id uuid.UUID) (string, error) {
	f, err := s.store.GetByIDPublic(ctx, id)
	if err != nil {
		return "", err
	}
	if f == nil || !f.BucketIsPublic {
		return "", apierr.ErrNotFound
	}
	return s.storage.PresignGetURL(ctx, f.ObjectKey, time.Hour)
}

// StreamPublicFile lit le contenu d'un fichier public depuis R2 et le retourne pour proxy côté API.
func (s *service) StreamPublicFile(ctx context.Context, id uuid.UUID) (io.ReadCloser, string, int64, error) {
	f, err := s.store.GetByIDPublic(ctx, id)
	if err != nil {
		return nil, "", 0, err
	}
	if f == nil || !f.BucketIsPublic {
		return nil, "", 0, apierr.ErrNotFound
	}
	return s.storage.GetObject(ctx, f.ObjectKey)
}

// StreamFile lit le contenu d'un fichier privé depuis R2 pour proxy authentifié.
func (s *service) StreamFile(ctx context.Context, userID, id uuid.UUID) (io.ReadCloser, string, int64, error) {
	f, err := s.store.GetByID(ctx, id, userID)
	if err != nil {
		return nil, "", 0, err
	}
	if f == nil {
		return nil, "", 0, apierr.ErrNotFound
	}
	return s.storage.GetObject(ctx, f.ObjectKey)
}

const presignTTL = 15 * time.Minute

func (s *service) PresignUpload(ctx context.Context, userID, bucketID uuid.UUID, filename, mimeType string) (*PresignData, error) {
	if !AllowedMIMETypes[mimeType] {
		return nil, apierr.ErrBadRequest(fmt.Sprintf("mime type %q is not allowed", mimeType))
	}
	if err := s.checkQuota(ctx, userID, 0); err != nil {
		return nil, err
	}
	ok, _, err := s.checkBucket(ctx, bucketID, userID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apierr.ErrNotFound
	}
	fileID := uuid.New()
	objectKey := fmt.Sprintf("%s/%s/%s_%s", bucketID, fileID, fileID, filename)
	uploadURL, err := s.storage.PresignPutURL(ctx, objectKey, presignTTL)
	if err != nil {
		return nil, err
	}
	return &PresignData{
		FileID:    fileID,
		ObjectKey: objectKey,
		UploadURL: uploadURL,
		ExpiresIn: int(presignTTL.Seconds()),
	}, nil
}

func (s *service) ConfirmUpload(ctx context.Context, userID, bucketID, fileID uuid.UUID, objectKey, filename, mimeType string, sizeBytes int64) (*File, error) {
	if err := s.checkFileSize(ctx, userID, sizeBytes); err != nil {
		return nil, err
	}
	if err := s.checkQuota(ctx, userID, sizeBytes); err != nil {
		return nil, err
	}
	ok, isPublic, err := s.checkBucket(ctx, bucketID, userID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apierr.ErrNotFound
	}
	f := &File{
		ID:        fileID,
		BucketID:  bucketID,
		ObjectKey: objectKey,
		Filename:  filename,
		MimeType:  mimeType,
		SizeBytes: sizeBytes,
		CreatedAt: time.Now().UTC(),
	}
	if isPublic {
		f.URL = s.publicFileURL(fileID)
	}
	if err := s.store.Create(ctx, f); err != nil {
		return nil, err
	}
	return f, nil
}

func (s *service) Delete(ctx context.Context, userID, id uuid.UUID) error {
	f, err := s.store.GetByID(ctx, id, userID)
	if err != nil {
		return err
	}
	if f == nil {
		return apierr.ErrNotFound
	}
	if err := s.storage.Delete(ctx, f.ObjectKey); err != nil {
		return fmt.Errorf("delete from R2: %w", err)
	}
	return s.store.Delete(ctx, id, userID)
}
