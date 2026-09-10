package logs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ db *pgxpool.Pool }

func NewStore(db *pgxpool.Pool) *Store { return &Store{db} }

// Async writes a log entry in a background goroutine — never blocks the caller.
func (s *Store) Async(userID uuid.UUID, action, resourceType, resourceID string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.db.Exec(ctx,
			`INSERT INTO activity_logs (id, user_id, action, resource_type, resource_id, created_at)
			 VALUES (gen_random_uuid(), $1, $2, $3, $4, now())`,
			userID, action, resourceType, resourceID,
		)
	}()
}

func (s *Store) List(ctx context.Context, f Filter) ([]*Entry, error) {
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}

	where := []string{"1=1"}
	args := []any{}
	i := 1

	if f.UserID != nil {
		where = append(where, fmt.Sprintf("al.user_id = $%d", i))
		args = append(args, *f.UserID)
		i++
	}
	if f.Action != "" {
		where = append(where, fmt.Sprintf("al.action LIKE $%d", i))
		args = append(args, f.Action+"%")
		i++
	}
	args = append(args, limit)

	q := fmt.Sprintf(`
		SELECT al.id, al.user_id, u.name, u.email, u.is_admin, u.is_super_admin,
		       al.action, COALESCE(al.resource_type,''), COALESCE(al.resource_id,''), al.created_at
		FROM activity_logs al
		JOIN users u ON u.id = al.user_id
		WHERE %s
		ORDER BY al.created_at DESC
		LIMIT $%d`,
		strings.Join(where, " AND "), i,
	)

	rows, err := s.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*Entry
	for rows.Next() {
		e := &Entry{}
		rows.Scan(&e.ID, &e.UserID, &e.UserName, &e.UserEmail, &e.IsAdmin, &e.IsSuperAdmin,
			&e.Action, &e.ResourceType, &e.ResourceID, &e.CreatedAt)
		list = append(list, e)
	}
	if list == nil {
		list = []*Entry{}
	}
	return list, rows.Err()
}
