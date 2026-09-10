package adullam

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

const baseURL = "https://api.adullam.dev"

type Payment struct {
	ID            string     `json:"id"`
	Status        string     `json:"status"` // pending, processing, completed, failed, expired
	Amount        int        `json:"amount"`
	Currency      string     `json:"currency"`
	CustomerPhone string     `json:"customer_phone"`
	Channel       string     `json:"channel"`
	Fees          int        `json:"fees"`
	NetAmount     int        `json:"net_amount"`
	Reference     string     `json:"reference"`
	RedirectURL   string     `json:"redirect_url"`
	FailedReason  string     `json:"failed_reason"`
	CreatedAt     time.Time  `json:"created_at"`
	CompletedAt   *time.Time `json:"completed_at"`
}

type CreatePaymentInput struct {
	Amount        int    `json:"amount"`
	Currency      string `json:"currency"`
	CustomerPhone string `json:"customer_phone"`
	Channel       string `json:"channel"`
	Description   string `json:"description"`
}

type Client struct {
	apiKey string
	http   *http.Client
}

func New(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		http:   &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) CreatePayment(ctx context.Context, idempotencyKey string, in CreatePaymentInput) (*Payment, error) {
	body, _ := json.Marshal(in)
	log.Printf("[adullam] CreatePayment → POST %s/v1/payments body=%s", baseURL, body)
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/v1/payments", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-auth", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-idempotency-key", idempotencyKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	log.Printf("[adullam] CreatePayment ← status=%d body=%s", resp.StatusCode, b)

	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(b, &e)
		return nil, fmt.Errorf("adullam %d: %s", resp.StatusCode, e.Error)
	}

	var p Payment
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (c *Client) GetPayment(ctx context.Context, paymentID string) (*Payment, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/v1/payments/"+paymentID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-auth", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(b, &e)
		return nil, fmt.Errorf("adullam %d: %s", resp.StatusCode, e.Error)
	}

	var p Payment
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	return &p, nil
}
