package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"nexium.ai/api/pkg/apierr"
	"nexium.ai/api/pkg/response"
)

// EnforceAPIQuota blocks external API key requests once the user's monthly quota is exceeded.
// The limit comes from the user's active plan; NULL means unlimited.
// Users with no active subscription fall back to the free plan limit.
func EnforceAPIQuota(db *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := GetUserID(r)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}

			month := time.Now().UTC().Format("2006-01")

			var limit *int64
			var count int64
			err := db.QueryRow(r.Context(), `
				SELECT
				    CASE WHEN s.id IS NOT NULL THEN p.max_monthly_api_requests
				         ELSE fp.max_monthly_api_requests
				    END,
				    COALESCE(
				        (SELECT arc.count FROM api_request_counts arc
				         WHERE arc.user_id = $1 AND arc.month = $2),
				        0
				    )
				FROM users u
				CROSS JOIN (SELECT max_monthly_api_requests FROM plans WHERE slug = 'free' LIMIT 1) fp
				LEFT JOIN subscriptions s ON s.user_id = u.id AND s.status = 'active'
				LEFT JOIN plans p ON p.id = s.plan_id
				WHERE u.id = $1
			`, userID, month).Scan(&limit, &count)
			if err != nil {
				// Fail open: infra issues should not block legitimate requests
				next.ServeHTTP(w, r)
				return
			}

			if limit != nil && count >= *limit {
				response.Error(w, apierr.New(429, "monthly API quota exceeded — upgrade your plan"))
				return
			}

			next.ServeHTTP(w, r)

			// Async increment — non-blocking, atomic upsert
			go db.Exec(context.Background(), `
				INSERT INTO api_request_counts (user_id, month, count)
				VALUES ($1, $2, 1)
				ON CONFLICT (user_id, month) DO UPDATE
				SET count = api_request_counts.count + 1
			`, userID, month)
		})
	}
}
