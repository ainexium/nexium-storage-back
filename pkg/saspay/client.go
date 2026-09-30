package saspay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const baseURL = "https://api.saspay.me/api/v1"

// Status values returned by SasPay (uppercase). We normalise to lowercase.
const (
	StatusPending    = "PENDING"
	StatusProcessing = "PROCESSING"
	StatusSuccess    = "SUCCESS"
	StatusFailed     = "FAILED"
	StatusCancelled  = "CANCELLED"
	StatusExpired    = "EXPIRED"
)

func NormaliseStatus(s string) string {
	switch strings.ToUpper(s) {
	case StatusSuccess:
		return "completed"
	case StatusFailed, StatusCancelled:
		return "failed"
	case StatusExpired:
		return "expired"
	case StatusProcessing:
		return "processing"
	default:
		return "pending"
	}
}

type Customer struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Phone     string `json:"phone"`
}

type CreateSoftpayInput struct {
	Amount      string   `json:"amount"`   // decimal string e.g. "5000.00"
	Currency    string   `json:"currency"` // XOF, XAF, ...
	Country     string   `json:"country"`  // ISO 2-letter e.g. "BJ"
	Customer    Customer `json:"customer"`
	Network     string   `json:"network"`     // SasPay network code e.g. "mtn_bj"
	Description string   `json:"description"` // optional
}

type Payment struct {
	ID            string `json:"id"`
	Status        string `json:"status"` // PENDING, SUCCESS, FAILED, EXPIRED
	CheckoutURL   string `json:"checkout_url"`
	FailureReason string `json:"failure_reason"`
	Description   string `json:"description"`
}

type Client struct {
	apiKey string
	http   *http.Client
}

func New(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		http:   &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *Client) CreatePayment(ctx context.Context, idempotencyKey string, in CreateSoftpayInput) (*Payment, error) {
	body, _ := json.Marshal(in)
	log.Printf("[saspay] CreatePayment → POST %s/payments/softpay/ body=%s", baseURL, body)

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/payments/softpay/", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	log.Printf("[saspay] CreatePayment ← status=%d body=%s", resp.StatusCode, b)

	if resp.StatusCode >= 300 {
		log.Printf("[saspay] CreatePayment error body=%s", b)
		return nil, fmt.Errorf("saspay %d: %s", resp.StatusCode, extractErrorMsg(b))
	}

	return unmarshalPayment(b)
}

func (c *Client) GetPayment(ctx context.Context, paymentID string) (*Payment, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/payments/"+paymentID+"/verify/", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 300 {
		log.Printf("[saspay] GetPayment error body=%s", b)
		return nil, fmt.Errorf("saspay %d: %s", resp.StatusCode, extractErrorMsg(b))
	}

	return unmarshalPayment(b)
}

// extractErrorMsg tries common error-field patterns used by SasPay/DRF APIs.
func extractErrorMsg(b []byte) string {
	var obj map[string]json.RawMessage
	if json.Unmarshal(b, &obj) != nil {
		if len(b) > 0 {
			return string(b)
		}
		return ""
	}
	for _, key := range []string{"detail", "message", "error", "description"} {
		if raw, ok := obj[key]; ok {
			var s string
			if json.Unmarshal(raw, &s) == nil && s != "" {
				return s
			}
		}
	}
	if raw, ok := obj["non_field_errors"]; ok {
		var arr []string
		if json.Unmarshal(raw, &arr) == nil && len(arr) > 0 {
			return strings.Join(arr, "; ")
		}
	}
	if raw, ok := obj["errors"]; ok {
		var s string
		if json.Unmarshal(raw, &s) == nil && s != "" {
			return s
		}
		var m map[string]json.RawMessage
		if json.Unmarshal(raw, &m) == nil {
			var parts []string
			for _, v := range m {
				var arr []string
				if json.Unmarshal(v, &arr) == nil {
					parts = append(parts, strings.Join(arr, ", "))
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, "; ")
			}
		}
	}
	if raw, ok := obj["data"]; ok {
		var inner map[string]json.RawMessage
		if json.Unmarshal(raw, &inner) == nil {
			for _, key := range []string{"message", "error", "detail"} {
				if v, ok := inner[key]; ok {
					var s string
					if json.Unmarshal(v, &s) == nil && s != "" {
						return s
					}
				}
			}
		}
	}
	return string(b)
}

// unmarshalPayment handles both the wrapped {"success":true,"data":{...}} form
// and any future flat form where fields appear at the root level.
func unmarshalPayment(b []byte) (*Payment, error) {
	var wrapper struct {
		Data *Payment `json:"data"`
	}
	if err := json.Unmarshal(b, &wrapper); err != nil {
		return nil, err
	}
	if wrapper.Data != nil && wrapper.Data.ID != "" {
		return wrapper.Data, nil
	}
	var p Payment
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	return &p, nil
}
