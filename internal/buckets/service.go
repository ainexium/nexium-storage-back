package buckets

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"nexium.ai/api/internal/storage"
	"nexium.ai/api/pkg/apierr"
)

var validBucketName = regexp.MustCompile(`^[a-z0-9][a-z0-9\-]{1,61}[a-z0-9]$`)

type Service interface {
	Create(ctx context.Context, userID, projectID uuid.UUID, req *CreateRequest) (*Bucket, error)
	List(ctx context.Context, userID, projectID uuid.UUID) ([]*Bucket, error)
	Update(ctx context.Context, userID, id uuid.UUID, req *UpdateRequest) (*Bucket, error)
	Delete(ctx context.Context, userID, id uuid.UUID) error
}

type service struct {
	store Store
	r2    *storage.R2Client
}

func NewService(store Store, r2 *storage.R2Client) Service { return &service{store, r2} }

func (s *service) Create(ctx context.Context, userID, projectID uuid.UUID, req *CreateRequest) (*Bucket, error) {
	name := strings.ToLower(strings.TrimSpace(req.Name))
	if !validBucketName.MatchString(name) {
		return nil, apierr.ErrBadRequest("bucket name must be 3-63 lowercase alphanumeric chars or hyphens, starting and ending with a letter or digit")
	}

	exists, err := s.store.ExistsByName(ctx, projectID, name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, apierr.ErrConflict("bucket name already exists in this project")
	}

	isPublic := true
	if req.IsPublic != nil {
		isPublic = *req.IsPublic
	}

	b := &Bucket{
		ID:        uuid.New(),
		ProjectID: projectID,
		Name:      name,
		IsPublic:  isPublic,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.store.Create(ctx, b); err != nil {
		return nil, err
	}
	return b, nil
}

func (s *service) List(ctx context.Context, userID, projectID uuid.UUID) ([]*Bucket, error) {
	buckets, err := s.store.ListByProject(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}
	if buckets == nil {
		return []*Bucket{}, nil
	}
	return buckets, nil
}

func (s *service) Update(ctx context.Context, userID, id uuid.UUID, req *UpdateRequest) (*Bucket, error) {
	b, err := s.store.GetByIDAndUser(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, apierr.ErrNotFound
	}

	if req.Name != "" {
		name := strings.ToLower(strings.TrimSpace(req.Name))
		if !validBucketName.MatchString(name) {
			return nil, apierr.ErrBadRequest("bucket name must be 3-63 lowercase alphanumeric chars or hyphens, starting and ending with a letter or digit")
		}
		if err := s.store.Rename(ctx, id, userID, name); err != nil {
			return nil, err
		}
		b.Name = name
	}

	if req.IsPublic != nil {
		if err := s.store.SetPublic(ctx, id, userID, *req.IsPublic); err != nil {
			return nil, err
		}
		b.IsPublic = *req.IsPublic
	}

	return b, nil
}

func (s *service) Delete(ctx context.Context, userID, id uuid.UUID) error {
	b, err := s.store.GetByIDAndUser(ctx, id, userID)
	if err != nil {
		return err
	}
	if b == nil {
		return apierr.ErrNotFound
	}
	_ = s.r2.DeleteByPrefix(ctx, id.String()+"/")
	return s.store.Delete(ctx, id, userID)
}
