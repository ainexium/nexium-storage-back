package middleware

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"nexium.ai/api/pkg/apierr"
	"nexium.ai/api/pkg/response"
)

func RequireSuperAdmin(db *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := GetUserID(r)
			if !ok {
				response.Error(w, apierr.ErrUnauthorized)
				return
			}
			var isSuperAdmin bool
			err := db.QueryRow(r.Context(),
				`SELECT is_super_admin FROM users WHERE id = $1`, userID,
			).Scan(&isSuperAdmin)
			if err != nil || !isSuperAdmin {
				response.Error(w, apierr.ErrForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
