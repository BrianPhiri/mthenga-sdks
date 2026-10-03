// mthenga: a zero-dependency client for mthenga's REST API, a headless
// programmatic inbox for AI agents over email, WhatsApp, Messenger and
// Instagram. It runs anywhere with `fetch` and Web Crypto (Node 18+,
// browsers, Deno, Bun, edge runtimes).
//
// It wraps only the mt_live_... API-key surface (inboxes, messages, threads,
// attachments, webhooks, API keys, settings), not the dashboard's
// FusionAuth-backed human auth. Same scope as the Go SDK.

export type Channel = "email" | "whatsapp" | "messenger" | "instagram";

/** Nullable fields are `null` for the channels they don't apply to. */
export interface Inbox {
  id: string;
  org_id: string;
  name: string;
  channel: Channel;
  email_username: string | null;
  email_domain: string | null;
  full_email: string | null;
  whatsapp_phone_number: string | null;
  whatsapp_phone_number_id: string | null;
  /** Facebook Page id (messenger) or IG Business Account id (instagram). */
  external_account_id: string | null;
  external_account_name: string | null;
  shield_enabled: boolean;
  is_active: boolean;
  created_at: string;
}

/**
 * Set exactly the fields for the channel being created:
 * email: `username` (+ optional `domain`, registered via createEmailDomain);
 * whatsapp: `phone_number` + `phone_number_id` from Meta;
 * messenger/instagram: `external_account_id` (+ optional `external_account_name`).
 */
export interface CreateInboxRequest {
  /** Defaults to "email" on the server. */
  channel?: Channel;
  name?: string;
  username?: string;
  domain?: string;
  phone_number?: string;
  phone_number_id?: string;
  external_account_id?: string;
  external_account_name?: string;
}

export interface Thread {
  id: string;
  inbox_id: string;
  contact_id: string | null;
  channel: Channel;
  /** null for non-email channels. */
  subject: string | null;
  status: string;
  last_message_at: string | null;
  created_at: string;
}

/** The injection shield's verdict. `passed: false` flags, it never blocks. */
export interface SecurityStatus {
  passed: boolean;
  flags: string[];
  /**
   * Semantic-classifier probabilities (0-1), currently only `prompt_injection`.
   * Absent when the server's semantic shield is off or was unavailable; a
   * score alone never changes `passed` or `flags`.
   */
  scores?: Record<string, number>;
}

export interface Message {
  id: string;
  thread_id: string;
  inbox_id: string;
  channel: Channel;
  direction: "inbound" | "outbound";
  sender_handle: string;
  sender_name: string | null;
  recipients: string[] | null;
  clean_text: string;
  raw_body: string | null;
  security_status: SecurityStatus;
  /** Email Message-ID, WhatsApp wamid, or Meta message id. */
  external_id: string | null;
  in_reply_to: string | null;
  created_at: string;
}

/**
 * Either `thread_id` (reply in an existing conversation) or `inbox_id` + `to`
 * (start a new one; `subject` required for a new email thread).
 */
export interface SendMessageRequest {
  thread_id?: string;
  inbox_id?: string;
  to?: string;
  subject?: string;
  text: string;
  /** Sent as the Idempotency-Key header, not in the body. */
  idempotencyKey?: string;
}

/**
 * Almost always the sent Message. In one rare case the send succeeded but
 * mthenga failed to persist it: then only `warning` is set, and the send
 * must NOT be retried (it would duplicate).
 */
export type SendMessageResponse = Message | { warning: string };

export interface Webhook {
  id: string;
  /** Only on createWebhook / rotateWebhookSecret. */
  org_id?: string;
  url: string;
  /** Only on createWebhook / rotateWebhookSecret, shown once. */
  secret?: string;
  is_active: boolean;
  events: string[];
  created_at: string;
}

export interface WebhookDelivery {
  id: string;
  webhook_config_id: string;
  webhook_url: string;
  message_id: string | null;
  event: string;
  payload: WebhookEvent;
  status: "pending" | "delivered" | "failed";
  attempts: number;
  last_error: string | null;
  created_at: string;
  next_attempt_at: string | null;
  delivered_at: string | null;
}

/** The JSON body mthenga POSTs to your webhook URL. */
export interface WebhookEvent {
  event: "message.received";
  message: {
    id: string;
    thread_id: string;
    inbox_id: string;
    channel: Channel;
    direction: "inbound";
    sender_handle: string;
    sender_name?: string;
    subject?: string;
    clean_text: string;
    security_status: SecurityStatus;
    attachments?: { id: string; filename: string; content_type: string; size_bytes: number }[];
    created_at: string;
  };
}

export interface APIKey {
  id: string;
  name: string;
  key_prefix: string;
  created_at: string;
  last_used_at: string | null;
  revoked_at: string | null;
}

/** `key` is the raw mt_live_... key, shown exactly once. */
export interface CreateAPIKeyResponse extends APIKey {
  key: string;
}

/** A child org a partner created (partner API). */
export interface PartnerOrg {
  id: string;
  name: string;
  external_ref?: string;
  created_at: string;
}

