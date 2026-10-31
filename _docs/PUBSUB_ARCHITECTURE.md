# Real-time Architecture (WebSocket + Redis Pub/Sub)

## Overview

Each server instance keeps its own WebSocket connections and uses Redis Pub/Sub to
reach clients connected to other instances. A message is saved to MongoDB first, then
published to the chat's Redis channel; every instance with members of that chat
online delivers it to its local connections.

Redis is required: the server exits at startup if it can't connect.

## Components

### WebSocket hub (`internal/realtime/hub.go`)

A process-wide singleton, `realtime.GetHubInstance()`, started once in
`cmd/server/main.go` with `hub.Start()`.

- **Client lifecycle**: clients are added and removed through the buffered
  `Register` and `Unregister` channels (capacity 1000), handled by one run loop.
- **Indexes**: `Clients` (by connection ID), `UserClients` (by user ID, so one user can
  have several connections) and `ChatClients` (by chat ID), guarded by a
  `sync.RWMutex`.
- **Auto-join on connect**: the hub reads the chat IDs in the user's `chats` field
  (`services.GetUserChats`) and subscribes to `chat:<id>` for each. See
  [Which chats a user receives](#which-chats-a-user-receives).
- **User info cache**: display info used in events, cached for 5 minutes.
- **Background jobs** (every 30 seconds): write each connected user's presence to Redis,
  and disconnect clients that have been inactive for 5 minutes.

### Pub/Sub manager (`internal/realtime/pubsub.go`)

Owns the Redis subscriptions and routes incoming Redis messages to local clients.
Each instance subscribes to only the channels its connected clients need, and
unsubscribes when the last client of a chat leaves.

### Presence (`internal/realtime/presence.go`)

An in-memory map of `realtime.UserPresence` per user for this instance, plus the
`presence:<userID>` Redis hash written by the hub's sync job (2-minute TTL).

### Chat service (`internal/services/chat.service.go`)

Persists messages, typing state and read receipts, then calls
`services.BroadcastToChat(chatID, msg)`. That goes through the `Broadcaster`
interface, which `main.go` sets to the hub with `services.SetBroadcaster(hub)`, so the
services package doesn't import `realtime`.

## Redis channels

| Channel | Published by | Used for |
| ------- | ------------ | -------- |
| `chat:<chatID>` | `BroadcastToChat` (messages, typing, read receipts, reactions, `chat_updated`), presence fan-out | Everything delivered in real time. Direct chats use it too: a direct chat is a chat with two participants. |
| `presence:global` | The hub, when a user connects or disconnects | The instance holding that user's connections re-publishes the update to each of their chats' `chat:<id>` channels. |
| `user:<userID>` | Nothing yet | Subscribed per connected user. The routing to that user's connections exists, but no code publishes to it. |
| `system:global` | Nothing yet | Subscribed by every instance; the handler only logs. |
| `instance:<instanceID>` | Nothing yet | Subscribed by each instance for itself; the handler only logs. |

## Message flow

### Chat message (direct or group)

```
Client A ── send_message ──► Instance 1
                              ├─ MongoDB: insert message, update chat's lastMessage
                              └─ Redis PUBLISH chat:<chatID>
                                   ├─► Instance 1 ─► local clients in ChatClients[chatID]
                                   └─► Instance 2 ─► local clients in ChatClients[chatID]
```

The sender's own instance receives the message back through Redis, the same as every
other instance; there is no separate local shortcut.

### Presence

```
User A connects to Instance 1
  └─ PUBLISH presence:global
       └─ Instance 1 (holds A's active chats) ─► PUBLISH chat:<id> for each chat
```

### Liveness

The server pings each connection every 54 seconds. A connection that sends nothing,
not even a pong, for 60 seconds hits its read deadline and is closed. Separately, the
hub's cleanup job removes clients with no activity for 5 minutes.

### Slow clients

Each connection has a send buffer of 10 messages (`Client.Send`). Delivery uses
`Client.TrySend`, which never blocks: when the buffer is full the client is
disconnected and unregistered instead of stalling delivery to everyone else.

## Which chats a user receives

On connect, a client is subscribed to the chats listed in the user's `chats` field.
That field is written when:

- a direct chat is created (both users),
- a group is created (the creator only),
- a user joins a group with an invite code.

It is cleared for users removed from a group. Members added at group creation or with
`add-members` / `invite_user` are stored in the chat's `participants` with
`requestStatus: pending` but are **not** added to their own `chats` list, so they don't
get that group's live events. They can still load its history over REST, because that
check uses `participants`. Accepting or declining an invite is
[#5](https://github.com/shoonya0/ECHO/issues/5).

`join_chat` subscribes the client to a single chat during a session; it only ever
creates or opens a direct chat between the caller and `senderId`.

## Startup and shutdown

```go
// cmd/server/main.go (abridged)
database.ConnectRedis(ctx)          // fatal on failure
hub := realtime.GetHubInstance()
services.SetBroadcaster(hub)
hub.Start()                         // fatal on failure
// ... HTTP server runs until SIGINT/SIGTERM ...
srv.Shutdown(shutdownCtx)           // 10s timeout
database.CloseRedis()
database.DisconnectMongo(shutdownCtx)
```

The hub has no `Stop` method; it ends with the process. Connected clients see the
socket close and should reconnect, possibly to another instance.

Redis is configured with `REDIS_URI` (a `host:port` or a `redis://` / `rediss://` URL,
default `localhost:6379`) and `REDIS_PASS`; see `internal/config/config.go` and
`internal/database/redis.go`.

## Monitoring

`GET /echo/v1/chats/hub/stats` (requires a JWT) returns this instance's numbers only:

```bash
curl -H "Authorization: Bearer <jwt_token>" http://localhost:8080/echo/v1/chats/hub/stats
```

```json
{
  "success": true,
  "message": "Hub statistics retrieved successfully",
  "data": {
    "instanceId": "…",
    "totalClients": 150,
    "totalChats": 45,
    "totalUsers": 120,
    "cacheSize": 120,
    "subscribedChannels": 167,
    "channels": ["presence:global", "system:global", "instance:…", "user:…", "chat:…"],
    "timestamp": "2026-10-31T12:00:00Z"
  }
}
```

## Troubleshooting

1. **A member gets no live messages for a group.** Check that the group is in their
   user document's `chats` field; pending members aren't
   ([see above](#which-chats-a-user-receives)). Then check that `chat:<id>` appears in
   `channels` from hub stats on the instance they're connected to.
2. **Clients get disconnected under load.** A full 10-message send buffer drops the
   client; look for `Client send channel full, disconnecting` in the logs.
3. **Messages not delivered at all.** Check Redis connectivity; every delivery,
   including same-instance, goes through Redis.

## Not implemented

- Rate limiting on the socket or REST API.
- Offline delivery or push notifications: a message reaches only connected clients;
  others load it from history.
- Publishing to `user:<id>`, `system:global` or `instance:<id>`.
- Message editing and deletion, and the `set_presence` request.

## Related documentation

- [WebSocket Protocol Reference](./WEBSOCKET_PROTOCOL.md) — message types and payloads
- [README](./README.md) — project overview, setup and REST API
