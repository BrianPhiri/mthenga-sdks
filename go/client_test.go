package mthenga

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
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
	return NewClient(srv.URL, "mt_live_test"), srv
}

func TestCreateInbox(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/inboxes" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer mt_live_test" {
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

func TestSendMessage_Template(t *testing.T) {
	var got map[string]any
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(SendMessageResponse{Message: Message{ID: "msg_1"}})
	})

	_, err := client.SendMessage(context.Background(), SendMessageRequest{
		InboxID: "inbox_1", To: "254700000001",
		Template: &Template{Name: "booking_reminder", Language: "en", Parameters: []string{"Amina", "Saturday 8am"}},
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	b, _ := json.Marshal(got)
	want := `{"inbox_id":"inbox_1","template":{"language":"en","name":"booking_reminder","parameters":["Amina","Saturday 8am"]},"to":"254700000001"}`
	if string(b) != want {
		t.Errorf("body = %s, want %s", b, want)
	}
}

func TestUploadThenSendAttachment(t *testing.T) {
	var sent map[string]any
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/attachments":
			file, header, err := r.FormFile("file")
			if err != nil {
				t.Fatalf("FormFile: %v", err)
			}
			data, _ := io.ReadAll(file)
			if header.Filename != `menu "v2".pdf` || header.Header.Get("Content-Type") != "application/pdf" || string(data) != "%PDF-1.4" {
				t.Errorf("upload = %q %q %q", header.Filename, header.Header.Get("Content-Type"), data)
			}
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"up_1","filename":"menu \"v2\".pdf","content_type":"application/pdf","size_bytes":8}`))
		case "/v1/messages":
			json.NewDecoder(r.Body).Decode(&sent)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(SendMessageResponse{Message: Message{ID: "msg_1"}})
		}
	})

	up, err := client.UploadAttachment(context.Background(), `menu "v2".pdf`, "application/pdf", []byte("%PDF-1.4"))
	if err != nil || up.ID != "up_1" || up.SizeBytes != 8 {
		t.Fatalf("UploadAttachment = %+v, %v", up, err)
	}
	if _, err := client.SendMessage(context.Background(), SendMessageRequest{ThreadID: "th_1", Text: "Our menu", AttachmentIDs: []string{up.ID}}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	b, _ := json.Marshal(sent)
	if want := `{"attachment_ids":["up_1"],"text":"Our menu","thread_id":"th_1"}`; string(b) != want {
		t.Errorf("body = %s, want %s", b, want)
	}
}

func TestWebhookMessageAttachments(t *testing.T) {
	body := []byte(`{"event":"message.received","message":{"id":"msg_1","channel":"whatsapp","clean_text":"see photo",` +
		`"attachments":[{"id":"9b2c","filename":"photo.jpg","content_type":"image/jpeg","size_bytes":48213}],"created_at":"2026-10-23T10:00:00Z"}}`)
	var payload struct {
		Event   string  `json:"event"`
		Message Message `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	want := Attachment{ID: "9b2c", Filename: "photo.jpg", ContentType: "image/jpeg", SizeBytes: 48213}
	if len(payload.Message.Attachments) != 1 || payload.Message.Attachments[0] != want {
		t.Errorf("attachments = %+v, want [%+v]", payload.Message.Attachments, want)
	}
}

func TestListWhatsAppTemplates(t *testing.T) {
	var gotPath string
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.Method + " " + r.URL.Path
		w.Write([]byte(`[{"name":"booking_reminder","language":"en","status":"APPROVED","category":"UTILITY","body":"Hi {{1}}, see you {{2}}.","parameter_count":2}]`))
	})

	got, err := client.ListWhatsAppTemplates(context.Background(), "inbox_1")
	if err != nil {
		t.Fatalf("ListWhatsAppTemplates: %v", err)
	}
	want := WhatsAppTemplate{Name: "booking_reminder", Language: "en", Status: "APPROVED", Category: "UTILITY", Body: "Hi {{1}}, see you {{2}}.", ParameterCount: 2}
	if gotPath != "GET /v1/inboxes/inbox_1/whatsapp-templates" || len(got) != 1 || got[0] != want {
		t.Errorf("%s -> %+v", gotPath, got)
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

func TestDeleteInbox(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/inboxes/inbox_1" {
			t.Errorf("request = %s %s, want DELETE /v1/inboxes/inbox_1", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	if err := client.DeleteInbox(context.Background(), "inbox_1"); err != nil {
		t.Errorf("DeleteInbox: %v", err)
	}
}

func TestRotateAPIKey(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/api-keys/key-1/rotate" {
			t.Errorf("request = %s %s, want POST /v1/api-keys/key-1/rotate", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"key-2","name":"Production","key_prefix":"mt_live_abcd","key":"mt_live_abcdef"}`))
	})

	resp, err := client.RotateAPIKey(context.Background(), "key-1")
	if err != nil {
		t.Fatalf("RotateAPIKey: %v", err)
	}
	if resp.ID != "key-2" || resp.Name != "Production" || resp.Key != "mt_live_abcdef" {
		t.Errorf("resp = %+v", resp)
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
