package mthenga

import (
	"context"
	"net/http"
	"time"
)

// Thread is one conversation on an inbox — one email subject thread, or
// one WhatsApp contact's ongoing conversation (WhatsApp has no
// subject-based threading, so a contact has exactly one open thread per
// inbox at a time).
type Thread struct {
	ID            string    `json:"id"`
	InboxID       string    `json:"inbox_id"`
	ContactID     string    `json:"contact_id"`
	Channel       string    `json:"channel"`
	Subject       string    `json:"subject"` // empty for WhatsApp
	Status        string    `json:"status"`  // "open" is the only value mthenga sets today
	LastMessageAt time.Time `json:"last_message_at"`
	CreatedAt     time.Time `json:"created_at"`
}

// SecurityStatus is the prompt-injection shield's verdict on a message.
// Passed:false does NOT mean the message was blocked — mthenga delivers
// it regardless (flag, never block) and expects the caller to decide what
// to do with Flags before letting an LLM act on the message unsupervised.
//
// Scores holds semantic-classifier probabilities (0-1), currently only
// "prompt_injection". Absent (nil) when the server's semantic shield is
// off or was unavailable for this message; a score alone never changes
// Passed or Flags.
type SecurityStatus struct {
	Passed bool               `json:"passed"`
	Flags  []string           `json:"flags"`
	Scores map[string]float64 `json:"scores,omitempty"`
}

// Message is one inbound or outbound message within a Thread. CleanText
// has quoted history/signatures stripped (email) and is passed through
// as-is (WhatsApp, which has no quoting convention) — always screened by
// the injection shield first.
type Message struct {
	ID             string         `json:"id"`
	ThreadID       string         `json:"thread_id"`
	InboxID        string         `json:"inbox_id"`
	Channel        string         `json:"channel"`
	Direction      string         `json:"direction"` // "inbound" | "outbound"
	SenderHandle   string         `json:"sender_handle"`
	SenderName     string         `json:"sender_name"`
	Recipients     []string       `json:"recipients"`
	CleanText      string         `json:"clean_text"`
	RawBody        string         `json:"raw_body"`
	SecurityStatus SecurityStatus `json:"security_status"`
	ExternalID     string         `json:"external_id"` // email Message-ID or WhatsApp wamid
	InReplyTo      string         `json:"in_reply_to"`
	CreatedAt      time.Time      `json:"created_at"`
}

// GetThread fetches one thread by id. Returns *Error with StatusCode 404
// if it doesn't exist or isn't owned by your org.
func (c *Client) GetThread(ctx context.Context, id string) (*Thread, error) {
	var thread Thread
	if err := c.request(ctx, http.MethodGet, "/v1/threads/"+id, nil, &thread, nil); err != nil {
		return nil, err
	}
	return &thread, nil
}

// ListThreadMessages returns every message in a thread, oldest first.
// mthenga has no pagination on this endpoint — fine for typical
// conversation lengths, a real limit for very long threads.
func (c *Client) ListThreadMessages(ctx context.Context, threadID string) ([]Message, error) {
	var messages []Message
	if err := c.request(ctx, http.MethodGet, "/v1/threads/"+threadID+"/messages", nil, &messages, nil); err != nil {
		return nil, err
	}
	return messages, nil
}
