package mthenga

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"
)

// Webhook is a registered delivery target. Secret is only ever populated
// by CreateWebhook and RotateWebhookSecret — ListWebhooks never includes
// it, since mthenga only shows it once, at creation/rotation time.
type Webhook struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id,omitempty"`
	URL       string    `json:"url"`
	Secret    string    `json:"secret,omitempty"`
	IsActive  bool      `json:"is_active"`
	Events    []string  `json:"events"`
	CreatedAt time.Time `json:"created_at"`
}

// WebhookDelivery is one delivery attempt (successful or not) of an event
// to a Webhook.
type WebhookDelivery struct {
	ID              string          `json:"id"`
	WebhookConfigID string          `json:"webhook_config_id"`
	WebhookURL      string          `json:"webhook_url"`
	MessageID       string          `json:"message_id"`
	Event           string          `json:"event"`
	Payload         json.RawMessage `json:"payload"`
	Status          string          `json:"status"` // "pending" | "delivered" | "failed"
	Attempts        int             `json:"attempts"`
	LastError       string          `json:"last_error"`
	CreatedAt       time.Time       `json:"created_at"`
	NextAttemptAt   time.Time       `json:"next_attempt_at"`
	DeliveredAt     time.Time       `json:"delivered_at"` // zero value if never delivered
}

// CreateWebhook registers url to receive signed message.received events.
// The returned Secret is shown exactly once — save it, it's what verifies
// X-Mthenga-Signature via VerifyWebhookSignature.
func (c *Client) CreateWebhook(ctx context.Context, url string) (*Webhook, error) {
	body := struct {
		URL string `json:"url"`
	}{URL: url}

	var webhook Webhook
	if err := c.request(ctx, http.MethodPost, "/v1/webhooks", body, &webhook, nil); err != nil {
		return nil, err
	}
	return &webhook, nil
}

// ListWebhooks lists your org's active webhooks. Never includes secrets.
func (c *Client) ListWebhooks(ctx context.Context) ([]Webhook, error) {
	var webhooks []Webhook
	if err := c.request(ctx, http.MethodGet, "/v1/webhooks", nil, &webhooks, nil); err != nil {
		return nil, err
	}
	return webhooks, nil
}

// DeleteWebhook soft-deactivates a webhook — never a hard delete, so
// delivery history stays visible in ListWebhookDeliveries. Also
// immediately fails any of its deliveries still pending, rather than
// leaving them to the normal retry schedule to notice. Idempotent: an
// unknown or already-deactivated id is not an error.
func (c *Client) DeleteWebhook(ctx context.Context, id string) error {
	return c.request(ctx, http.MethodDelete, "/v1/webhooks/"+id, nil, nil, nil)
}

// RotateWebhookSecret issues a new signing secret for an existing webhook
// (same url/events), without needing to re-register it. The new Secret,
// like CreateWebhook's, is shown exactly once.
func (c *Client) RotateWebhookSecret(ctx context.Context, id string) (*Webhook, error) {
	var webhook Webhook
	if err := c.request(ctx, http.MethodPost, "/v1/webhooks/"+id+"/rotate", nil, &webhook, nil); err != nil {
		return nil, err
	}
	return &webhook, nil
}

// ListWebhookDeliveries returns the most recent 50 delivery attempts
// across all of your org's webhooks, for inspecting whether events are
// actually landing.
func (c *Client) ListWebhookDeliveries(ctx context.Context) ([]WebhookDelivery, error) {
	var deliveries []WebhookDelivery
	if err := c.request(ctx, http.MethodGet, "/v1/webhook-deliveries", nil, &deliveries, nil); err != nil {
		return nil, err
	}
	return deliveries, nil
}

// VerifyWebhookSignature checks the X-Mthenga-Signature header against
// the raw request body (before any JSON parsing — re-serializing a parsed
// body will not reliably match) and the secret from CreateWebhook or
// RotateWebhookSecret. Uses a constant-time comparison.
func VerifyWebhookSignature(secret string, rawBody []byte, signature string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(rawBody)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
