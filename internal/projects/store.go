package projects

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	Create(ctx context.Context, p *Project) error
	GetByID(ctx context.Context, id, userID uuid.UUID) (*Project, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]*Project, error)
	Update(ctx context.Context, p *Project) error
	Delete(ctx context.Context, id, userID uuid.UUID) error
}

type pgStore struct{ db *pgxpool.Pool }

func NewStore(db *pgxpool.Pool) Store { return &pgStore{db} }

func (s *pgStore) Create(ctx context.Context, p *Project) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO projects (id, user_id, name, slug, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		p.ID, p.UserID, p.Name, p.Slug, p.CreatedAt, p.UpdatedAt,
	)
	return err
}

func (s *pgStore) GetByID(ctx context.Context, id, userID uuid.UUID) (*Project, error) {
	p := &Project{}
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, name, slug, created_at, updated_at
		 FROM projects WHERE id = $1 AND user_id = $2`, id, userID,
	).Scan(&p.ID, &p.UserID, &p.Name, &p.Slug, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return p, err
}

func (s *pgStore) ListByUser(ctx context.Context, userID uuid.UUID) ([]*Project, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, user_id, name, slug, created_at, updated_at
		 FROM projects WHERE user_id = $1 ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*Project
	for rows.Next() {
		p := &Project{}
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.Slug, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

func (s *pgStore) Update(ctx context.Context, p *Project) error {
	_, err := s.db.Exec(ctx,
		`UPDATE projects SET name = $1, slug = $2, updated_at = $3
		 WHERE id = $4 AND user_id = $5`,
		p.Name, p.Slug, p.UpdatedAt, p.ID, p.UserID,
	)
	return err
}

func (s *pgStore) Delete(ctx context.Context, id, userID uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM projects WHERE id = $1 AND user_id = $2`, id, userID,
	)
	return err
}
