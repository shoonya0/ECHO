// Contract test for the ECHO REST + WebSocket API.
//
// Mirrors the Postman collections "ECHO API" and "ECHOWebsocket", but is
// self-contained: it creates two throwaway users, chains the IDs between
// requests, and deletes both users at the end.
//
// For every step it records the HTTP status, the response `message`, and the
// *shape* of the body (keys + JSON types, not values). Comparing two snapshots
// shows whether a change altered anything the Android app depends on.
//
// Usage:
//   npm install
//   node run.mjs [baseUrl] [snapshot.json]
//   node run.mjs http://localhost:8080 before.json
//   node diff.mjs before.json after.json

import { writeFileSync } from "node:fs";
import WebSocket from "ws";

const BASE = process.argv[2] || "http://localhost:8080";
const OUT = process.argv[3] || "snapshot.json";
const API = `${BASE}/echo/v1`;
const WS_URL = API.replace(/^http/, "ws") + "/ws/chat";

const results = {};
let failures = 0;

// ---------- helpers ----------

function shape(v) {
  if (v === null) return "null";
  if (Array.isArray(v)) return v.length ? [shape(v[0])] : [];
  if (typeof v === "object") {
    const out = {};
    for (const k of Object.keys(v).sort()) out[k] = shape(v[k]);
    return out;
  }
  return typeof v;
}

function record(name, entry, expectStatus) {
  results[name] = entry;
  const ok = expectStatus === undefined || entry.status === expectStatus;
  if (!ok) failures++;
  console.log(`${ok ? "PASS" : "FAIL"}  ${String(entry.status).padEnd(4)} ${name}${ok ? "" : `  (expected ${expectStatus})`}`);
}

async function call(name, method, path, { token, body, expect } = {}) {
  const headers = {};
  if (token) headers.Authorization = `Bearer ${token}`;
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const res = await fetch(API + path, {
    method,
    headers,
    body: body === undefined ? undefined : typeof body === "string" ? body : JSON.stringify(body),
  });
  const text = await res.text();
  let json;
  try { json = JSON.parse(text); } catch { json = text; }
  record(name, { status: res.status, message: json?.message ?? json?.error ?? null, code: json?.code ?? null, shape: shape(json) }, expect);
  return json;
}

