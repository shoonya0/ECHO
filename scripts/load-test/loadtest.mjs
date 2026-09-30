// Load test for the ECHO real-time chat server.
//
// Two phases against a running server:
//   1. Concurrency  — open N concurrent authenticated WebSocket connections,
//                     measure connect success rate + connect-time percentiles,
//                     and confirm the hub's own connection count.
//   2. Latency      — P chat pairs each send M messages; measure the
//                     send_message -> server "response" round-trip
//                     (validation + persistence + reply) as p50/p95/p99,
//                     plus sustained message throughput.
//
// It creates throwaway users, runs, prints a JSON summary, and best-effort
// deletes every user it created.
//
// Usage:
//   node loadtest.mjs [baseUrl]
// Tunables (env vars, with defaults):
//   CONNS=1000        target concurrent connections (phase 1)
//   POOL=20           distinct users backing those connections
//   CONN_BATCH=100    connections opened per batch
//   PAIRS=50          chat pairs (phase 2)  -> 2*PAIRS users, PAIRS senders
//   MSGS=40           messages each sender sends
//   MSG_GAP=25        ms between a sender's messages
//   RTT_TIMEOUT=10000 ms to wait for a message's response before counting it lost
//
// Example: CONNS=2000 PAIRS=100 MSGS=50 node loadtest.mjs http://localhost:8080

import WebSocket from "ws";

const BASE = process.argv[2] || process.env.BASE || "http://localhost:8080";
const API = `${BASE}/echo/v1`;
const WS_URL = API.replace(/^http/, "ws") + "/ws/chat";

const CONNS = int("CONNS", 1000);
const POOL = int("POOL", 20);
const CONN_BATCH = int("CONN_BATCH", 100);
const PAIRS = int("PAIRS", 50);
const MSGS = int("MSGS", 40);
const MSG_GAP = int("MSG_GAP", 25);
const RTT_TIMEOUT = int("RTT_TIMEOUT", 10000);

