// Package mthenga is a client for mthenga's REST API — a headless,
// programmatic inbox for AI agents over email and WhatsApp. No dependencies
// beyond the standard library, matching mthenga's own hand-rolled-client
// philosophy (see its Postmark/WhatsApp/FusionAuth clients).
//
// This wraps the mt_live_... API-key-authenticated surface only (inboxes,
// messages, threads, webhooks, API keys, settings) — not the separate
// FusionAuth-backed human/dashboard auth (register/login/invites), which
// is a different actor and trust model.
package mthenga

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultTimeout = 15 * time.Second

// Client calls mthenga's API as one organization, authenticated with a
// single mt_live_... API key.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// Option configures a Client at construction time.
type Option func(*Client)

// WithHTTPClient overrides the default http.Client — for a custom
// transport, proxy, or timeout.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// NewClient builds a Client. baseURL is wherever mthenga is deployed
// (e.g. "https://api.yourdomain.com"), no trailing slash required.
func NewClient(baseURL, apiKey string, opts ...Option) *Client {
	c := &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: defaultTimeout},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Error is returned for any non-2xx response. Check StatusCode for the
// specific cases each method documents (a 404 vs a 409, for example).
type Error struct {
	StatusCode int
	Message    string // mthenga's own {"error": "..."} body, when present
	RawBody    string
}

func (e *Error) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("mthenga: %d %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("mthenga: %d %s", e.StatusCode, e.RawBody)
}

// IsStatus reports whether err is a *Error with the given HTTP status —
// e.g. mthenga.IsStatus(err, http.StatusConflict) after CreateEmailDomain
// to detect "domain already registered" without string-matching.
func IsStatus(err error, status int) bool {
	var rbErr *Error
	if errors.As(err, &rbErr) {
		return rbErr.StatusCode == status
	}
	return false
}

// request performs one HTTP call. body is marshaled to JSON if non-nil;
// out is decoded from the JSON response body if non-nil. headers are
// applied after Authorization/Content-Type so a caller can add or (in
// principle) override either.
func (c *Client) request(ctx context.Context, method, path string, body, out any, headers map[string]string) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("mthenga request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		apiErr := &Error{StatusCode: resp.StatusCode, RawBody: string(respBody)}
		var parsed struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(respBody, &parsed) == nil {
			apiErr.Message = parsed.Error
		}
		return apiErr
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}
