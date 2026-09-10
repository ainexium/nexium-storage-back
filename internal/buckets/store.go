package buckets

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	Create(ctx context.Context, b *Bucket) error
	GetByID(ctx context.Context, id uuid.UUID) (*Bucket, error)
	GetByIDAndUser(ctx context.Context, id, userID uuid.UUID) (*Bucket, error)
	ListByProject(ctx context.Context, projectID, userID uuid.UUID) ([]*Bucket, error)
	ListIDsByProject(ctx context.Context, projectID, userID uuid.UUID) ([]uuid.UUID, error)
	Rename(ctx context.Context, id, userID uuid.UUID, name string) error
	SetPublic(ctx context.Context, id, userID uuid.UUID, isPublic bool) error
	Delete(ctx context.Context, id, userID uuid.UUID) error
	ExistsByName(ctx context.Context, projectID uuid.UUID, name string) (bool, error)
}

type pgStore struct{ db *pgxpool.Pool }

func NewStore(db *pgxpool.Pool) Store { return &pgStore{db} }

func (s *pgStore) Create(ctx context.Context, b *Bucket) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO buckets (id, project_id, name, is_public, created_at) VALUES ($1, $2, $3, $4, $5)`,
		b.ID, b.ProjectID, b.Name, b.IsPublic, b.CreatedAt,
	)
	return err
}

func (s *pgStore) GetByID(ctx context.Context, id uuid.UUID) (*Bucket, error) {
	b := &Bucket{}
	err := s.db.QueryRow(ctx,
		`SELECT id, project_id, name, is_public, created_at FROM buckets WHERE id = $1`, id,
	).Scan(&b.ID, &b.ProjectID, &b.Name, &b.IsPublic, &b.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return b, err
}

func (s *pgStore) GetByIDAndUser(ctx context.Context, id, userID uuid.UUID) (*Bucket, error) {
	b := &Bucket{}
	err := s.db.QueryRow(ctx,
		`SELECT b.id, b.project_id, b.name, b.is_public, b.created_at
		 FROM buckets b
		 JOIN projects p ON p.id = b.project_id
		 WHERE b.id = $1 AND p.user_id = $2`, id, userID,
	).Scan(&b.ID, &b.ProjectID, &b.Name, &b.IsPublic, &b.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return b, err
}

func (s *pgStore) ListByProject(ctx context.Context, projectID, userID uuid.UUID) ([]*Bucket, error) {
	rows, err := s.db.Query(ctx,
		`SELECT b.id, b.project_id, b.name, b.is_public, b.created_at
		 FROM buckets b
		 JOIN projects p ON p.id = b.project_id
		 WHERE b.project_id = $1 AND p.user_id = $2
		 ORDER BY b.created_at DESC`, projectID, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*Bucket
	for rows.Next() {
		b := &Bucket{}
		if err := rows.Scan(&b.ID, &b.ProjectID, &b.Name, &b.IsPublic, &b.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, b)
	}
	return list, rows.Err()
}

func (s *pgStore) Rename(ctx context.Context, id, userID uuid.UUID, name string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE buckets b SET name = $3
		 FROM projects p
		 WHERE b.id = $1 AND b.project_id = p.id AND p.user_id = $2`,
		id, userID, name,
	)
	return err
}

func (s *pgStore) SetPublic(ctx context.Context, id, userID uuid.UUID, isPublic bool) error {
	_, err := s.db.Exec(ctx,
		`UPDATE buckets b SET is_public = $3
		 FROM projects p
		 WHERE b.id = $1 AND b.project_id = p.id AND p.user_id = $2`,
		id, userID, isPublic,
	)
	return err
}

func (s *pgStore) Delete(ctx context.Context, id, userID uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM buckets b
		 USING projects p
		 WHERE b.id = $1 AND b.project_id = p.id AND p.user_id = $2`, id, userID,
	)
	return err
}

func (s *pgStore) ListIDsByProject(ctx context.Context, projectID, userID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.db.Query(ctx,
		`SELECT b.id FROM buckets b
		 JOIN projects p ON p.id = b.project_id
		 WHERE b.project_id = $1 AND p.user_id = $2`, projectID, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		rows.Scan(&id)
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *pgStore) ExistsByName(ctx context.Context, projectID uuid.UUID, name string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM buckets WHERE project_id = $1 AND name = $2)`,
		projectID, name,
	).Scan(&exists)
	return exists, err
}
