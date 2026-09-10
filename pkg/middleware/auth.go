package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"nexium.ai/api/pkg/apierr"
	"nexium.ai/api/pkg/response"
)

type contextKey string

const (
	UserIDKey    contextKey = "user_id"
	ProjectIDKey contextKey = "project_id"
)

func Authenticate(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				response.Error(w, apierr.ErrUnauthorized)
				return
			}
			tokenStr := strings.TrimPrefix(header, "Bearer ")

			token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, apierr.ErrUnauthorized
				}
				return []byte(jwtSecret), nil
			})
			if err != nil || !token.Valid {
				response.Error(w, apierr.ErrUnauthorized)
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				response.Error(w, apierr.ErrUnauthorized)
				return
			}

			sub, ok := claims["sub"].(string)
			if !ok {
				response.Error(w, apierr.ErrUnauthorized)
				return
			}

			userID, err := uuid.Parse(sub)
			if err != nil {
				response.Error(w, apierr.ErrUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), UserIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AuthenticateAPIKey resolves userID + projectID from an API key (nx_live_...).
func AuthenticateAPIKey(db *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				response.Error(w, apierr.ErrUnauthorized)
				return
			}
			key := strings.TrimPrefix(header, "Bearer ")
			hash := sha256hex(key)

			var userID, projectID uuid.UUID
			err := db.QueryRow(r.Context(), `
				SELECT p.user_id, p.id
				FROM api_keys k
				JOIN projects p ON p.id = k.project_id
				WHERE k.key_hash = $1 AND k.revoked_at IS NULL
			`, hash).Scan(&userID, &projectID)
			if err != nil {
				response.Error(w, apierr.ErrUnauthorized)
				return
			}

			go db.Exec(context.Background(),
				`UPDATE api_keys SET last_used_at = now() WHERE key_hash = $1`, hash)

			ctx := context.WithValue(r.Context(), UserIDKey, userID)
			ctx = context.WithValue(ctx, ProjectIDKey, projectID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetUserID(r *http.Request) (uuid.UUID, bool) {
	id, ok := r.Context().Value(UserIDKey).(uuid.UUID)
	return id, ok
}

// TrackActivity met à jour last_active_at de façon asynchrone après chaque requête authentifiée.
func TrackActivity(db *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
			if userID, ok := GetUserID(r); ok {
				go db.Exec(context.Background(),
					`UPDATE users SET last_active_at = now(), inactivity_warned_at = NULL WHERE id = $1`,
					userID,
				)
			}
		})
	}
}

func GetProjectID(r *http.Request) (uuid.UUID, bool) {
	id, ok := r.Context().Value(ProjectIDKey).(uuid.UUID)
	return id, ok
}

func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
