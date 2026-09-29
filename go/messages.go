package mthenga

import (
	"context"
	"net/http"
)

// SendMessageRequest sends outbound mail or WhatsApp — dispatched
// automatically by which channel the target inbox/thread belongs to. Set
// ThreadID to reply within an existing conversation (Subject is ignored —
// mthenga reuses "Re: " + the original subject for email), or set
// InboxID + To to start a new one (Subject required for a new email
// thread, ignored for WhatsApp).
type SendMessageRequest struct {
	ThreadID string `json:"thread_id,omitempty"`
	InboxID  string `json:"inbox_id,omitempty"`
	To       string `json:"to,omitempty"`
	Subject  string `json:"subject,omitempty"`
	Text     string `json:"text"`

	// IdempotencyKey, if set, is sent as the Idempotency-Key header — a
	// retried request with the same key (same org, same key, same body)
	// replays the original response instead of sending again. Reusing the
	// key with a different body gets *Error{StatusCode: 422}; two genuinely
	// concurrent requests with the same key give the loser *Error{StatusCode: 409}.
	IdempotencyKey string `json:"-"`
}

// SendMessageResponse is almost always just the sent Message. Warning is
// only ever set in one rare case: the send itself succeeded but mthenga
// failed to persist the message row afterward — Message will be its zero
// value then, and the send should NOT be retried (retrying would send a
// duplicate; mthenga reports this as 201, not an error, for exactly this
// reason).
type SendMessageResponse struct {
	Message
	Warning string `json:"warning,omitempty"`
}

// SendMessage sends via POST /v1/messages. Returns *Error with
// StatusCode 400 for a malformed request (e.g. missing subject on a new
// email thread), 404 if the thread/inbox doesn't exist or isn't yours,
// 422 for idempotency-key misuse (same key, different body), 409 if a
// concurrent request with the same idempotency key won the race, or 502
// if the send itself failed at the provider (Postmark/Meta) — safe to
// retry a 502.
func (c *Client) SendMessage(ctx context.Context, req SendMessageRequest) (*SendMessageResponse, error) {
	var headers map[string]string
	if req.IdempotencyKey != "" {
		headers = map[string]string{"Idempotency-Key": req.IdempotencyKey}
	}

	var resp SendMessageResponse
	if err := c.request(ctx, http.MethodPost, "/v1/messages", req, &resp, headers); err != nil {
		return nil, err
	}
	return &resp, nil
}
