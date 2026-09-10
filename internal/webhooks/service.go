package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"nexium.ai/api/pkg/apierr"
)

const (
	maxAttempts    = 3
	deliveryTimeout = 10 * time.Second
)

type Service interface {
	Create(ctx context.Context, projectID uuid.UUID, url string, events []string) (*Webhook, string, error)
	Get(ctx context.Context, id, projectID uuid.UUID) (*Webhook, error)
	List(ctx context.Context, projectID uuid.UUID) ([]*Webhook, error)
	Update(ctx context.Context, id, projectID uuid.UUID, url string, events []string, isActive bool) (*Webhook, error)
	Delete(ctx context.Context, id, projectID uuid.UUID) error
	ListDeliveries(ctx context.Context, webhookID, projectID uuid.UUID) ([]*Delivery, error)
	Fire(projectID uuid.UUID, event string, payload any)
}

type service struct {
	store Store
}

func NewService(store Store) Service {
	return &service{store: store}
}

func (s *service) Create(ctx context.Context, projectID uuid.UUID, url string, events []string) (*Webhook, string, error) {
	if err := validateURL(url); err != nil {
		return nil, "", err
	}
	if err := validateEvents(events); err != nil {
		return nil, "", err
	}

	secret, err := generateSecret()
	if err != nil {
		return nil, "", err
	}

	now := time.Now().UTC()
	w := &Webhook{
		ID:        uuid.New(),
		ProjectID: projectID,
		URL:       url,
		Events:    events,
		IsActive:  true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.store.Create(ctx, w, secret); err != nil {
		return nil, "", err
	}
	return w, secret, nil
}

func (s *service) Get(ctx context.Context, id, projectID uuid.UUID) (*Webhook, error) {
	return s.store.GetByID(ctx, id, projectID)
}

func (s *service) List(ctx context.Context, projectID uuid.UUID) ([]*Webhook, error) {
	return s.store.ListByProject(ctx, projectID)
}

func (s *service) Update(ctx context.Context, id, projectID uuid.UUID, url string, events []string, isActive bool) (*Webhook, error) {
	if err := validateURL(url); err != nil {
		return nil, err
	}
	if err := validateEvents(events); err != nil {
		return nil, err
	}
	w, err := s.store.Update(ctx, id, projectID, url, events, isActive)
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, apierr.ErrNotFound
	}
	return w, nil
}

func (s *service) Delete(ctx context.Context, id, projectID uuid.UUID) error {
	return s.store.Delete(ctx, id, projectID)
}

func (s *service) ListDeliveries(ctx context.Context, webhookID, projectID uuid.UUID) ([]*Delivery, error) {
	wh, err := s.store.GetByID(ctx, webhookID, projectID)
	if err != nil {
		return nil, err
	}
	if wh == nil {
		return nil, apierr.ErrNotFound
	}
	return s.store.ListDeliveries(ctx, webhookID, 50)
}

// Fire dispatches an event to all active webhooks of the project asynchronously.
func (s *service) Fire(projectID uuid.UUID, event string, payload any) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		hooks, err := s.store.ListActiveByProject(ctx, projectID)
		if err != nil {
			log.Printf("[webhook] failed to list hooks for project %s: %v", projectID, err)
			return
		}

		body := map[string]any{
			"id":         uuid.New().String(),
			"event":      event,
			"project_id": projectID.String(),
			"created_at": time.Now().UTC().Format(time.RFC3339),
			"data":       payload,
		}
		rawPayload, _ := json.Marshal(body)

		for _, hook := range hooks {
			if !slices.Contains(hook.Events, event) {
				continue
			}
			s.deliver(ctx, hook, event, rawPayload)
		}
	}()
}

func (s *service) deliver(ctx context.Context, hook *webhookWithSecret, event string, payload []byte) {
	sig := sign(payload, hook.Secret)

	var (
		statusCode  *int
		success     bool
		errMsg      *string
		deliveredAt *time.Time
		attempts    int
	)

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		attempts = attempt
		code, err := sendHTTP(hook.URL, sig, payload)
		statusCode = &code
		if err == nil && code < 300 {
			now := time.Now().UTC()
			deliveredAt = &now
			success = true
			break
		}
		msg := fmt.Sprintf("attempt %d: status=%d err=%v", attempt, code, err)
		errMsg = &msg
		if attempt < maxAttempts {
			time.Sleep(time.Duration(attempt*attempt) * time.Second) // 1s, 4s
		}
	}

	d := &deliveryRow{
		ID:          uuid.New(),
		WebhookID:   hook.ID,
		Event:       event,
		Payload:     payload,
		StatusCode:  statusCode,
		Success:     success,
		Attempts:    attempts,
		Error:       errMsg,
		DeliveredAt: deliveredAt,
	}
	if err := s.store.SaveDelivery(ctx, d); err != nil {
		log.Printf("[webhook] failed to save delivery for webhook %s: %v", hook.ID, err)
	}
	if success {
		log.Printf("[webhook] delivered event=%s to %s (attempt %d)", event, hook.URL, attempts)
	} else {
		log.Printf("[webhook] failed event=%s to %s after %d attempts", event, hook.URL, attempts)
	}
}

func sendHTTP(url, signature string, payload []byte) (int, error) {
	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nexium-Signature", signature)
	req.Header.Set("User-Agent", "NEXIUM-Storage-Webhook/1.0")

	client := &http.Client{Timeout: deliveryTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}

func sign(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func generateSecret() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "whsec_" + hex.EncodeToString(b), nil
}

func validateURL(url string) error {
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return apierr.ErrBadRequest("url must start with http:// or https://")
	}
	return nil
}

func validateEvents(events []string) error {
	if len(events) == 0 {
		return apierr.ErrBadRequest("at least one event is required")
	}
	for _, e := range events {
		if !slices.Contains(AllEvents, e) {
			return apierr.ErrBadRequest(fmt.Sprintf("unknown event %q", e))
		}
	}
	return nil
}
