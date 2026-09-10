package usage

import (
	"context"

	"github.com/google/uuid"
)

type Service interface {
	GetSummary(ctx context.Context, userID, projectID uuid.UUID) (*Summary, error)
}

type service struct{ store Store }

func NewService(store Store) Service { return &service{store} }

func (s *service) GetSummary(ctx context.Context, userID, projectID uuid.UUID) (*Summary, error) {
	return s.store.GetSummary(ctx, projectID, userID)
}
