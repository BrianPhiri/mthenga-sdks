package mthenga

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// EmailDomain is a custom sending domain registered to your org.
//
// With Postmark automation on (the operator set POSTMARK_ACCOUNT_TOKEN),
// Status starts "pending" and the DKIM/ReturnPath fields hold the DNS
// records to publish: a TXT record at DKIMHost with DKIMValue (required to
// send) and a CNAME at ReturnPathHost pointing to ReturnPathValue
// (recommended). Status becomes "verified" once Postmark sees DKIM, or
// "failed" if it never does (VerifyEmailDomain can still recover it).
// Without automation Status is "manual": the operator verifies it and the
// record fields are empty. Only "verified" and "manual" domains may send.
type EmailDomain struct {
	ID                 string     `json:"id"`
	OrgID              string     `json:"org_id"`
	Domain             string     `json:"domain"`
	CreatedAt          time.Time  `json:"created_at"`
	Status             string     `json:"status"` // "pending", "verified", "failed" or "manual"
	DKIMHost           string     `json:"dkim_host"`
	DKIMValue          string     `json:"dkim_value"`
	ReturnPathHost     string     `json:"return_path_host"`
	ReturnPathValue    string     `json:"return_path_value"`
	DKIMVerified       bool       `json:"dkim_verified"`
	ReturnPathVerified bool       `json:"return_path_verified"`
	LastCheckedAt      *time.Time `json:"last_checked_at"`
}

// CanSend reports whether inboxes on the domain may send.
func (d *EmailDomain) CanSend() bool { return d.Status == "verified" || d.Status == "manual" }

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

// ListEmailDomains returns your org's domains with their status and DNS records.
func (c *Client) ListEmailDomains(ctx context.Context) ([]EmailDomain, error) {
	var out []EmailDomain
	if err := c.request(ctx, http.MethodGet, "/v1/email-domains", nil, &out, nil); err != nil {
		return nil, err
	}
	return out, nil
}

// VerifyEmailDomain asks mthenga to check the domain's DNS records now
// instead of waiting for its background re-check, and returns the result.
// Returns *Error with StatusCode 404 if the domain isn't your org's.
func (c *Client) VerifyEmailDomain(ctx context.Context, id string) (*EmailDomain, error) {
	var ed EmailDomain
	if err := c.request(ctx, http.MethodPost, "/v1/email-domains/"+url.PathEscape(id)+"/verify", nil, &ed, nil); err != nil {
		return nil, err
	}
	return &ed, nil
}

// DeleteEmailDomain removes a domain. Returns *Error with StatusCode 409
// while inboxes still use it, 404 if it isn't your org's.
func (c *Client) DeleteEmailDomain(ctx context.Context, id string) error {
	return c.request(ctx, http.MethodDelete, "/v1/email-domains/"+url.PathEscape(id), nil, nil, nil)
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
