package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	CreateUser(ctx context.Context, u *User) error
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (*User, error)
	UpdateUser(ctx context.Context, id uuid.UUID, name, email, passwordHash string) error
	SetVerified(ctx context.Context, userID uuid.UUID) error
	UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string) error
	SaveRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error
	GetRefreshToken(ctx context.Context, tokenHash string) (uuid.UUID, error)
	DeleteRefreshToken(ctx context.Context, tokenHash string) error
	DeleteUserTokens(ctx context.Context, userID uuid.UUID) error
	CreateVerificationCode(ctx context.Context, userID uuid.UUID, codeHash, purpose string, expiresAt time.Time) error
	GetLatestCode(ctx context.Context, userID uuid.UUID, purpose string) (id uuid.UUID, codeHash string, err error)
	MarkCodeUsed(ctx context.Context, id uuid.UUID) error
	// Quota — retourne -1 si aucun quota custom (utiliser le défaut plateforme), 0 = illimité
	GetStorageQuota(ctx context.Context, userID uuid.UUID) (int64, error)
	SetStorageQuota(ctx context.Context, userID uuid.UUID, quotaBytes *int64) error
	// Activité — réinitialise last_active_at et annule l'avertissement d'inactivité
	TouchActivity(ctx context.Context, userID uuid.UUID) error
	// Changement d'email vérifié
	SetPendingEmail(ctx context.Context, userID uuid.UUID, email string) error
	ClearPendingEmail(ctx context.Context, userID uuid.UUID) error
}

type pgStore struct{ db *pgxpool.Pool }

func NewStore(db *pgxpool.Pool) Store { return &pgStore{db} }

func (s *pgStore) CreateUser(ctx context.Context, u *User) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO users (id, name, email, password_hash, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		u.ID, u.Name, u.Email, u.PasswordHash, u.CreatedAt, u.UpdatedAt,
	)
	return err
}

func (s *pgStore) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	u := &User{}
	err := s.db.QueryRow(ctx,
		`SELECT id, name, email, password_hash, is_admin, is_super_admin, is_verified, storage_quota_bytes, COALESCE(pending_email,''), created_at, updated_at
		 FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Name, &u.Email, &u.PasswordHash, &u.IsAdmin, &u.IsSuperAdmin, &u.IsVerified, &u.storageQuotaBytes, &u.PendingEmail, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return u, err
}

func (s *pgStore) GetUserByID(ctx context.Context, id uuid.UUID) (*User, error) {
	u := &User{}
	err := s.db.QueryRow(ctx,
		`SELECT id, name, email, password_hash, is_admin, is_super_admin, is_verified, storage_quota_bytes, COALESCE(pending_email,''), created_at, updated_at
		 FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Name, &u.Email, &u.PasswordHash, &u.IsAdmin, &u.IsSuperAdmin, &u.IsVerified, &u.storageQuotaBytes, &u.PendingEmail, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return u, err
}

func (s *pgStore) UpdateUser(ctx context.Context, id uuid.UUID, name, email, passwordHash string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE users SET name=$2, email=$3, password_hash=$4, updated_at=now() WHERE id=$1`,
		id, name, email, passwordHash,
	)
	return err
}

func (s *pgStore) SetVerified(ctx context.Context, userID uuid.UUID) error {
	_, err := s.db.Exec(ctx, `UPDATE users SET is_verified = true, updated_at = now() WHERE id = $1`, userID)
	return err
}

func (s *pgStore) UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	_, err := s.db.Exec(ctx, `UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1`, userID, passwordHash)
	return err
}

func (s *pgStore) SaveRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3, $4)`,
		uuid.New(), userID, tokenHash, expiresAt,
	)
	return err
}

func (s *pgStore) GetRefreshToken(ctx context.Context, tokenHash string) (uuid.UUID, error) {
	var userID uuid.UUID
	err := s.db.QueryRow(ctx,
		`SELECT user_id FROM refresh_tokens
		 WHERE token_hash = $1 AND expires_at > now()`,
		tokenHash,
	).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, errors.New("invalid or expired token")
	}
	return userID, err
}

func (s *pgStore) DeleteRefreshToken(ctx context.Context, tokenHash string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM refresh_tokens WHERE token_hash = $1`, tokenHash)
	return err
}

func (s *pgStore) DeleteUserTokens(ctx context.Context, userID uuid.UUID) error {
	_, err := s.db.Exec(ctx, `DELETE FROM refresh_tokens WHERE user_id = $1`, userID)
	return err
}

func (s *pgStore) CreateVerificationCode(ctx context.Context, userID uuid.UUID, codeHash, purpose string, expiresAt time.Time) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO verification_codes (id, user_id, code_hash, purpose, expires_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		uuid.New(), userID, codeHash, purpose, expiresAt,
	)
	return err
}

func (s *pgStore) GetLatestCode(ctx context.Context, userID uuid.UUID, purpose string) (uuid.UUID, string, error) {
	var id uuid.UUID
	var codeHash string
	err := s.db.QueryRow(ctx,
		`SELECT id, code_hash FROM verification_codes
		 WHERE user_id = $1 AND purpose = $2 AND used_at IS NULL AND expires_at > now()
		 ORDER BY created_at DESC LIMIT 1`,
		userID, purpose,
	).Scan(&id, &codeHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", nil
	}
	return id, codeHash, err
}

func (s *pgStore) MarkCodeUsed(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.Exec(ctx, `UPDATE verification_codes SET used_at = now() WHERE id = $1`, id)
	return err
}

func (s *pgStore) GetStorageQuota(ctx context.Context, userID uuid.UUID) (int64, error) {
	var quota *int64
	err := s.db.QueryRow(ctx,
		`SELECT storage_quota_bytes FROM users WHERE id = $1`, userID,
	).Scan(&quota)
	if err != nil {
		return -1, err
	}
	if quota == nil {
		return -1, nil // pas de quota custom → utiliser le défaut plateforme
	}
	return *quota, nil
}

func (s *pgStore) SetStorageQuota(ctx context.Context, userID uuid.UUID, quotaBytes *int64) error {
	_, err := s.db.Exec(ctx,
		`UPDATE users SET storage_quota_bytes = $2, updated_at = now() WHERE id = $1`,
		userID, quotaBytes,
	)
	return err
}

func (s *pgStore) TouchActivity(ctx context.Context, userID uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`UPDATE users SET last_active_at = now(), inactivity_warned_at = NULL WHERE id = $1`,
		userID,
	)
	return err
}

func (s *pgStore) SetPendingEmail(ctx context.Context, userID uuid.UUID, email string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE users SET pending_email = $2, updated_at = now() WHERE id = $1`,
		userID, email,
	)
	return err
}

func (s *pgStore) ClearPendingEmail(ctx context.Context, userID uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`UPDATE users SET pending_email = NULL, updated_at = now() WHERE id = $1`,
		userID,
	)
	return err
}
