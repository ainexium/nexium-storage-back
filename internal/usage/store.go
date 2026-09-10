package usage

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	GetSummary(ctx context.Context, projectID, userID uuid.UUID) (*Summary, error)
}

type pgStore struct{ db *pgxpool.Pool }

func NewStore(db *pgxpool.Pool) Store { return &pgStore{db} }

func (s *pgStore) GetSummary(ctx context.Context, projectID, userID uuid.UUID) (*Summary, error) {
	sum := &Summary{ProjectID: projectID}
	err := s.db.QueryRow(ctx,
		`SELECT
		    COALESCE(SUM(f.size_bytes), 0),
		    COUNT(DISTINCT f.id),
		    COUNT(DISTINCT b.id)
		 FROM projects p
		 LEFT JOIN buckets b ON b.project_id = p.id
		 LEFT JOIN files f ON f.bucket_id = b.id
		 WHERE p.id = $1 AND p.user_id = $2`,
		projectID, userID,
	).Scan(&sum.StorageBytes, &sum.FileCount, &sum.BucketCount)
	return sum, err
}
