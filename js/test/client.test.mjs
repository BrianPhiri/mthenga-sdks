import { test } from "node:test";
import assert from "node:assert/strict";
import { createHmac } from "node:crypto";
import { MthengaClient, MthengaError, isStatus, verifyWebhookSignature } from "../dist/index.js";

// Signed exactly the way mthenga's internal/webhook/sign.go does it, with
// node:crypto rather than the SDK's Web Crypto path, so the test proves
// the two agree instead of calling the same code twice.
const serverSign = (secret, body) => createHmac("sha256", secret).update(body).digest("hex");

test("verifyWebhookSignature round-trip", async () => {
  const secret = "a".repeat(64);
  const body = JSON.stringify({ event: "message.received", message: { id: "msg_1", clean_text: "héllo" } });
  const sig = serverSign(secret, body);

  assert.equal(await verifyWebhookSignature(secret, body, sig), true);
  assert.equal(await verifyWebhookSignature(secret, new TextEncoder().encode(body), sig), true);
  assert.equal(await verifyWebhookSignature("wrong", body, sig), false);
  assert.equal(await verifyWebhookSignature(secret, body + " ", sig), false);
  assert.equal(await verifyWebhookSignature(secret, body, sig.slice(0, -1) + (sig.endsWith("0") ? "1" : "0")), false);
  assert.equal(await verifyWebhookSignature(secret, body, "zz"), false);
  assert.equal(await verifyWebhookSignature(secret, body, null), false);
});

function mockFetch(status, body, headers = {}) {
  const calls = [];
  const fetch = async (url, init) => {
    calls.push({ url, init });
    return new Response(body, { status, headers });
  };
  return { fetch, calls };
}

test("sendMessage builds the request and parses the response", async () => {
  const { fetch, calls } = mockFetch(201, JSON.stringify({ id: "msg_1", thread_id: "th_1", clean_text: "hi" }));
  const client = new MthengaClient({ baseUrl: "https://api.example.com/", apiKey: "mt_live_test", fetch });

  const res = await client.sendMessage({ thread_id: "th_1", text: "hi", idempotencyKey: "k1" });

  assert.equal(res.id, "msg_1");
  assert.equal(calls[0].url, "https://api.example.com/v1/messages");
  assert.equal(calls[0].init.method, "POST");
  assert.equal(calls[0].init.headers.Authorization, "Bearer mt_live_test");
  assert.equal(calls[0].init.headers["Idempotency-Key"], "k1");
  assert.deepEqual(JSON.parse(calls[0].init.body), { thread_id: "th_1", text: "hi" });
});

test("pagination goes in the query string", async () => {
  const { fetch, calls } = mockFetch(200, "[]");
  const client = new MthengaClient({ baseUrl: "https://api.example.com", apiKey: "k", fetch });
  await client.listThreadMessages("th_1", { limit: 10, offset: 20 });
  assert.equal(calls[0].url, "https://api.example.com/v1/threads/th_1/messages?limit=10&offset=20");
});

test("non-2xx throws MthengaError", async () => {
  const { fetch } = mockFetch(404, JSON.stringify({ error: "inbox not found" }));
  const client = new MthengaClient({ baseUrl: "https://api.example.com", apiKey: "k", fetch });

  await assert.rejects(client.getInbox("inbox_x"), (err) => {
    assert.ok(err instanceof MthengaError);
    assert.equal(err.status, 404);
    assert.equal(err.message, "inbox not found");
    assert.ok(isStatus(err, 404));
    return true;
  });
});

test("getAttachmentURL returns the redirect Location", async () => {
  const { fetch, calls } = mockFetch(302, null, { Location: "https://r2.example.com/f?sig=x" });
  const client = new MthengaClient({ baseUrl: "https://api.example.com", apiKey: "k", fetch });
  assert.equal(await client.getAttachmentURL("att_1"), "https://r2.example.com/f?sig=x");
  assert.equal(calls[0].init.redirect, "manual");
});

test("createPartnerOrg sends name and external_ref and returns the child's key", async () => {
  const { fetch, calls } = mockFetch(201, JSON.stringify({ org: { id: "o1", name: "Maya Shoes", external_ref: "proj-1", created_at: "2026-10-03T10:00:00Z" }, api_key: "mt_live_child" }));
  const client = new MthengaClient({ baseUrl: "https://api.example.com", apiKey: "mt_live_partner", fetch });

  const res = await client.createPartnerOrg("Maya Shoes", "proj-1");

  assert.equal(res.org.id, "o1");
  assert.equal(res.api_key, "mt_live_child");
  assert.equal(calls[0].url, "https://api.example.com/v1/partner/orgs");
  assert.deepEqual(JSON.parse(calls[0].init.body), { name: "Maya Shoes", external_ref: "proj-1" });
});