/** A child org plus a new raw API key for it, shown once. */
export interface PartnerOrgKey {
  org: PartnerOrg;
  api_key: string;
}

export interface EmailDomain {
  id: string;
  org_id: string;
  domain: string;
  created_at: string;
}

export interface Page {
  limit?: number;
  offset?: number;
}

export interface ClientOptions {
  baseUrl: string;
  apiKey: string;
  /** Defaults to the global fetch. */
  fetch?: typeof fetch;
  /** Per-request timeout. Default 15000; 0 disables. */
  timeoutMs?: number;
}

/** Thrown for any non-2xx response. */
export class MthengaError extends Error {
  readonly status: number;
  /** Raw response body. `message` is mthenga's {"error": "..."} when present. */
  readonly body: string;

  constructor(status: number, body: string) {
    let msg = body;
    try {
      const parsed = JSON.parse(body);
      if (typeof parsed?.error === "string") msg = parsed.error;
    } catch {}
    super(`mthenga: ${status} ${msg}`);
    this.name = "MthengaError";
    this.status = status;
    this.body = body;
    this.message = msg;
  }
}

/** True if err is a MthengaError with the given HTTP status. */
export function isStatus(err: unknown, status: number): boolean {
  return err instanceof MthengaError && err.status === status;
}

export class MthengaClient {
  private readonly baseUrl: string;
  private readonly apiKey: string;
  private readonly fetchFn: typeof fetch;
  private readonly timeoutMs: number;

  constructor(opts: ClientOptions) {
    this.baseUrl = opts.baseUrl.replace(/\/+$/, "");
    this.apiKey = opts.apiKey;
    // Wrapped, not stored bare: calling a browser's fetch with `this` set to
    // another object throws "Illegal invocation".
    this.fetchFn = opts.fetch ?? ((input, init) => globalThis.fetch(input, init));
    this.timeoutMs = opts.timeoutMs ?? 15000;
  }

  private async send(method: string, path: string, body?: unknown, headers: Record<string, string> = {}, redirect?: RequestRedirect): Promise<Response> {
    const res = await this.fetchFn(this.baseUrl + path, {
      method,
      headers: {
        Authorization: `Bearer ${this.apiKey}`,
        ...(body !== undefined && { "Content-Type": "application/json" }),
        ...headers,
      },
      body: body === undefined ? undefined : JSON.stringify(body),
      redirect,
      signal: this.timeoutMs > 0 ? AbortSignal.timeout(this.timeoutMs) : undefined,
    });
    if (res.status >= 400) throw new MthengaError(res.status, await res.text());
    return res;
  }

  private async request<T>(method: string, path: string, body?: unknown, headers?: Record<string, string>): Promise<T> {
    const text = await (await this.send(method, path, body, headers)).text();
    return (text ? JSON.parse(text) : undefined) as T;
  }

  // Inboxes

  /** 409 if the address/number/account is taken, 403 if `domain` isn't registered to your org. */
  createInbox(req: CreateInboxRequest): Promise<Inbox> {
    return this.request("POST", "/v1/inboxes", req);
  }

  /** 404 if missing or not your org's. */
  getInbox(id: string): Promise<Inbox> {
    return this.request("GET", `/v1/inboxes/${enc(id)}`);
  }

  /** Stops sending/receiving without deleting. Reversible via reactivateInbox. */
  deactivateInbox(id: string): Promise<Inbox> {
    return this.request("POST", `/v1/inboxes/${enc(id)}/deactivate`);
  }

  reactivateInbox(id: string): Promise<Inbox> {
    return this.request("POST", `/v1/inboxes/${enc(id)}/reactivate`);
  }

  /** Most recently active first. Server default limit 50, max 200. */
  listInboxThreads(inboxId: string, page?: Page): Promise<Thread[]> {
    return this.request("GET", `/v1/inboxes/${enc(inboxId)}/threads${qs(page)}`);
  }

  // Messages and threads

  /**
   * 400 malformed, 404 unknown thread/inbox, 422 idempotency key reused with
   * a different body (or thread has no recipient), 409 concurrent request with
   * the same key, 502 provider send failed (safe to retry).
   */
  sendMessage(req: SendMessageRequest): Promise<SendMessageResponse> {
    const { idempotencyKey, ...body } = req;
    return this.request("POST", "/v1/messages", body, idempotencyKey ? { "Idempotency-Key": idempotencyKey } : undefined);
  }

  getThread(id: string): Promise<Thread> {
    return this.request("GET", `/v1/threads/${enc(id)}`);
  }

  /** Oldest first. Server default limit 500, max 1000. */
  listThreadMessages(threadId: string, page?: Page): Promise<Message[]> {
    return this.request("GET", `/v1/threads/${enc(threadId)}/messages${qs(page)}`);
  }

