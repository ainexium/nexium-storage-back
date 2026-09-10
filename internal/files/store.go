package files

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	Create(ctx context.Context, f *File) error
	GetByID(ctx context.Context, id, userID uuid.UUID) (*File, error)
	GetByIDPublic(ctx context.Context, id uuid.UUID) (*File, error)
	ListByBucket(ctx context.Context, bucketID, userID uuid.UUID, search string, limit, offset int) ([]*File, error)
	CountByBucket(ctx context.Context, bucketID, userID uuid.UUID, search string) (int, error)
	Rename(ctx context.Context, id, userID uuid.UUID, filename string) error
	ListObjectKeysByBucket(ctx context.Context, bucketID uuid.UUID) ([]string, error)
	Delete(ctx context.Context, id, userID uuid.UUID) error
	TotalStorageBytes(ctx context.Context) (int64, error)
	TotalStorageBytesByUser(ctx context.Context, userID uuid.UUID) (int64, error)
}

type pgStore struct{ db *pgxpool.Pool }

func NewStore(db *pgxpool.Pool) Store { return &pgStore{db} }

func (s *pgStore) Create(ctx context.Context, f *File) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO files (id, bucket_id, object_key, filename, mime_type, size_bytes, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		f.ID, f.BucketID, f.ObjectKey, f.Filename, f.MimeType, f.SizeBytes, f.CreatedAt,
	)
	return err
}

// GetByIDPublic récupère un fichier sans vérification d'ownership.
// Utilisé uniquement pour l'endpoint public — vérifie is_public côté service.
func (s *pgStore) GetByIDPublic(ctx context.Context, id uuid.UUID) (*File, error) {
	f := &File{}
	err := s.db.QueryRow(ctx,
		`SELECT f.id, f.bucket_id, f.object_key, f.filename, f.mime_type, f.size_bytes, f.created_at, b.is_public
		 FROM files f
		 JOIN buckets b ON b.id = f.bucket_id
		 WHERE f.id = $1`, id,
	).Scan(&f.ID, &f.BucketID, &f.ObjectKey, &f.Filename, &f.MimeType, &f.SizeBytes, &f.CreatedAt, &f.BucketIsPublic)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return f, err
}

func (s *pgStore) GetByID(ctx context.Context, id, userID uuid.UUID) (*File, error) {
	f := &File{}
	err := s.db.QueryRow(ctx,
		`SELECT f.id, f.bucket_id, f.object_key, f.filename, f.mime_type, f.size_bytes, f.created_at, b.is_public
		 FROM files f
		 JOIN buckets b ON b.id = f.bucket_id
		 JOIN projects p ON p.id = b.project_id
		 WHERE f.id = $1 AND p.user_id = $2`, id, userID,
	).Scan(&f.ID, &f.BucketID, &f.ObjectKey, &f.Filename, &f.MimeType, &f.SizeBytes, &f.CreatedAt, &f.BucketIsPublic)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return f, err
}

func (s *pgStore) ListByBucket(ctx context.Context, bucketID, userID uuid.UUID, search string, limit, offset int) ([]*File, error) {
	rows, err := s.db.Query(ctx,
		`SELECT f.id, f.bucket_id, f.object_key, f.filename, f.mime_type, f.size_bytes, f.created_at, b.is_public
		 FROM files f
		 JOIN buckets b ON b.id = f.bucket_id
		 JOIN projects p ON p.id = b.project_id
		 WHERE f.bucket_id = $1 AND p.user_id = $2
		   AND ($3 = '' OR f.filename ILIKE '%' || $3 || '%')
		 ORDER BY f.created_at DESC
		 LIMIT $4 OFFSET $5`, bucketID, userID, search, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*File
	for rows.Next() {
		f := &File{}
		if err := rows.Scan(&f.ID, &f.BucketID, &f.ObjectKey, &f.Filename, &f.MimeType, &f.SizeBytes, &f.CreatedAt, &f.BucketIsPublic); err != nil {
			return nil, err
		}
		list = append(list, f)
	}
	return list, rows.Err()
}

func (s *pgStore) CountByBucket(ctx context.Context, bucketID, userID uuid.UUID, search string) (int, error) {
	var count int
	err := s.db.QueryRow(ctx,
		`SELECT COUNT(*)
		 FROM files f
		 JOIN buckets b ON b.id = f.bucket_id
		 JOIN projects p ON p.id = b.project_id
		 WHERE f.bucket_id = $1 AND p.user_id = $2
		   AND ($3 = '' OR f.filename ILIKE '%' || $3 || '%')`,
		bucketID, userID, search,
	).Scan(&count)
	return count, err
}

func (s *pgStore) Rename(ctx context.Context, id, userID uuid.UUID, filename string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE files f SET filename = $3
		 FROM buckets b
		 JOIN projects p ON p.id = b.project_id
		 WHERE f.id = $1 AND f.bucket_id = b.id AND p.user_id = $2`,
		id, userID, filename,
	)
	return err
}

func (s *pgStore) ListObjectKeysByBucket(ctx context.Context, bucketID uuid.UUID) ([]string, error) {
	rows, err := s.db.Query(ctx,
		`SELECT object_key FROM files WHERE bucket_id = $1`, bucketID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k string
		rows.Scan(&k)
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *pgStore) TotalStorageBytes(ctx context.Context) (int64, error) {
	var total int64
	err := s.db.QueryRow(ctx, `SELECT COALESCE(SUM(size_bytes), 0) FROM files`).Scan(&total)
	return total, err
}

func (s *pgStore) TotalStorageBytesByUser(ctx context.Context, userID uuid.UUID) (int64, error) {
	var total int64
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(f.size_bytes), 0)
		FROM files f
		JOIN buckets b ON b.id = f.bucket_id
		JOIN projects p ON p.id = b.project_id
		WHERE p.user_id = $1
	`, userID).Scan(&total)
	return total, err
}

func (s *pgStore) Delete(ctx context.Context, id, userID uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM files f
		 USING buckets b, projects p
		 WHERE f.id = $1 AND f.bucket_id = b.id AND b.project_id = p.id AND p.user_id = $2`,
		id, userID,
	)
	return err
}