function int(name, def) {
  const v = process.env[name];
  return v === undefined || v === "" ? def : parseInt(v, 10);
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const createdTokens = []; // for cleanup

// ---------- HTTP helpers ----------

async function http(method, path, { token, body } = {}) {
  const headers = {};
  if (token) headers.Authorization = `Bearer ${token}`;
  if (body !== undefined) headers["Content-Type"] = "application/json";
  let res;
  try {
    res = await fetch(API + path, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch (e) {
    // e.g. ENOBUFS/EADDRNOTAVAIL when the OS is out of ephemeral ports at the ceiling
    return { status: 0, json: null, error: String(e?.cause?.code || e?.message || e) };
  }
  let json;
  try { json = await res.json(); } catch { json = null; }
  return { status: res.status, json };
}

let userSeq = 0;
async function makeUser() {
  const stamp = Date.now();
  const n = ++userSeq;
  const u = {
    email: `lt.${stamp}.${n}@echo.test`,
    username: `lt_${stamp}_${n}`,
    password: "loadtest123",
  };
  const s = await http("POST", "/signup", { body: u });
  if (s.status !== 200) throw new Error(`signup failed (${s.status}) ${JSON.stringify(s.json)}`);
  const l = await http("POST", "/login", { body: { email: u.email, password: u.password } });
  if (l.status !== 200) throw new Error(`login failed (${l.status})`);
  const token = l.json.data.token;
  const id = l.json.data.user._id ?? l.json.data.user.id;
  createdTokens.push(token);
  return { token, id };
}

// Run an async factory over `count` items with bounded concurrency.
async function pool(count, concurrency, fn) {
  const results = new Array(count);
  let next = 0;
  const workers = Array.from({ length: Math.min(concurrency, count) }, async () => {
    while (true) {
      const i = next++;
      if (i >= count) break;
      results[i] = await fn(i);
    }
  });
  await Promise.all(workers);
  return results;
}

// ---------- WS helpers ----------

function openWS(token, onMessage) {
  return new Promise((resolve, reject) => {
    const t0 = performance.now();
    const ws = new WebSocket(WS_URL, { headers: { Authorization: `Bearer ${token}` } });
    let settled = false;
    ws.on("open", () => { settled = true; resolve({ ws, connectMs: performance.now() - t0 }); });
    ws.on("message", (d) => { if (onMessage) { try { onMessage(JSON.parse(d.toString())); } catch {} } });
    ws.on("unexpected-response", (_q, res) => { if (!settled) reject(new Error(`upgrade ${res.statusCode}`)); });
    ws.on("error", (e) => { if (!settled) reject(e); });
  });
}

// ---------- percentiles ----------

function pct(sorted, p) {
  if (sorted.length === 0) return null;
  const idx = Math.min(sorted.length - 1, Math.ceil((p / 100) * sorted.length) - 1);
  return sorted[idx];
}
function stats(arr) {
  const s = [...arr].sort((a, b) => a - b);
  const sum = s.reduce((a, b) => a + b, 0);
  return {
    count: s.length,
    min: round(s[0]),
    p50: round(pct(s, 50)),
    p95: round(pct(s, 95)),
    p99: round(pct(s, 99)),
    max: round(s[s.length - 1]),
    mean: round(s.length ? sum / s.length : null),
  };
}
const round = (n) => (n == null ? null : Math.round(n * 100) / 100);

// ---------- phase 1: concurrency ----------

async function phaseConcurrency() {
  console.log(`\n[phase 1] concurrency: opening ${CONNS} connections across ${POOL} users (batch ${CONN_BATCH})`);
  const users = await pool(POOL, 10, () => makeUser());
  const sockets = [];
  const connectMs = [];
  let failures = 0;
  const errs = {};

  for (let base = 0; base < CONNS; base += CONN_BATCH) {
    const n = Math.min(CONN_BATCH, CONNS - base);
    const batch = await Promise.allSettled(
      Array.from({ length: n }, (_, k) => openWS(users[(base + k) % POOL].token))
    );
    for (const r of batch) {
      if (r.status === "fulfilled") { sockets.push(r.value.ws); connectMs.push(r.value.connectMs); }
      else { failures++; const m = String(r.reason?.message || r.reason).slice(0, 40); errs[m] = (errs[m] || 0) + 1; }
    }
    process.stdout.write(`\r  open=${sockets.length} fail=${failures}   `);
  }
  process.stdout.write("\n");

  // The hub registers clients on a single goroutine, each doing a DB + Redis
  // round-trip, so registration lags acceptance. Poll until it plateaus to
  // measure the true registration throughput and confirm all sockets register.
  const tReg = performance.now();
  let peak = 0, lastData = null, stable = 0;
  for (let i = 0; i < 120; i++) { // up to ~60s
    const hub = await http("GET", "/chats/hub/stats", { token: users[0].token });
    lastData = hub.json?.data ?? hub.json ?? null;
    const n = lastData?.totalClients ?? 0;
    if (n > peak) { peak = n; stable = 0; } else { stable++; }
    process.stdout.write(`\r  registered=${n}/${sockets.length}   `);
    if (n >= sockets.length || stable >= 4) break;
    await sleep(500);
  }
  process.stdout.write("\n");
  const registerMs = performance.now() - tReg;

  const held = sockets.length;
  for (const ws of sockets) { try { ws.close(); } catch {} }
  await sleep(500);

  return {
    target: CONNS,
    established: held,
    failed: failures,
    errorBreakdown: errs,
    connectMs: stats(connectMs),
    hubRegisteredPeak: peak,
    registerSettleMs: round(registerMs),
    registerRatePerSec: round((peak / registerMs) * 1000),
    hubStats: lastData,
  };
}

// ---------- phase 2: latency ----------

async function phaseLatency() {
  console.log(`\n[phase 2] latency: ${PAIRS} pairs x ${MSGS} msgs (gap ${MSG_GAP}ms)`);
  console.log(`  provisioning ${PAIRS * 2} users + ${PAIRS} direct chats ...`);

  const pairs = await pool(PAIRS, 20, async () => {
    const a = await makeUser();
    const b = await makeUser();
    const chat = await http("POST", `/chats/direct/${b.id}`, { token: a.token });
    const chatId = chat.json?.data?.chatId ?? chat.json?.data?._id ?? chat.json?.data?.id;
    if (!chatId) throw new Error(`direct chat failed (${chat.status})`);
    return { a, b, chatId };
  });

  // Open sockets + join chats.
  const latencies = [];
  let lost = 0, sendErrors = 0;
  const senders = [];

  for (const p of pairs) {
    const pending = new Map(); // requestId -> t0
    const sConn = await openWS(p.a.token, (msg) => {
      if (msg.type === "response") {
        const rid = msg.requestId || msg.data?.requestId;
        const t0 = pending.get(rid);
        if (t0 !== undefined) { latencies.push(performance.now() - t0); pending.delete(rid); }
      } else if (msg.type === "error") {
        const rid = msg.requestId || msg.data?.requestId;
        if (pending.delete(rid)) sendErrors++;
      }
    });
    const rConn = await openWS(p.b.token); // receiver present so broadcast fan-out is exercised
    senders.push({ p, ws: sConn.ws, rws: rConn.ws, pending });
  }

  // Both members join the chat so send permission checks pass.
  for (const s of senders) {
    s.ws.send(JSON.stringify({ type: "join_chat", chatId: s.p.chatId, senderId: s.p.b.id, requestId: "join_a" }));
    s.rws.send(JSON.stringify({ type: "join_chat", chatId: s.p.chatId, senderId: s.p.a.id, requestId: "join_b" }));
  }
  await sleep(500);

  const t0all = performance.now();
  await Promise.all(senders.map(async (s, si) => {
    for (let m = 0; m < MSGS; m++) {
      const rid = `p${si}_m${m}_${Date.now()}`;
      s.pending.set(rid, performance.now());
      try {
        s.ws.send(JSON.stringify({
          type: "send_message", chatId: s.p.chatId,
          content: `load ${si}:${m}`, messageType: "text", requestId: rid,
        }));
      } catch { sendErrors++; s.pending.delete(rid); }
      if (MSG_GAP) await sleep(MSG_GAP);
    }
  }));

  // Drain outstanding responses.
  const deadline = performance.now() + RTT_TIMEOUT;
  while (performance.now() < deadline) {
    const outstanding = senders.reduce((a, s) => a + s.pending.size, 0);
    if (outstanding === 0) break;
    await sleep(50);
  }
  const wallMs = performance.now() - t0all;
  lost = senders.reduce((a, s) => a + s.pending.size, 0);

  for (const s of senders) { try { s.ws.close(); } catch {}; try { s.rws.close(); } catch {} }
  await sleep(500);

  const sent = PAIRS * MSGS;
  return {
    pairs: PAIRS,
    messagesPerSender: MSGS,
    messagesSent: sent,
    responsesReceived: latencies.length,
    lost,
    sendErrors,
    wallMs: round(wallMs),
    throughputMsgPerSec: round((latencies.length / wallMs) * 1000),
    roundTripMs: stats(latencies),
  };
}

// ---------- cleanup ----------

async function cleanup() {
  console.log(`\n[cleanup] deleting ${createdTokens.length} test users ...`);
  await pool(createdTokens.length, 20, async (i) => {
    try { await http("DELETE", "/profile/delete", { token: createdTokens[i] }); } catch {}
  });
}

// ---------- main ----------

(async () => {
  const health = await fetch(`${BASE}/health`).then((r) => ({ status: r.status })).catch(() => ({ status: 0 }));
  if (health.status !== 200) { console.error(`server not healthy at ${BASE} (status ${health.status})`); process.exit(1); }
  console.log(`ECHO load test -> ${BASE}`);

  const summary = { base: BASE, when: new Date().toISOString() };
  try {
    summary.concurrency = await phaseConcurrency();
    summary.latency = await phaseLatency();
  } finally {
    await cleanup();
  }

  console.log("\n================ SUMMARY ================");
  console.log(JSON.stringify(summary, null, 2));
  process.exit(0);
})();
