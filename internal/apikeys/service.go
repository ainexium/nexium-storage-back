package apikeys

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"nexium.ai/api/pkg/apierr"
)

type Service interface {
	Create(ctx context.Context, userID, projectID uuid.UUID, req *CreateRequest) (*CreateResponse, error)
	List(ctx context.Context, userID, projectID uuid.UUID) ([]*APIKey, error)
	Revoke(ctx context.Context, userID, id uuid.UUID) error
	ValidateKey(ctx context.Context, keyHash string) (uuid.UUID, error)
}

type service struct{ store Store }

func NewService(store Store) Service { return &service{store} }

func (s *service) Create(ctx context.Context, userID, projectID uuid.UUID, req *CreateRequest) (*CreateResponse, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, apierr.ErrBadRequest("api key name is required")
	}

	raw, err := generateAPIKey()
	if err != nil {
		return nil, err
	}

	hash := sha256.Sum256([]byte(raw))
	keyHash := hex.EncodeToString(hash[:])
	prefix := raw[:len("nx_live_")+8]

	key := &APIKey{
		ID:        uuid.New(),
		ProjectID: projectID,
		Name:      name,
		Prefix:    prefix,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.store.Create(ctx, key, keyHash); err != nil {
		return nil, err
	}

	return &CreateResponse{APIKey: key, FullKey: raw}, nil
}

func (s *service) List(ctx context.Context, userID, projectID uuid.UUID) ([]*APIKey, error) {
	keys, err := s.store.ListByProject(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}
	if keys == nil {
		return []*APIKey{}, nil
	}
	return keys, nil
}

func (s *service) Revoke(ctx context.Context, userID, id uuid.UUID) error {
	return s.store.Revoke(ctx, id, userID)
}

func (s *service) ValidateKey(ctx context.Context, keyHash string) (uuid.UUID, error) {
	projectID, err := s.store.GetProjectByKeyHash(ctx, keyHash)
	if err != nil {
		return uuid.Nil, apierr.ErrUnauthorized
	}
	go func() {
		_ = s.store.UpdateLastUsed(context.Background(), keyHash)
	}()
	return projectID, nil
}

func generateAPIKey() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}
	return "nx_live_" + hex.EncodeToString(b), nil
}
