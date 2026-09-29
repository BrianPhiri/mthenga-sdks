package mthenga

import (
	"context"
	"net/http"
	"time"
)

// EmailDomain is a custom sending/receiving domain registered to your org.
// Registering it in mthenga is bookkeeping only — you still verify it
// with your DNS/email provider yourself (see mthenga's PROVIDERS.md).
type EmailDomain struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id"`
	Domain    string    `json:"domain"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateEmailDomain registers domain to your org. Returns *Error with
// StatusCode 409 if it's already registered — to any org, not just yours;
// a domain belongs to exactly one org platform-wide.
func (c *Client) CreateEmailDomain(ctx context.Context, domain string) (*EmailDomain, error) {
	body := struct {
		Domain string `json:"domain"`
	}{Domain: domain}

	var ed EmailDomain
	if err := c.request(ctx, http.MethodPost, "/v1/email-domains", body, &ed, nil); err != nil {
		return nil, err
	}
	return &ed, nil
}

// SetWhatsAppCredentials registers your org's own Meta app
// (bring-your-own-Meta-app), replacing any previously stored credentials.
// Every WhatsApp send/webhook-verification/media-download for your org's
// inboxes uses these from then on instead of the platform default. This
// call is entirely optional — without it, your org's WhatsApp inboxes just
// keep using the platform's own Meta app.
func (c *Client) SetWhatsAppCredentials(ctx context.Context, accessToken, appSecret string) error {
	body := struct {
		AccessToken string `json:"access_token"`
		AppSecret   string `json:"app_secret"`
	}{AccessToken: accessToken, AppSecret: appSecret}

	return c.request(ctx, http.MethodPut, "/v1/whatsapp-credentials", body, nil, nil)
}