function openWS(token) {
  return new Promise((resolve, reject) => {
    const ws = new WebSocket(WS_URL, { headers: { Authorization: `Bearer ${token}` } });
    ws.inbox = [];
    ws.on("message", (d) => ws.inbox.push(JSON.parse(d.toString())));
    ws.on("open", () => resolve(ws));
    ws.on("unexpected-response", (_req, res) => reject(new Error(`WS upgrade failed: ${res.statusCode}`)));
    ws.on("error", reject);
  });
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// Send a WS request, wait, and record the event types each socket received.
async function wsStep(name, sender, payload, sockets, waitMs = 400) {
  for (const s of Object.values(sockets)) s.inbox.length = 0;
  sender.send(typeof payload === "string" ? payload : JSON.stringify(payload));
  await sleep(waitMs);
  const received = {};
  for (const [who, s] of Object.entries(sockets)) {
    received[who] = s.inbox
      .map((m) => ({ type: m.type, code: m.data?.code ?? null, shape: shape(m) }))
      .sort((a, b) => (a.type + a.code).localeCompare(b.type + b.code));
  }
  results[`WS ${name}`] = { received };
  const summary = Object.entries(received)
    .map(([who, evs]) => `${who}:[${evs.map((e) => e.type + (e.code ? "/" + e.code : "")).join(",")}]`)
    .join(" ");
  console.log(`WS    ${name.padEnd(28)} ${summary}`);
  return received;
}

// ---------- flow ----------

const stamp = Date.now();
const A = { email: `ct.a.${stamp}@echo.test`, username: `ct_a_${stamp}`, password: "testpass123" };
const B = { email: `ct.b.${stamp}@echo.test`, username: `ct_b_${stamp}`, password: "testpass123" };

// Auth
await call("signup A", "POST", "/signup", { body: A, expect: 200 });
await call("signup B", "POST", "/signup", { body: B, expect: 200 });
await call("signup duplicate", "POST", "/signup", { body: A, expect: 409 });
await call("signup invalid", "POST", "/signup", { body: { email: "bad" }, expect: 400 });
const la = await call("login A", "POST", "/login", { body: { email: A.email, password: A.password }, expect: 200 });
const lb = await call("login B", "POST", "/login", { body: { email: B.email, password: B.password }, expect: 200 });
await call("login wrong password", "POST", "/login", { body: { email: A.email, password: "nope-nope" }, expect: 401 });
const tA = la.data.token, tB = lb.data.token;
const idA = la.data.user._id ?? la.data.user.id, idB = lb.data.user._id ?? lb.data.user.id;

// Auth middleware
await call("no auth header", "GET", "/profile/", { expect: 401 });
await call("malformed token", "GET", "/profile/", { token: "abc", expect: 401 });

// Profile
await call("get profile", "GET", "/profile/", { token: tA, expect: 200 });
await call("update profile", "PUT", "/profile/", { token: tA, body: { profile: { displayName: "Contract A" } }, expect: 200 });

// Users
await call("user suggestions", "GET", "/users/suggestions", { token: tA, expect: 200 });
await call("nearby users", "GET", "/users/nearby", { token: tA, expect: 200 });
await call("popular users", "GET", "/users/popular", { token: tA, expect: 200 });
await call("user by id", "GET", `/users/${idB}`, { token: tA, expect: 200 });
await call("user by bad id", "GET", "/users/not-an-id", { token: tA, expect: 400 });

// Contacts
await call("send contact request", "POST", `/users/contacts/${idB}`, { token: tA, expect: 200 });
await call("send contact request again", "POST", `/users/contacts/${idB}`, { token: tA });
await call("sent requests", "GET", "/users/contacts/sent-requests", { token: tA, expect: 200 });
await call("incoming requests", "GET", "/users/contacts/requests", { token: tB, expect: 200 });
await call("accept bad action", "PUT", `/users/contacts/${idA}?action=maybe`, { token: tB, expect: 400 });
await call("accept request", "PUT", `/users/contacts/${idA}?action=accepted`, { token: tB, expect: 200 });
await call("contacts list", "GET", "/users/contacts/", { token: tA, expect: 200 });
await call("add favorite", "POST", `/users/contacts/favorite/${idB}`, { token: tA, expect: 200 });
await call("favorites list", "GET", "/users/contacts/favorites", { token: tA, expect: 200 });
await call("remove favorite", "DELETE", `/users/contacts/favorite/${idB}`, { token: tA, expect: 200 });
await call("block user", "POST", `/users/contacts/blockUnblock/${idB}`, { token: tA, expect: 200 });
await call("blocked list", "GET", "/users/contacts/blocked", { token: tA, expect: 200 });
await call("unblock user", "POST", `/users/contacts/blockUnblock/${idB}?action=unblock`, { token: tA, expect: 200 });

// Chats
const direct = await call("create direct chat", "POST", `/chats/direct/${idB}`, { token: tA, expect: 200 });
const group = await call("create group chat", "POST", "/chats/group", {
  token: tA, body: { name: "Contract Group", description: "ct", participants: [idB] }, expect: 200,
});
await call("create group no participants", "POST", "/chats/group", { token: tA, body: { name: "x", participants: ["bad"] }, expect: 400 });
await call("hub stats", "GET", "/chats/hub/stats", { token: tA, expect: 200 });
const chatId = direct.data?.chatId ?? direct.data?._id ?? direct.data?.id;
const groupId = group.data?.chatId ?? group.data?._id ?? group.data?.id;

// Groups / invites
await call("add group members", "POST", `/groups/${groupId}/add-members`, { token: tA, body: { userIDs: [idB] } });
await call("create invite", "POST", `/groups/${groupId}/invites`, { token: tA, expect: 200 });
const invites = await call("group invites", "GET", `/groups/${groupId}/invites`, { token: tA, expect: 200 });
const inviteCode = invites.data?.[0]?.inviteCode;
await call("all invites of user", "GET", "/groups/invites", { token: tB, expect: 200 });
await call("send invite to user", "POST", `/groups/invites/${inviteCode}/${idB}/send`, { token: tA });
await call("join group by invite", "POST", `/groups/join/${inviteCode}`, { token: tB });
await call("joined users by invite", "GET", `/groups/invites/${inviteCode}/joined`, { token: tA });
await call("delete invite", "DELETE", `/groups/invites/${inviteCode}`, { token: tA });

// WebSocket
const wsA = await openWS(tA);
const wsB = await openWS(tB);
await sleep(500);
results["WS connect welcome"] = { A: wsA.inbox.map((m) => ({ type: m.type, shape: shape(m) })) };
console.log(`WS    connect welcome              A:[${wsA.inbox.map((m) => m.type).join(",")}]`);
const sockets = { A: wsA, B: wsB };

await wsStep("join_chat", wsA, { type: "join_chat", chatId, senderId: idB, requestId: "r1" }, sockets);
const sent = await wsStep("send_message", wsA, { type: "send_message", chatId, content: "hello from contract test", messageType: "text", attachments: [], mentions: [], requestId: "r2" }, sockets, 800);
await wsStep("send_message group", wsA, { type: "send_message", chatId: groupId, content: "hi group", requestId: "r2g" }, sockets, 800);
await wsStep("set_typing", wsA, { type: "set_typing", chatId, metadata: { isTyping: true }, requestId: "r3" }, sockets);

const msgs = await call("chat messages", "GET", `/messages/${chatId}/messages?limit=50&offset=0`, { token: tA, expect: 200 });
const messageId = msgs.data?.messages?.[0]?._id ?? msgs.data?.messages?.[0]?.id;

await wsStep("mark_read", wsB, { type: "mark_read", chatId, metadata: { messageIds: [messageId] }, requestId: "r4" }, sockets);
await wsStep("add_reaction", wsB, { type: "add_reaction", chatId, metadata: { messageId, emoji: "👍" }, requestId: "r5" }, sockets);
await wsStep("remove_reaction", wsB, { type: "remove_reaction", chatId, metadata: { messageId, emoji: "👍" }, requestId: "r6" }, sockets);
await wsStep("edit_message", wsA, { type: "edit_message", chatId, requestId: "r7" }, sockets);
await wsStep("delete_message", wsA, { type: "delete_message", chatId, requestId: "r8" }, sockets);
await wsStep("update_chat", wsA, { type: "update_chat", chatId, requestId: "r9" }, sockets);
await wsStep("invite_user bad", wsA, { type: "invite_user", chatId: groupId, metadata: {}, requestId: "r10" }, sockets);
await wsStep("remove_user bad", wsA, { type: "remove_user", chatId: groupId, metadata: {}, requestId: "r11" }, sockets);
await wsStep("leave_chat", wsA, { type: "leave_chat", chatId, requestId: "r12" }, sockets);
await wsStep("unknown type", wsA, { type: "nope", requestId: "r13" }, sockets);
await wsStep("malformed json", wsA, "{not json", sockets);
await wsStep("send_message missing content", wsA, { type: "send_message", chatId, requestId: "r14" }, sockets);

wsB.close();
await sleep(400);
results["WS A after B disconnect"] = { A: wsA.inbox.map((m) => ({ type: m.type, shape: shape(m) })) };
console.log(`WS    after B disconnect           A:[${wsA.inbox.map((m) => m.type).join(",")}]`);
wsA.close();

// WS upgrade without auth must be rejected
try {
  await new Promise((resolve, reject) => {
    const ws = new WebSocket(WS_URL);
    ws.on("unexpected-response", (_q, res) => { record("ws no auth", { status: res.statusCode }, 401); resolve(); });
    ws.on("open", () => { record("ws no auth", { status: 101 }, 401); ws.close(); resolve(); });
    ws.on("error", reject);
  });
} catch (e) { record("ws no auth", { status: String(e.message) }, 401); }

// Logout revokes the token
await call("logout A", "POST", "/auth/logout", { token: tA, expect: 200 });
await call("revoked token", "GET", "/profile/", { token: tA, expect: 401 });

// Cleanup (also checks delete)
await call("delete profile B", "DELETE", "/profile/delete", { token: tB, expect: 200 });
const la2 = await call("re-login A", "POST", "/login", { body: { email: A.email, password: A.password }, expect: 200 });
await call("delete profile A", "DELETE", "/profile/delete", { token: la2.data.token, expect: 200 });

writeFileSync(OUT, JSON.stringify(results, null, 2));
console.log(`\n${failures ? failures + " unexpected status code(s)" : "all expected status codes matched"} — snapshot written to ${OUT}`);
process.exit(failures ? 1 : 0);
