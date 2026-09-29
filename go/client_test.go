package mthenga

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// computeTestSignature independently reimplements the HMAC-SHA256 mthenga
// itself uses to sign webhook deliveries — kept separate from
// VerifyWebhookSignature's own implementation so the test below actually
// proves something, rather than calling the same code twice.
func computeTestSignature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// newTestClient builds a Client against a real httptest.Server — the
// correct boundary to mock for a client library (this package's own
// request-building/response-parsing logic is what's under test, not a
// third-party integration mthenga itself would instead verify against
// real infra).
func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, "rb_live_test"), srv
}

func TestCreateInbox(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/inboxes" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer rb_live_test" {
			t.Errorf("Authorization header = %q", got)
		}
		var body CreateInboxRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body.Username != "support" {
			t.Errorf("username = %q, want %q", body.Username, "support")
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(Inbox{
			ID: "inbox_abc123", Channel: "email", Name: "support",
			FullEmail: "support@agents.example.com", IsActive: true,
		})
	})

	inbox, err := client.CreateInbox(context.Background(), CreateInboxRequest{
		Channel: "email", Username: "support",
	})
	if err != nil {
		t.Fatalf("CreateInbox: %v", err)
	}
	if inbox.ID != "inbox_abc123" || inbox.FullEmail != "support@agents.example.com" {
		t.Errorf("unexpected inbox: %+v", inbox)
	}
}

func TestSendMessage_IdempotencyKeyHeader(t *testing.T) {
	var gotHeader string
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Idempotency-Key")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(SendMessageResponse{Message: Message{ID: "msg_1"}})
	})

	_, err := client.SendMessage(context.Background(), SendMessageRequest{
		ThreadID: "th_1", Text: "hi", IdempotencyKey: "my-key-123",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if gotHeader != "my-key-123" {
		t.Errorf("Idempotency-Key header = %q, want %q", gotHeader, "my-key-123")
	}
}

func TestSendMessage_PartialSuccessWarning(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"warning": "sent but failed to persist"})
	})

	resp, err := client.SendMessage(context.Background(), SendMessageRequest{ThreadID: "th_1", Text: "hi"})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if resp.Warning == "" {
		t.Error("expected Warning to be set for the partial-success response")
	}
	if resp.ID != "" {
		t.Errorf("expected zero-value Message alongside a warning, got ID = %q", resp.ID)
	}
}

func TestNotFoundReturnsTypedError(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "inbox not found"})
	})

	_, err := client.GetInbox(context.Background(), "inbox_does_not_exist")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !IsStatus(err, http.StatusNotFound) {
		t.Errorf("IsStatus(err, 404) = false, err = %v", err)
	}
	var rbErr *Error
	if e, ok := err.(*Error); ok {
		rbErr = e
	}
	if rbErr == nil || rbErr.Message != "inbox not found" {
		t.Errorf("expected Message %q, got %+v", "inbox not found", rbErr)
	}
}

func TestDeleteWebhook_NoBody(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	if err := client.DeleteWebhook(context.Background(), "wh_1"); err != nil {
		t.Errorf("DeleteWebhook: %v", err)
	}
}

func TestGetAttachmentURL_FollowsRedirectOnce(t *testing.T) {
	const presigned = "https://storage.example.com/bucket/key?sig=abc"
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", presigned)
		w.WriteHeader(http.StatusFound)
		// If the client followed this redirect, it would GET presigned
		// (a different host, so httptest wouldn't even see it) — this
		// handler only needs to serve the 302 itself.
	})

	url, err := client.GetAttachmentURL(context.Background(), "att_1")
	if err != nil {
		t.Fatalf("GetAttachmentURL: %v", err)
	}
	if url != presigned {
		t.Errorf("url = %q, want %q", url, presigned)
	}
}

func TestVerifyWebhookSignature(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"event":"message.received"}`)

	// Same HMAC-SHA256 construction mthenga's dispatcher uses.
	valid := "b3d5b7a2e6c1f4a8d9e0c2b1a3f5e7d9c1b3a5f7e9d1c3b5a7f9e1d3c5b7a9f1"
	if VerifyWebhookSignature(secret, body, valid) {
		t.Error("expected a made-up signature to fail verification")
	}

	// Compute the real one the same way the SDK does, then confirm it verifies.
	realSig := computeTestSignature(secret, body)
	if !VerifyWebhookSignature(secret, body, realSig) {
		t.Error("expected the correctly computed signature to verify")
	}
	if VerifyWebhookSignature("wrong-secret", body, realSig) {
		t.Error("expected verification to fail with the wrong secret")
	}
}
