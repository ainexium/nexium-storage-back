package webhooks

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	Create(ctx context.Context, w *Webhook, secret string) error
	GetByID(ctx context.Context, id, projectID uuid.UUID) (*Webhook, error)
	ListByProject(ctx context.Context, projectID uuid.UUID) ([]*Webhook, error)
	Update(ctx context.Context, id, projectID uuid.UUID, url string, events []string, isActive bool) (*Webhook, error)
	Delete(ctx context.Context, id, projectID uuid.UUID) error
	ListActiveByProject(ctx context.Context, projectID uuid.UUID) ([]*webhookWithSecret, error)
	SaveDelivery(ctx context.Context, d *deliveryRow) error
	ListDeliveries(ctx context.Context, webhookID uuid.UUID, limit int) ([]*Delivery, error)
}

type webhookWithSecret struct {
	Webhook
	Secret string
}

type deliveryRow struct {
	ID          uuid.UUID
	WebhookID   uuid.UUID
	Event       string
	Payload     []byte
	StatusCode  *int
	Success     bool
	Attempts    int
	Error       *string
	DeliveredAt *time.Time
}

type pgStore struct{ db *pgxpool.Pool }

func NewStore(db *pgxpool.Pool) Store { return &pgStore{db} }

func (s *pgStore) Create(ctx context.Context, w *Webhook, secret string) error {
	return s.db.QueryRow(ctx,
		`INSERT INTO webhooks (id, project_id, url, secret, events, is_active, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, created_at, updated_at`,
		w.ID, w.ProjectID, w.URL, secret, w.Events, w.IsActive, w.CreatedAt, w.UpdatedAt,
	).Scan(&w.ID, &w.CreatedAt, &w.UpdatedAt)
}

func (s *pgStore) GetByID(ctx context.Context, id, projectID uuid.UUID) (*Webhook, error) {
	w := &Webhook{}
	err := s.db.QueryRow(ctx,
		`SELECT id, project_id, url, events, is_active, created_at, updated_at
		 FROM webhooks WHERE id = $1 AND project_id = $2`,
		id, projectID,
	).Scan(&w.ID, &w.ProjectID, &w.URL, &w.Events, &w.IsActive, &w.CreatedAt, &w.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return w, err
}

func (s *pgStore) ListByProject(ctx context.Context, projectID uuid.UUID) ([]*Webhook, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, project_id, url, events, is_active, created_at, updated_at
		 FROM webhooks WHERE project_id = $1 ORDER BY created_at DESC`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*Webhook
	for rows.Next() {
		w := &Webhook{}
		if err := rows.Scan(&w.ID, &w.ProjectID, &w.URL, &w.Events, &w.IsActive, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, w)
	}
	if list == nil {
		list = []*Webhook{}
	}
	return list, rows.Err()
}

func (s *pgStore) Update(ctx context.Context, id, projectID uuid.UUID, url string, events []string, isActive bool) (*Webhook, error) {
	w := &Webhook{}
	err := s.db.QueryRow(ctx,
		`UPDATE webhooks SET url=$3, events=$4, is_active=$5, updated_at=now()
		 WHERE id=$1 AND project_id=$2
		 RETURNING id, project_id, url, events, is_active, created_at, updated_at`,
		id, projectID, url, events, isActive,
	).Scan(&w.ID, &w.ProjectID, &w.URL, &w.Events, &w.IsActive, &w.CreatedAt, &w.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return w, err
}

func (s *pgStore) Delete(ctx context.Context, id, projectID uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM webhooks WHERE id = $1 AND project_id = $2`, id, projectID,
	)
	return err
}

func (s *pgStore) ListActiveByProject(ctx context.Context, projectID uuid.UUID) ([]*webhookWithSecret, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, project_id, url, secret, events, is_active, created_at, updated_at
		 FROM webhooks WHERE project_id = $1 AND is_active = true`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*webhookWithSecret
	for rows.Next() {
		w := &webhookWithSecret{}
		if err := rows.Scan(&w.ID, &w.ProjectID, &w.URL, &w.Secret, &w.Events, &w.IsActive, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, w)
	}
	return list, rows.Err()
}

func (s *pgStore) SaveDelivery(ctx context.Context, d *deliveryRow) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO webhook_deliveries
		 (id, webhook_id, event, payload, status_code, success, attempts, error, delivered_at, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,now())`,
		d.ID, d.WebhookID, d.Event, d.Payload, d.StatusCode, d.Success, d.Attempts, d.Error, d.DeliveredAt,
	)
	return err
}

func (s *pgStore) ListDeliveries(ctx context.Context, webhookID uuid.UUID, limit int) ([]*Delivery, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.Query(ctx,
		`SELECT id, webhook_id, event, status_code, success, attempts, error, delivered_at, created_at
		 FROM webhook_deliveries WHERE webhook_id = $1 ORDER BY created_at DESC LIMIT $2`,
		webhookID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*Delivery
	for rows.Next() {
		d := &Delivery{}
		if err := rows.Scan(&d.ID, &d.WebhookID, &d.Event, &d.StatusCode, &d.Success, &d.Attempts, &d.Error, &d.DeliveredAt, &d.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	if list == nil {
		list = []*Delivery{}
	}
	return list, rows.Err()
}
