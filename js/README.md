# mthenga (JavaScript / TypeScript)

A client for [mthenga](https://github.com/BrianPhiri/mthenga), a headless,
programmatic inbox for AI agents over email, WhatsApp, Messenger and
Instagram. Zero runtime dependencies: it uses the global `fetch` and Web
Crypto, so it runs on Node 18+, Deno, Bun, edge runtimes and browsers. Ships
ESM with TypeScript types.

```bash
npm install mthenga
```

## Usage

```ts
import { MthengaClient } from "mthenga";

const client = new MthengaClient({
  baseUrl: "https://api.yourdomain.com",
  apiKey: "mt_live_...",
});

const inbox = await client.createInbox({ channel: "email", username: "support" });

await client.sendMessage({ thread_id: threadId, text: "Thanks for reaching out!" });
```

Every non-2xx response throws a `MthengaError` you can inspect:

```ts
import { isStatus } from "mthenga";

try {
  await client.getInbox(id);
} catch (err) {
  if (isStatus(err, 404)) {
    // handle "no such inbox" specifically
  } else throw err;
}
```

Options: `fetch` (a custom fetch, for tests or runtimes without a global
one) and `timeoutMs` (per request, default 15000, `0` disables).

## What this wraps

Only the `mt_live_...` API-key surface: inboxes, messages, threads,
attachments, webhooks, API keys, and per-org domain/WhatsApp settings. It
does **not** wrap the dashboard's FusionAuth-backed human auth (register,
login, invites). Same scope as the Go SDK.

| Method | Endpoint |
| :--- | :--- |
| `createInbox`, `getInbox` | `POST/GET /v1/inboxes` |
| `deactivateInbox`, `reactivateInbox` | `POST /v1/inboxes/:id/(de)activate` |
| `deleteInbox` | `DELETE /v1/inboxes/:id` |
| `listWhatsAppTemplates` | `GET /v1/inboxes/:id/whatsapp-templates` |
| `listInboxThreads` | `GET /v1/inboxes/:id/threads` |
| `sendMessage` | `POST /v1/messages` |
| `getThread`, `listThreadMessages` | `GET /v1/threads/:id[/messages]` |
| `uploadAttachment` | `POST /v1/attachments` |
| `getAttachmentURL` | `GET /v1/attachments/:id` |
| `createWebhook`, `listWebhooks`, `deleteWebhook`, `rotateWebhookSecret` | `.../v1/webhooks` |
| `listWebhookDeliveries` | `GET /v1/webhook-deliveries` |
| `verifyWebhookSignature` | (pure function, no request) |
| `createAPIKey`, `listAPIKeys`, `rotateAPIKey`, `revokeAPIKey` | `.../v1/api-keys` |
| `createEmailDomain` | `POST /v1/email-domains` |
| `setWhatsAppCredentials` | `PUT /v1/whatsapp-credentials` |

Request and response objects use the API's own snake_case field names, so
mthenga's API reference applies to them unchanged.

The list methods take an optional `{ limit, offset }`:

```ts
const messages = await client.listThreadMessages(threadId, { limit: 100, offset: 100 });
```

`getAttachmentURL` returns a 15-minute presigned URL without downloading the
file. It needs to read a redirect's `Location` header, which browsers hide
from `fetch`, so it works in server-side runtimes only.

## Sending email via SMTP

Software that only speaks SMTP (FusionAuth, most mail plugins) can relay
through mthenga directly — no API client needed. The server is mthenga's
own address on its SMTP port (587), login is an inbox's full email as the
username and the org's `mt_live_` API key as the password, and the message
then goes through the exact same path as `sendMessage` (verified domain,
plan limits, Postmark, threading). Node has no built-in SMTP client, so use
any — nodemailer shown:

```ts
import nodemailer from "nodemailer";

const transport = nodemailer.createTransport({
  host: "smtp.yourdomain.com",
  port: 587,
  secure: false, // STARTTLS — required when the server has TLS certs configured
  auth: { user: "support@agents.example.com", pass: "mt_live_..." },
});

await transport.sendMail({
  from: "noreply@agents.example.com", // must use the inbox's domain
  to: "alice@example.com",
  subject: "Password reset",
  text: "Click to reset your password.",
  html: "<p>Click to reset your password.</p>",
});
```

