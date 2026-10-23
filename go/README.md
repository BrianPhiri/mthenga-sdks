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

client := mthenga.NewClient("https://api.yourdomain.com", "mt_live_...")

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

The `mt_live_...` API-key-authenticated surface only — inboxes, messages,
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
| `DeleteInbox` | `DELETE /v1/inboxes/:id` |
| `ListInboxThreads` | `GET /v1/inboxes/:id/threads` |
| `ListWhatsAppTemplates` | `GET /v1/inboxes/:id/whatsapp-templates` |
| `SendMessage` | `POST /v1/messages` |
| `GetThread`, `ListThreadMessages` | `GET /v1/threads/:id[/messages]` |
| `GetAttachmentURL` | `GET /v1/attachments/:id` |
| `CreateWebhook`, `ListWebhooks`, `DeleteWebhook`, `RotateWebhookSecret` | `.../v1/webhooks` |
| `ListWebhookDeliveries` | `GET /v1/webhook-deliveries` |
| `VerifyWebhookSignature` | (pure function — no request) |
| `CreateAPIKey`, `ListAPIKeys`, `RotateAPIKey`, `RevokeAPIKey` | `.../v1/api-keys` |
| `CreateEmailDomain` | `POST /v1/email-domains` |
| `SetWhatsAppCredentials` | `PUT /v1/whatsapp-credentials` |

See mthenga's own `04_api_reference.md` for exact request/response
shapes and every documented error case — this client is a thin,
line-for-line wrapper over that reference, not an abstraction on top of it.

## Sending email via SMTP

Software that only speaks SMTP (FusionAuth, most mail plugins) can relay
through mthenga directly — no API client needed. The server is mthenga's
own address on its SMTP port (587), login is an inbox's full email as the
username and the org's `mt_live_` API key as the password, and the message
then goes through the exact same path as `SendMessage` (verified domain,
plan limits, Postmark, threading). Any SMTP client works; stdlib in Go:

```go
import "net/smtp"

host := "smtp.yourdomain.com"
auth := smtp.PlainAuth("", "support@agents.example.com", "mt_live_...", host)

msg := []byte("From: Support <noreply@agents.example.com>\r\n" +
    "To: alice@example.com\r\n" +
    "Subject: Password reset\r\n" +
    "Content-Type: text/html; charset=utf-8\r\n" +
    "\r\n" +
    "<p>Click to reset your password.</p>\r\n")

err := smtp.SendMail(host+":587", auth, "noreply@agents.example.com",
    []string{"alice@example.com"}, msg)
```

`SendMail` upgrades to STARTTLS automatically whenever the server advertises
it (it's required when TLS certs are configured server-side). The `From`
address must use the authenticated inbox's domain. SMTP status codes map to
the API's errors: `530`/`535` bad credentials, `554` unverified sending
domain or plan limit reached, `452` rate limited (retry later), `451`
transient failure (retry).

## Rotating keys and deleting inboxes

```go
// Mint a replacement (same name); the old key keeps working until revoked.
next, err := client.RotateAPIKey(ctx, oldKeyID)
// ...deploy next.Key (shown once), then:
err = client.RevokeAPIKey(ctx, oldKeyID)
```

`DeactivateInbox` pauses an inbox but keeps its address or phone number
claimed. `DeleteInbox(ctx, id)` removes the inbox with all its conversations,
messages and attachments, and frees the address for reuse. It is irreversible
and, for WhatsApp, Messenger and Instagram, changes nothing on Meta's side.

The dashboard makes you type the inbox's name before it deletes, but the API
(and so `DeleteInbox`) does not: the call deletes immediately. If your own
app exposes delete to people, add your own confirmation step before calling it.

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

## WhatsApp templates (outside the 24-hour window)

WhatsApp only delivers free-form text within 24 hours of the customer's
last message. Outside it, send an approved template instead of `Text`:

```go
resp, err := client.SendMessage(ctx, mthenga.SendMessageRequest{
    InboxID: inboxID, // or ThreadID to reply in an existing conversation
    To:      "254700000001",
    Template: &mthenga.Template{
        Name:       "booking_reminder",
        Language:   "en",
        Parameters: []string{"Amina", "Saturday 8am"}, // body {{1}}, {{2}}, ...
    },
})
```

Set exactly one of `Text` or `Template`; a template on a non-WhatsApp inbox
is a `400`. A text send after the window has closed returns
`*Error{StatusCode: 422}` whose message says to use a template; an
unknown/unapproved template or a wrong parameter count is also a `422`.
The stored message's `CleanText` reads `[template booking_reminder: Amina, Saturday 8am]`.

`ListWhatsAppTemplates(ctx, inboxID)` lists the inbox's templates live from
Meta (`Name`, `Language`, `Status`, `Category`, `Body` with its `{{1}}`
placeholders, `ParameterCount`). It needs the inbox's
`WhatsAppBusinessAccountID`: pass it in `CreateInboxRequest`, or mthenga
records it from the number's first inbound message. Without one it returns
`*Error{StatusCode: 422}`.

## Sending files

Upload first, then name the upload in `AttachmentIDs`:

```go
up, err := client.UploadAttachment(ctx, "invoice.pdf", "application/pdf", pdfBytes)
if err != nil {
    return err
}
resp, err := client.SendMessage(ctx, mthenga.SendMessageRequest{
    ThreadID:      threadID,
    Text:          "Here is your invoice",
    AttachmentIDs: []string{up.ID},
})
```

Uploads take up to 25 MB and are deleted if not sent within 24 hours.
Per channel: email needs `Text` and allows several files, 7 MB in total;
WhatsApp, Messenger and Instagram take exactly one file per message. On
WhatsApp `Text` becomes the caption; Messenger and Instagram can't carry
text with a file, so send it separately. Not combinable with `Template`.

Inbound files arrive on the `message.received` webhook: decode the body's
`message` into `mthenga.Message` and read `Attachments` (`ID`, `Filename`,
`ContentType`, `SizeBytes`); `GetAttachmentURL(ctx, a.ID)` returns a
short-lived download URL.

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

## Security status

Every message carries `security_status`: `passed`, `flags` and, when the
server runs its semantic shield, `scores` (e.g. `{"prompt_injection": 0.93}`,
a 0-1 probability). `passed: false` never means blocked; mthenga flags and
delivers. In the server's default `score` mode a high score does not touch
`passed`/`flags`, so threshold it yourself; in `flag` mode a score >= 0.8 also
adds the `semantic_prompt_injection` flag. `scores` may be missing (shield off
or unavailable), so treat it as optional. `flags` can be `null` when empty.

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
