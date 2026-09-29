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
  apiKey: "rb_live_...",
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

Only the `rb_live_...` API-key surface: inboxes, messages, threads,
attachments, webhooks, API keys, and per-org domain/WhatsApp settings. It
does **not** wrap the dashboard's FusionAuth-backed human auth (register,
login, invites). Same scope as the Go SDK.

| Method | Endpoint |
| :--- | :--- |
| `createInbox`, `getInbox` | `POST/GET /v1/inboxes` |
| `deactivateInbox`, `reactivateInbox` | `POST /v1/inboxes/:id/(de)activate` |
| `listInboxThreads` | `GET /v1/inboxes/:id/threads` |
| `sendMessage` | `POST /v1/messages` |
| `getThread`, `listThreadMessages` | `GET /v1/threads/:id[/messages]` |
| `getAttachmentURL` | `GET /v1/attachments/:id` |
| `createWebhook`, `listWebhooks`, `deleteWebhook`, `rotateWebhookSecret` | `.../v1/webhooks` |
| `listWebhookDeliveries` | `GET /v1/webhook-deliveries` |
| `verifyWebhookSignature` | (pure function, no request) |
| `createAPIKey`, `listAPIKeys`, `revokeAPIKey` | `.../v1/api-keys` |
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
