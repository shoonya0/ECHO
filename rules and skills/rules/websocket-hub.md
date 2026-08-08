# WebSocket hub & real-time chat — ECHO

## Rule

Real-time communication goes through the **EnhancedHub** singleton with Redis pub/sub for multi-instance support:

1. **EnhancedHub** manages WebSocket clients (register, unregister), chat rooms, user-client mappings, and presence.
2. **PubSubManager** handles cross-instance message delivery via Redis channels (`chat:*`, `user:*`, `presence:global`, `system:global`).
3. Clients are identified by a unique `client.ID` (UUID) and bound to a `userID` (bson.ObjectID).
4. Messages follow a typed protocol (`send_message`, `join_chat`, `leave_chat`, `typing`, `mark_read`, etc.) with `requestId` correlation.

## Why

- The hub is the concurrency hot path — every WebSocket message flows through it. The singleton pattern ensures one source of truth for connection state.
- Redis pub/sub is what makes ECHO horizontally scalable — messages published to a Redis channel are received by all server instances subscribed to that channel.
- The `requestId` correlation in WebSocket messages lets clients match responses to requests, essential for a chat app with concurrent operations.

## How to apply

### 1. The EnhancedHub singleton

```go
// internal/services/websocket.hub.go

var (
    enhancedHubInstance *EnhancedHub
    enhancedHubOnce     sync.Once
)

func GetHubInstance() *EnhancedHub {
    enhancedHubOnce.Do(func() {
        rdb := redis.NewClient(&redis.Options{
            Addr:     objects.MainConfiguration.RedisUri,
            Password: objects.MainConfiguration.RedisPass,
            DB:       0,
        })
        // Panics if Redis is unreachable — hub cannot function without it

        instanceID := fmt.Sprintf("instance-%s-%d", uuid.New().String()[:8], time.Now().Unix())

        baseHub := &models.Hub{
            Clients:         make(map[string]*models.Client),
            ChatClients:     make(map[string]map[string]*models.Client),
            UserClients:     make(map[string]map[string]*models.Client),
            UserInfoCache:   make(map[string]*models.UserDisplayInfo),
            CacheExpiry:     make(map[string]time.Time),
            Register:        make(chan *models.Client, 1000),
            Unregister:      make(chan *models.Client, 1000),
            UserActiveChats: make(map[string][]string),
        }

        ctx, cancel := context.WithCancel(context.Background())
        baseHub.Ctx = ctx
        baseHub.Cancel = cancel

        zapLogger, _ := zap.NewProduction()

        enhancedHubInstance = &EnhancedHub{
            Hub:         baseHub,
            redisClient: rdb,
            logger:      zapLogger,
            instanceID:  instanceID,
        }

        enhancedHubInstance.pubSubManager = NewPubSubManager(rdb, baseHub, zapLogger, instanceID)
    })
    return enhancedHubInstance
}
```

Started in `main.go`:
```go
go func() {
    services.GetHubInstance().Start()
}()
```

`Start()` initializes pub/sub, launches `runHub()`, `cleanupInactiveConnections()`, and `syncPresenceWithRedis()` goroutines.

### 2. Client lifecycle

**Registration** (`hub.Register <- client`):
1. Query DB for user's chat IDs (`getUserChats`) — done **before** acquiring the mutex to avoid deadlocks.
2. Subscribe to `user:{userID}` and `chat:{chatID}` Redis channels via PubSubManager.
3. Under mutex: add to `Clients`, `UserClients[userID]`, and `ChatClients[chatID]` maps.
4. Update presence via `GetPresenceInstance().Set(...)` and publish presence update.
5. Send welcome `WSMessageTypeResponse` to the client.

**Unregistration** (`hub.Unregister <- client`):
1. Under mutex: remove from all chat maps.
2. If no more clients for this user on this instance: unsubscribe from Redis channels, delete presence, publish `offline`.
3. Close client's send channel.

### 3. Client and chat maps

```
Clients         map[clientID] -> *Client              // all connections
UserClients     map[userHex] -> map[clientID]*Client  // user → connections
ChatClients     map[chatHex] -> map[clientID]*Client  // chat → connections
UserActiveChats map[userHex] -> []chatID              // user → active chats
UserInfoCache   map[userHex] -> *UserDisplayInfo      // 5-min TTL cache
```

The `Clients` map is the authoritative registry. `UserClients` and `ChatClients` are derived indices for fast lookup.

### 4. WebSocket message protocol

**Request types (client → server):**

