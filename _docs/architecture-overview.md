# ECHO: architecture overview

Snapshot at `e1291b2` (main). VERIFIED = read in the code or run. Target = the direction
we want; the code doesn't follow it yet. Detail on pub/sub lives in
[PUBSUB_ARCHITECTURE.md](./PUBSUB_ARCHITECTURE.md), the wire protocol in
[WEBSOCKET_PROTOCOL.md](./WEBSOCKET_PROTOCOL.md).

## What it is

A Go chat backend: REST (Gin) plus one WebSocket endpoint, MongoDB for persistence,
Redis for token revocation and cross-instance pub/sub. Deployed as a Docker image to
Render (`render.yaml`, `autoDeploy: true` from `main`). The Android client lives
elsewhere; `Android/` holds only its planning docs.

## Module map (VERIFIED)

| Package | Role | Depends on |
|---|---|---|
| `cmd/server` | Entry: config → Mongo → indexes (background) → Redis → hub → Gin → graceful shutdown | everything below |
| `cmd/cleandb` | Dev tool: wipes Mongo collections and flushes Redis | config, database |
| `internal/config` | Env + optional `.env` (env wins) | — |
| `internal/router` | Route table (`/echo/v1/...`) | controller, middleware |
| `internal/middleware` | JWT auth (+ Redis blacklist), CORS, request logging | config, database, utils |
| `internal/controller` | HTTP + WebSocket handlers, request parsing, error mapping | services, realtime, models |
| `internal/services` | Business logic and MongoDB access (chats, messages, users, contacts, invites) | database, models, utils |
| `internal/realtime` | WebSocket hub, Redis pub/sub manager, in-memory presence | services, database, models |
| `internal/models` | Structs for Mongo docs, API payloads, WS messages; `Client`/`Hub` types | — |
| `internal/database` | Mongo/Redis clients, index specs | config |
| `internal/utils` | JWT, response helpers, default constructors, invite-code AES | config, models |

`services` reaches the hub only through the `Broadcaster` interface set at startup
(`services.SetBroadcaster`), which avoids an import cycle.

## Main flows (VERIFIED)

1. **Login:** `POST /login` → bcrypt check → 7-day HS256 JWT carrying user, profile,
   account status and presence claims. Middleware trusts these claims (see #33).
2. **Connect:** `GET /ws/chat` (Bearer header) → `Register` channel → hub goroutine
   loads `users.chats` from Mongo, subscribes `user:<id>` + `chat:<id>` Redis channels,
   adds the client to `ChatClients`.
3. **Send message:** WS `send_message` → membership check (`GetChat`) → insert message →
   update chat stats → publish to `chat:<id>` → every instance fans out to its local
   clients in `ChatClients[chatID]`.
4. **History:** `GET /messages/:chatID/messages?before=<id>` → membership check → cursor
   page on index `chatId_1__id_-1`.
5. **Groups and invites:** create group (creator + pending members + one AES invite
   code), add/remove members (owner/admin), join by invite code.

## Data model (VERIFIED)

- `users`: profile, presence, `contactInfo` ID lists, `chats` map (chatID → unread),
  `chatInvitations`. Unique index on `email`.
- `chats`: `participants` map keyed by user ID (role, status, denormalized user info),
  `inviteCode[]`, `readReceipts`, `typingUsers`, `stats`. No secondary indexes.
- `messages`: `chatId`, sender (denormalized), content, reactions map, `readBy` map.

Two sources of truth for membership: `chats.participants` (used by REST checks) and
`users.chats` (used by the hub to subscribe). They drift (#14, #17, #28).

## Target layers (not yet enforced)

| Layer | May import | Must not import |
|---|---|---|
| controller | services, realtime, models, utils | database |
| services | database, models, utils | controller, realtime, gin |
| realtime | services (via interfaces), models, database | controller |
| models | stdlib, bson | everything internal |

Today `controller/auth.controller.go` talks to `database` directly. Enforcement would be
`depguard` in golangci-lint (deferred; see the setup log).

## Riskiest areas

| Area | Where | Issue |
|---|---|---|
| Hub map concurrency | `internal/realtime/pubsub.go:283`, `hub.go:230` | #19 |
| Disconnect cleanup leak | `internal/realtime/hub.go:242` | #20 |
| One Redis conn per channel | `internal/realtime/pubsub.go:163` | #21 |
| Roles never assigned | `internal/utils/defaultInitialization.go:141` | #18 |
| WS handlers without membership checks | `internal/controller/websocket.controller.go:386-533` | #23 |
| Block model | `internal/services/contact.service.go:321` | #26, #27 |
| Signup `_id` + account deletion | `internal/controller/auth.controller.go:24`, `services/user.service.go:108` | #34, #35 |
| JWT-only account status | `internal/middleware/auth.go:20` | #33 |
| Non-transactional multi-doc writes | `services/chat.service.go`, `contact.service.go` | #37 |
| Invite codes (hard-coded key) | `internal/utils/defaultInitialization.go:149` | #9, #15 |
