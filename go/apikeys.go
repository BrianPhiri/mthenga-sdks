package mthenga

import (
	"context"
	"net/http"
	"time"
)

// APIKey never carries the raw key or its hash — Key (below) only appears
// on CreateAPIKey's response, once.
type APIKey struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	KeyPrefix  string    `json:"key_prefix"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"` // zero value if never used
	RevokedAt  time.Time `json:"revoked_at"`   // zero value if not revoked
}

// CreateAPIKeyResponse embeds the new key's metadata plus the raw key,
// shown exactly once — every other endpoint (including ListAPIKeys)
// omits it and the underlying hash entirely.
type CreateAPIKeyResponse struct {
	APIKey
	Key string `json:"key"`
}

// CreateAPIKey mints another mt_live_... key for your org — requires the
// Client already be authenticated with a valid key (self-service
// rotation/expansion, not how a brand-new org gets its first key; that's
// cmd/seed or the dashboard's own register flow on mthenga's side).
func (c *Client) CreateAPIKey(ctx context.Context, name string) (*CreateAPIKeyResponse, error) {
	body := struct {
		Name string `json:"name"`
	}{Name: name}

	var resp CreateAPIKeyResponse
	if err := c.request(ctx, http.MethodPost, "/v1/api-keys", body, &resp, nil); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListAPIKeys lists every key for your org, active and revoked, newest
// first.
func (c *Client) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	var keys []APIKey
	if err := c.request(ctx, http.MethodGet, "/v1/api-keys", nil, &keys, nil); err != nil {
		return nil, err
	}
	return keys, nil
}

// RevokeAPIKey invalidates a key immediately and irreversibly — any
// request using it gets 401 from that point on. Idempotent: an unknown or
// already-revoked id is not an error.
func (c *Client) RevokeAPIKey(ctx context.Context, id string) error {
	return c.request(ctx, http.MethodDelete, "/v1/api-keys/"+id, nil, nil, nil)
}
