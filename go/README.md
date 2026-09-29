# mthenga-go

A Go client for [mthenga](https://github.com/BrianPhiri/mthenga) — a
headless, programmatic inbox for AI agents over email and WhatsApp. No
dependencies beyond the standard library.

```bash
go get github.com/BrianPhiri/mthenga-sdks/go
```

## Usage

```go
import mthenga "github.com/BrianPhiri/mthenga-sdks/go"

client := mthenga.NewClient("https://api.yourdomain.com", "rb_live_...")

inbox, err := client.CreateInbox(ctx, mthenga.CreateInboxRequest{
    Channel:  "email",
    Username: "support",
})

_, err = client.SendMessage(ctx, mthenga.SendMessageRequest{
    ThreadID: threadID,
    Text:     "Thanks for reaching out!",
})
```

Every method takes a `context.Context` first and returns a typed error you
can inspect:

```go
inbox, err := client.GetInbox(ctx, id)
if mthenga.IsStatus(err, http.StatusNotFound) {
    // handle "no such inbox" specifically
}
```

## What this wraps

The `rb_live_...` API-key-authenticated surface only — inboxes, messages,
threads, webhooks, API keys, and per-org domain/WhatsApp settings. This is
the surface a backend service integrates against to give an AI agent a
real inbox. It does **not** wrap mthenga's separate FusionAuth-backed
human/dashboard auth (register, login, team invites) — that's a different
actor and trust model (a person logging into a browser, not a service
calling an API), out of scope for a machine client.

| Method | Endpoint |
| :--- | :--- |
| `CreateInbox`, `GetInbox` | `POST/GET /v1/inboxes` |
| `DeactivateInbox`, `ReactivateInbox` | `POST /v1/inboxes/:id/(de)activate` |
| `ListInboxThreads` | `GET /v1/inboxes/:id/threads` |
| `SendMessage` | `POST /v1/messages` |
| `GetThread`, `ListThreadMessages` | `GET /v1/threads/:id[/messages]` |
| `GetAttachmentURL` | `GET /v1/attachments/:id` |
| `CreateWebhook`, `ListWebhooks`, `DeleteWebhook`, `RotateWebhookSecret` | `.../v1/webhooks` |
| `ListWebhookDeliveries` | `GET /v1/webhook-deliveries` |
| `VerifyWebhookSignature` | (pure function — no request) |
| `CreateAPIKey`, `ListAPIKeys`, `RevokeAPIKey` | `.../v1/api-keys` |
| `CreateEmailDomain` | `POST /v1/email-domains` |
| `SetWhatsAppCredentials` | `PUT /v1/whatsapp-credentials` |

See mthenga's own `04_api_reference.md` for exact request/response
shapes and every documented error case — this client is a thin,
line-for-line wrapper over that reference, not an abstraction on top of it.

## Idempotent sends

```go
resp, err := client.SendMessage(ctx, mthenga.SendMessageRequest{
    ThreadID:       threadID,
    Text:           "...",
    IdempotencyKey: requestID, // your own retry-safe key
})
```

A retried request with the same key (same org, same body) replays the
original response instead of sending again. Reusing the key with a
*different* body returns `*Error{StatusCode: 422}`; a genuine race between
two concurrent requests with the same key gives the loser `409`.

## Verifying webhook deliveries

```go
func handleWebhook(w http.ResponseWriter, r *http.Request) {
    body, _ := io.ReadAll(r.Body)
    sig := r.Header.Get("X-Mthenga-Signature")

    if !mthenga.VerifyWebhookSignature(webhookSecret, body, sig) {
        w.WriteHeader(http.StatusUnauthorized)
        return
    }
    // ...
}
```

Verify against the **raw** request body, before any JSON parsing —
re-serializing the parsed object and hashing that will not reliably match.

## Errors

Every non-2xx response comes back as `*mthenga.Error`:

```go
type Error struct {
    StatusCode int
    Message    string // mthenga's {"error": "..."} body, when present
    RawBody    string
}
```

Use `mthenga.IsStatus(err, code)` rather than comparing `StatusCode`
directly — it's `errors.As`-based, so it still works if something wraps
the error further up your own call stack.

## Testing this library

```bash
go test ./...
```

Tests run against a real `httptest.Server`, not a mocked transport — the
thing under test is this package's own request-building and
response-parsing, so mocking at the HTTP boundary (not deeper) is the
right level. Verified separately, by hand, against a real running mthenga
instance for every method in this README before the first release.
