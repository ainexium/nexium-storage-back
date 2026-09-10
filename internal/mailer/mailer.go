package mailer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

type Mailer struct {
	apiKey    string
	secretKey string
	fromEmail string
	fromName  string
}

func New(apiKey, secretKey, fromEmail, fromName string) *Mailer {
	return &Mailer{apiKey: apiKey, secretKey: secretKey, fromEmail: fromEmail, fromName: fromName}
}

func (m *Mailer) Send(ctx context.Context, toEmail, toName, subject, htmlBody string) error {
	if m.apiKey == "" {
		log.Printf("[mailer] SKIPPED (no API key) — to=%s subject=%q", toEmail, subject)
		return nil
	}
	payload := map[string]any{
		"Messages": []map[string]any{
			{
				"From": map[string]string{"Email": m.fromEmail, "Name": m.fromName},
				"To":   []map[string]string{{"Email": toEmail, "Name": toName}},
				"Subject":  subject,
				"HTMLPart": htmlBody,
			},
		},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.mailjet.com/v3.1/send", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.SetBasicAuth(m.apiKey, m.secretKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[mailer] ERROR sending to=%s: %v", toEmail, err)
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		log.Printf("[mailer] ERROR to=%s status=%d body=%s", toEmail, resp.StatusCode, string(b))
		return fmt.Errorf("mailjet %d: %s", resp.StatusCode, string(b))
	}
	log.Printf("[mailer] OK to=%s subject=%q status=%d", toEmail, subject, resp.StatusCode)
	return nil
}

// SendWithAttachment envoie un email avec une pièce jointe (ex: reçu PDF).
func (m *Mailer) SendWithAttachment(ctx context.Context, toEmail, toName, subject, htmlBody, filename, contentType string, data []byte) error {
	if m.apiKey == "" {
		log.Printf("[mailer] SKIPPED (no API key) — to=%s subject=%q", toEmail, subject)
		return nil
	}
	payload := map[string]any{
		"Messages": []map[string]any{
			{
				"From": map[string]string{"Email": m.fromEmail, "Name": m.fromName},
				"To":   []map[string]string{{"Email": toEmail, "Name": toName}},
				"Subject":  subject,
				"HTMLPart": htmlBody,
				"Attachments": []map[string]string{
					{
						"ContentType":   contentType,
						"Filename":      filename,
						"Base64Content": base64.StdEncoding.EncodeToString(data),
					},
				},
			},
		},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.mailjet.com/v3.1/send", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.SetBasicAuth(m.apiKey, m.secretKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[mailer] ERROR sending to=%s: %v", toEmail, err)
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		log.Printf("[mailer] ERROR to=%s status=%d body=%s", toEmail, resp.StatusCode, string(b))
		return fmt.Errorf("mailjet %d: %s", resp.StatusCode, string(b))
	}
	log.Printf("[mailer] OK (with attachment) to=%s subject=%q", toEmail, subject)
	return nil
}
