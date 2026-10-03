package mthenga

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// Partner API: only for an org the Mthenga operator has made a partner.
// Other keys get a 403 *Error. A partner creates a child org per business
// it serves, then uses the child's own API key with the rest of this
// client (NewClient(baseURL, childKey)) to set up its inboxes, Meta
// credentials, domains and webhooks.

// PartnerOrg is a child org the partner created.
type PartnerOrg struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	ExternalRef string    `json:"external_ref,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// PartnerOrgKey is a child org plus a new raw API key for it, shown once.
type PartnerOrgKey struct {
	Org    PartnerOrg `json:"org"`
	APIKey string     `json:"api_key"`
}

// CreatePartnerOrg creates a child org named name. externalRef is your own
// id for it (e.g. your project id): with one set the call is idempotent —
// repeating it returns the same org rather than a duplicate — and every
// call returns a fresh API key, so a retry after a lost response still
// gets a usable key.
func (c *Client) CreatePartnerOrg(ctx context.Context, name, externalRef string) (*PartnerOrgKey, error) {
	body := struct {
		Name        string `json:"name"`
		ExternalRef string `json:"external_ref,omitempty"`
	}{name, externalRef}
	var resp PartnerOrgKey
	if err := c.request(ctx, http.MethodPost, "/v1/partner/orgs", body, &resp, nil); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListPartnerOrgs lists your child orgs, newest first.
func (c *Client) ListPartnerOrgs(ctx context.Context) ([]PartnerOrg, error) {
	var resp struct {
		Orgs []PartnerOrg `json:"orgs"`
	}
	if err := c.request(ctx, http.MethodGet, "/v1/partner/orgs", nil, &resp, nil); err != nil {
		return nil, err
	}
	return resp.Orgs, nil
}

// CreatePartnerOrgAPIKey mints a new key for one of your child orgs (for
// rotation or a lost key). Revoke the old one with the child's own client:
// RevokeAPIKey.
func (c *Client) CreatePartnerOrgAPIKey(ctx context.Context, orgID string) (*PartnerOrgKey, error) {
	var resp PartnerOrgKey
	if err := c.request(ctx, http.MethodPost, "/v1/partner/orgs/"+url.PathEscape(orgID)+"/api-keys", nil, &resp, nil); err != nil {
		return nil, err
	}
	return &resp, nil
}
