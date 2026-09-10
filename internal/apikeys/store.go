package apikeys

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	Create(ctx context.Context, k *APIKey, keyHash string) error
	ListByProject(ctx context.Context, projectID, userID uuid.UUID) ([]*APIKey, error)
	Revoke(ctx context.Context, id, userID uuid.UUID) error
	GetProjectByKeyHash(ctx context.Context, keyHash string) (uuid.UUID, error)
	UpdateLastUsed(ctx context.Context, keyHash string) error
}

type pgStore struct{ db *pgxpool.Pool }

func NewStore(db *pgxpool.Pool) Store { return &pgStore{db} }

func (s *pgStore) Create(ctx context.Context, k *APIKey, keyHash string) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO api_keys (id, project_id, name, prefix, key_hash, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		k.ID, k.ProjectID, k.Name, k.Prefix, keyHash, k.CreatedAt,
	)
	return err
}

func (s *pgStore) ListByProject(ctx context.Context, projectID, userID uuid.UUID) ([]*APIKey, error) {
	rows, err := s.db.Query(ctx,
		`SELECT k.id, k.project_id, k.name, k.prefix, k.last_used_at, k.revoked_at, k.created_at
		 FROM api_keys k
		 JOIN projects p ON p.id = k.project_id
		 WHERE k.project_id = $1 AND p.user_id = $2
		 ORDER BY k.created_at DESC`, projectID, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*APIKey
	for rows.Next() {
		k := &APIKey{}
		if err := rows.Scan(&k.ID, &k.ProjectID, &k.Name, &k.Prefix, &k.LastUsedAt, &k.RevokedAt, &k.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, k)
	}
	return list, rows.Err()
}

func (s *pgStore) Revoke(ctx context.Context, id, userID uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`UPDATE api_keys k SET revoked_at = now()
		 FROM projects p
		 WHERE k.id = $1 AND k.project_id = p.id AND p.user_id = $2 AND k.revoked_at IS NULL`,
		id, userID,
	)
	return err
}

func (s *pgStore) GetProjectByKeyHash(ctx context.Context, keyHash string) (uuid.UUID, error) {
	var projectID uuid.UUID
	err := s.db.QueryRow(ctx,
		`SELECT project_id FROM api_keys
		 WHERE key_hash = $1 AND revoked_at IS NULL`, keyHash,
	).Scan(&projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, errors.New("key not found")
	}
	return projectID, err
}

func (s *pgStore) UpdateLastUsed(ctx context.Context, keyHash string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE api_keys SET last_used_at = $1 WHERE key_hash = $2`,
		time.Now().UTC(), keyHash,
	)
	return err
}