| Type | Purpose | Required fields |
| --- | --- | --- |
| `send_message` | Send a chat message | `chatId`, `content` |
| `join_chat` | Join a chat room | `senderId` (target user) |
| `leave_chat` | Leave a chat room | `chatId` |
| `set_typing` | Typing indicator | `chatId`, metadata `isTyping` |
| `mark_read` | Mark messages read | `chatId`, metadata `messageIds` |
| `add_reaction` | React to message | `chatId`, metadata `messageId`, `emoji` |
| `remove_reaction` | Remove reaction | `chatId`, metadata `messageId`, `emoji` |
| `edit_message` | Edit message content | (TODO) |
| `delete_message` | Soft-delete message | (TODO) |
| `invite_user` | Invite to group | (TODO) |
| `remove_user` | Remove from group | (TODO) |
| `update_chat` | Update chat settings | (TODO) |

**Response/event types (server → client):**

| Type | Purpose |
| --- | --- |
| `response` | Success response (correlated via `requestId`) |
| `error` | Error response (with `code` and `message`) |
| `chat` | Incoming chat message broadcast |
| `presence` | Presence status change |
| `typing` | Typing indicator broadcast |
| `delivery` | Read receipt / delivery status |
| `reaction` | Reaction added/removed |
| `join` | User joined chat |
| `leave` | User left chat |

**Correlation:** Every `MessageRequest` carries a `requestId`. The server echoes it in the `response`/`error` frame so the client can match.

### 5. Message flow example: sending a message

1. Client sends `{"type":"send_message","chatId":"...","content":"hello","requestId":"r1"}`.
2. `handleClientRead` unmarshals to `MessageRequest`, routes to `handleSendMessage`.
3. Controller validates: chat ID format, user permissions, content length/type.
4. Controller calls `services.SendMessage(ctx, chatID, senderID, content, ...)`.
5. Service inserts message into MongoDB (`MessageColl`), updates chat's `lastMessageId` + `stats.messageCount`.
6. Service broadcasts via `BroadcastToChat` → `PubSubManager.PublishToChat`.
7. All instances subscribed to `chat:{chatID}` receive the message and deliver to their local clients.

### 6. Redis pub/sub channel conventions

| Channel | Pattern | Purpose |
| --- | --- | --- |
| User | `user:{userHex}` | Direct messages and notifications for a specific user |
| Chat | `chat:{chatHex}` | Messages broadcast to all members of a chat |
| Presence | `presence:global` | Global presence status updates |
| System | `system:global` | System-wide broadcasts (shutdown, config reload) |
| Instance | `instance:{instanceID}` | Targeted messages for this server instance |

Subscriptions are reference-counted: a chat channel is only unsubscribed when no more local clients are in that chat.

### 7. Presence management

Two systems track presence:

1. **`PresenceManager`** (`internal/services/presence.go`) — in-memory `map[bson.ObjectID]UserPresence`, sync.Once singleton. Updated on connect/disconnect.
2. **Redis presence keys** — `presence:{userHex}` with TTL 30 minutes, synced every 30 seconds by `syncPresenceWithRedis()`. Provides persistence across server restarts and multi-instance sharing.

Presence updates are published to the `presence:global` channel, which then fans out to all active chat channels the user is in.

### 8. Inactive connection cleanup

Every 30 seconds, `performCleanup()` scans all clients. Any client with `LastActivity` older than 5 minutes is sent to the `Unregister` channel. This prevents stale connections from accumulating.

### 9. User info caching

The hub maintains a `UserInfoCache` (5-minute TTL) to avoid database lookups for display info (username, avatar). Checked on every registration, typing indicator, and reaction broadcast. Misses fall back to `GetUserDisplayInfoFromDB`.

### 10. Thread safety

- `hub.Mutex` (RWMutex) protects all client maps (`Clients`, `ChatClients`, `UserClients`, `UserActiveChats`, `UserInfoCache`).
- **Critical rule:** External calls (DB queries, Redis operations, presence lookups) must happen **before** acquiring the mutex to avoid deadlocks.
- The `Register` and `Unregister` channels serialize hub mutations — only `runHub()` processes them.
- Client `Send` channel (buffered, capacity 10) is used with `select`/`default` to avoid blocking the hub on slow clients.

### 11. Hub restart

If the hub's context is cancelled (e.g., Redis connection loss), the hub can be restarted:
```go
select {
case <-hub.Ctx.Done():
    hub.Restart()  // Creates new context, restarts goroutines
default:
}
```

### 12. Hub stats (for monitoring)

```go
stats := services.GetHubInstance().GetStats()
// Returns: instanceId, totalClients, totalChats, totalUsers, cacheSize, subscribedChannels, timestamp
```

Exposed at `GET /echo/v1/chats/hub/stats`.