  /**
   * Resolves an attachment id to a short-lived (15 min) presigned URL without
   * downloading the file. Server-side runtimes only: browsers hide the
   * redirect's Location header from fetch.
   */
  async getAttachmentURL(id: string): Promise<string> {
    const res = await this.send("GET", `/v1/attachments/${enc(id)}`, undefined, {}, "manual");
    if (res.type === "opaqueredirect") throw new Error("mthenga: getAttachmentURL is not supported in browsers (redirect Location is hidden)");
    const location = res.headers.get("Location");
    if (res.status !== 302 || !location) throw new MthengaError(res.status, await res.text());
    return location;
  }

  // Webhooks

  /** The returned `secret` is shown once; keep it for verifyWebhookSignature. */
  createWebhook(url: string): Promise<Webhook> {
    return this.request("POST", "/v1/webhooks", { url });
  }

  /** Active webhooks only, never with secrets. */
  listWebhooks(): Promise<Webhook[]> {
    return this.request("GET", "/v1/webhooks");
  }

  /** Soft-deactivates and fails its pending deliveries. Idempotent. */
  async deleteWebhook(id: string): Promise<void> {
    await this.request("DELETE", `/v1/webhooks/${enc(id)}`);
  }

  /** New secret, shown once. */
  rotateWebhookSecret(id: string): Promise<Webhook> {
    return this.request("POST", `/v1/webhooks/${enc(id)}/rotate`);
  }

  /** Most recent first, across all webhooks. Server default limit 50, max 200. */
  listWebhookDeliveries(page?: Page): Promise<WebhookDelivery[]> {
    return this.request("GET", `/v1/webhook-deliveries${qs(page)}`);
  }

  // API keys

  /** Server defaults name to "Default Key". */
  createAPIKey(name?: string): Promise<CreateAPIKeyResponse> {
    return this.request("POST", "/v1/api-keys", { name: name ?? "" });
  }

  /** Active and revoked, newest first. */
  listAPIKeys(): Promise<APIKey[]> {
    return this.request("GET", "/v1/api-keys");
  }

  /** Immediate and irreversible. Idempotent for unknown/revoked ids. */
  async revokeAPIKey(id: string): Promise<void> {
    await this.request("DELETE", `/v1/api-keys/${enc(id)}`);
  }

  // Partner API — only for an org the Mthenga operator made a partner
  // (others get a 403 MthengaError). Use each child's key with its own
  // client for its inboxes, credentials, domains and webhooks.

  /**
   * Create a child org. With an `externalRef` (your own id for it) the call
   * is idempotent: repeating it returns the same org, with a fresh key.
   */
  createPartnerOrg(name: string, externalRef?: string): Promise<PartnerOrgKey> {
    return this.request("POST", "/v1/partner/orgs", { name, external_ref: externalRef });
  }

  /** Your child orgs, newest first. */
  async listPartnerOrgs(): Promise<PartnerOrg[]> {
    const res = await this.request<{ orgs: PartnerOrg[] }>("GET", "/v1/partner/orgs");
    return res.orgs;
  }

  /** A new key for one of your child orgs (rotation or a lost key). */
  createPartnerOrgAPIKey(orgId: string): Promise<PartnerOrgKey> {
    return this.request("POST", `/v1/partner/orgs/${enc(orgId)}/api-keys`);
  }

  // Settings

  /** 409 if already registered to any org. */
  createEmailDomain(domain: string): Promise<EmailDomain> {
    return this.request("POST", "/v1/email-domains", { domain });
  }

  /** Bring-your-own Meta app. Replaces any stored credentials. */
  async setWhatsAppCredentials(accessToken: string, appSecret: string): Promise<void> {
    await this.request("PUT", "/v1/whatsapp-credentials", { access_token: accessToken, app_secret: appSecret });
  }
}

const enc = encodeURIComponent;

function qs(page?: Page): string {
  const p = new URLSearchParams();
  if (page?.limit !== undefined) p.set("limit", String(page.limit));
  if (page?.offset !== undefined) p.set("offset", String(page.offset));
  const s = p.toString();
  return s ? `?${s}` : "";
}

/**
 * Checks an X-Mthenga-Signature header (hex HMAC-SHA256 of the raw body keyed
 * with the webhook secret). Pass the raw body bytes/string, before any JSON
 * parsing. Constant-time: the comparison is done by crypto.subtle.verify.
 */
export async function verifyWebhookSignature(secret: string, rawBody: string | Uint8Array | ArrayBuffer, signature: string | null | undefined): Promise<boolean> {
  if (!signature || !/^[0-9a-fA-F]{64}$/.test(signature)) return false;
  const sig = new Uint8Array(32);
  for (let i = 0; i < 32; i++) sig[i] = parseInt(signature.slice(i * 2, i * 2 + 2), 16);
  const te = new TextEncoder();
  const key = await crypto.subtle.importKey("raw", te.encode(secret), { name: "HMAC", hash: "SHA-256" }, false, ["verify"]);
  // Cast: TS types reject Uint8Array<ArrayBufferLike> (e.g. a Node Buffer) though Web Crypto accepts it.
  const data = (typeof rawBody === "string" ? te.encode(rawBody) : rawBody) as BufferSource;
  return crypto.subtle.verify("HMAC", key, sig, data);
}
