package projects

import (
	"context"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"nexium.ai/api/internal/storage"
	"nexium.ai/api/pkg/apierr"
)

type ListBucketIDs func(ctx context.Context, projectID, userID uuid.UUID) ([]uuid.UUID, error)

type Service interface {
	Create(ctx context.Context, userID uuid.UUID, req *CreateRequest) (*Project, error)
	GetByID(ctx context.Context, userID, id uuid.UUID) (*Project, error)
	List(ctx context.Context, userID uuid.UUID) ([]*Project, error)
	Update(ctx context.Context, userID, id uuid.UUID, req *UpdateRequest) (*Project, error)
	Delete(ctx context.Context, userID, id uuid.UUID) error
}

type service struct {
	store        Store
	r2           *storage.R2Client
	listBucketIDs ListBucketIDs
}

func NewService(store Store, r2 *storage.R2Client, listBucketIDs ListBucketIDs) Service {
	return &service{store, r2, listBucketIDs}
}

func (s *service) Create(ctx context.Context, userID uuid.UUID, req *CreateRequest) (*Project, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, apierr.ErrBadRequest("project name is required")
	}
	now := time.Now().UTC()
	p := &Project{
		ID:        uuid.New(),
		UserID:    userID,
		Name:      name,
		Slug:      slugify(name),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.store.Create(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *service) GetByID(ctx context.Context, userID, id uuid.UUID) (*Project, error) {
	p, err := s.store.GetByID(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, apierr.ErrNotFound
	}
	return p, nil
}

func (s *service) List(ctx context.Context, userID uuid.UUID) ([]*Project, error) {
	projects, err := s.store.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if projects == nil {
		return []*Project{}, nil
	}
	return projects, nil
}

func (s *service) Update(ctx context.Context, userID, id uuid.UUID, req *UpdateRequest) (*Project, error) {
	p, err := s.store.GetByID(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, apierr.ErrNotFound
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, apierr.ErrBadRequest("project name is required")
	}
	p.Name = name
	p.Slug = slugify(name)
	p.UpdatedAt = time.Now().UTC()
	if err := s.store.Update(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *service) Delete(ctx context.Context, userID, id uuid.UUID) error {
	p, err := s.store.GetByID(ctx, id, userID)
	if err != nil {
		return err
	}
	if p == nil {
		return apierr.ErrNotFound
	}
	// Delete all R2 objects across all buckets before removing DB records
	if bucketIDs, err := s.listBucketIDs(ctx, id, userID); err == nil {
		for _, bid := range bucketIDs {
			_ = s.r2.DeleteByPrefix(ctx, bid.String()+"/")
		}
	}
	return s.store.Delete(ctx, id, userID)
}

func slugify(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else if unicode.IsSpace(r) || r == '-' || r == '_' {
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