The `From` address must use the authenticated inbox's domain. SMTP status
codes map to the API's errors: `530`/`535` bad credentials, `554` unverified
sending domain or plan limit reached, `452` rate limited (retry later),
`451` transient failure (retry).

## Rotating keys and deleting inboxes

```ts
// Mint a replacement (same name); the old key keeps working until revoked.
const next = await client.rotateAPIKey(oldKeyId);
// ...deploy next.key (shown once), then:
await client.revokeAPIKey(oldKeyId);
```

`deactivateInbox` pauses an inbox but keeps its address or phone number
claimed. `deleteInbox(id)` removes the inbox with all its conversations,
messages and attachments, and frees the address for reuse. It is
irreversible and, for WhatsApp, Messenger and Instagram, changes nothing on
Meta's side.

The dashboard makes you type the inbox's name before it deletes, but the API
(and so `deleteInbox`) does not: the call deletes immediately. If your own
app exposes delete to people, add your own confirmation step before calling it.

## Idempotent sends

```ts
const res = await client.sendMessage({
  thread_id: threadId,
  text: "...",
  idempotencyKey: requestId, // your own retry-safe key
});

if ("warning" in res) {
  // Sent, but mthenga failed to save it. Do NOT retry.
}
```

A retried request with the same key (same org, same body) replays the
original response instead of sending again. Reusing the key with a
*different* body throws a `MthengaError` with status `422`. When two requests
with the same key run at once, the one that loses gets `409`.

## WhatsApp templates (outside the 24-hour window)

WhatsApp only delivers free-form text within 24 hours of the customer's
last message. Outside it, send an approved template instead of `text`:

```ts
await client.sendMessage({
  inbox_id: inboxId, // or thread_id to reply in an existing conversation
  to: "254700000001",
  template: {
    name: "booking_reminder",
    language: "en",
    parameters: ["Amina", "Saturday 8am"], // body {{1}}, {{2}}, ...
  },
});
```

Set exactly one of `text` or `template`; a template on a non-WhatsApp inbox
is a `400`. A text send after the window has closed throws a `MthengaError`
with status `422` whose message says to use a template; an
unknown/unapproved template or a wrong parameter count is also a `422`.
The stored message's `clean_text` reads `[template booking_reminder: Amina, Saturday 8am]`.

`listWhatsAppTemplates(inboxId)` lists the inbox's templates live from Meta
(`name`, `language`, `status`, `category`, `body` with its `{{1}}`
placeholders, `parameter_count`). It needs the inbox's
`whatsapp_business_account_id`: pass it to `createInbox`, or mthenga records
it from the number's first inbound message. Without one it throws a
`MthengaError` with status `422`.

## Verifying webhook deliveries

```ts
import { verifyWebhookSignature, type WebhookEvent } from "mthenga";

// e.g. a fetch-style handler (Deno, Bun, Cloudflare Workers, Next.js routes)
export async function POST(req: Request) {
  const body = await req.text();
  const ok = await verifyWebhookSignature(webhookSecret, body, req.headers.get("X-Mthenga-Signature"));
  if (!ok) return new Response(null, { status: 401 });

  const event: WebhookEvent = JSON.parse(body);
  // ...
  return new Response(null, { status: 200 });
}
```

Verify against the **raw** request body before any JSON parsing. Parsing the
body and serializing it again will not reliably produce the same bytes. With
Express, use `express.raw({ type: "application/json" })` and pass the
`Buffer`. The check is asynchronous because Web Crypto is, and the comparison
is constant-time.

## Security status

Every message carries `security_status`: `passed`, `flags` and, when the
server runs its semantic shield, `scores` (e.g. `{"prompt_injection": 0.93}`,
a 0-1 probability). `passed: false` never means blocked; mthenga flags and
delivers. In the server's default `score` mode a high score does not touch
`passed`/`flags`, so threshold it yourself; in `flag` mode a score >= 0.8 also
adds the `semantic_prompt_injection` flag. `scores` may be missing (shield off
or unavailable), so treat it as optional. `flags` can be `null` when empty.

## Errors

```ts
class MthengaError extends Error {
  status: number;  // HTTP status
  message: string; // mthenga's {"error": "..."} body, when present
  body: string;    // raw response body
}
```

## Development

```bash
npm install
npm test   # compiles with tsc, then runs node:test against dist/
```
