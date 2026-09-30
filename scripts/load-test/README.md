# Load test

WebSocket load test for the ECHO server. Two phases against a running instance:

1. **Concurrency** — opens `CONNS` authenticated WebSocket connections across a
   small pool of users, reports connect success rate and connect-time
   percentiles, and cross-checks the hub's own `totalClients` via
   `GET /chats/hub/stats`.
2. **Latency** — provisions `PAIRS` user pairs, each with a direct chat; every
   sender fires `MSGS` messages and the harness times each
   `send_message` → server `response` round-trip (validation + persistence +
   reply), reporting p50/p95/p99 and sustained throughput.

The harness creates throwaway users (`lt_*@echo.test`) and best-effort deletes
every one it created at the end.

## Run

Start the stack and server first (from the repo root):

```bash
docker compose -f docker/docker-compose.yml up -d
go run ./cmd/server
```

Then:

```bash
cd scripts/load-test
npm install          # first time only (installs ws)
node loadtest.mjs http://localhost:8080
```

## Tunables (env vars)

| Var           | Default | Meaning                                             |
| ------------- | ------- | --------------------------------------------------- |
| `CONNS`       | `1000`  | Target concurrent connections (phase 1)             |
| `POOL`        | `20`    | Distinct users backing those connections            |
| `CONN_BATCH`  | `100`   | Connections opened per batch                        |
| `PAIRS`       | `50`    | Chat pairs in the latency phase (uses `2×PAIRS` users) |
| `MSGS`        | `40`    | Messages each sender sends                          |
| `MSG_GAP`     | `25`    | Milliseconds between a sender's messages            |
| `RTT_TIMEOUT` | `10000` | Max wait (ms) for a message's response before it's counted lost |

Example:

```bash
CONNS=2000 PAIRS=100 MSGS=50 node loadtest.mjs http://localhost:8080
```

The run prints a JSON summary. The figures quoted in
[`_docs/README.md`](../../_docs/README.md) come from five runs at the default
settings; latency varies run-to-run (first run after start carries warmup), so
prefer the median of a few runs over any single number.
