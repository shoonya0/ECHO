# Echo - Real-Time Chat System

A powerful, scalable real-time chat system built with Go, featuring WebSocket support, group messaging, and comprehensive presence tracking.

## Key Features

### Core Functionality

- **Real-Time Communication** via WebSockets
- **Multiple Chat Types**: Direct messages, group chats, and channels
- **User Presence & Activity Tracking**
- **Message Persistence** with MongoDB
- **Caching Layer** with Redis
- **JWT-Based Authentication**

### Enhanced Chat Features

- **Typing Indicators**
- **Read Receipts**
- **Message Reactions**
- **User Presence Tracking**
- **File Attachments Support**
- **Message History**
- **Message Editing/Deletion** (planned)
- **Group Invite System** with invite codes

### Performance & Scalability

Measured against a single server instance with the load harness in
[`scripts/load-test`](../scripts/load-test) (see that folder's README for how to
reproduce):

| Metric                                   | Result                    |
| ---------------------------------------- | ------------------------- |
| Concurrent WebSocket connections         | 14,000 held, 0 failed (hub-confirmed) |
| Connection registration throughput       | ~1,750 registrations/s    |
| Connection setup time (p95)              | ~85–195 ms                |
| Message round-trip — send → persisted → server ack (p50) | ~12 ms    |
| Message round-trip (p95)                 | ~16–50 ms                 |
| Message round-trip (p99)                 | ~20–70 ms                 |
| Sustained message throughput            | ~1,600–1,900 msg/s        |
| Messages lost / dropped                  | 0 (across all runs)       |

Verified at 1k / 5k / 8k / 14k concurrent connections; message-latency ranges span
several runs at each level (the first run after start carries warmup). At 14k the hub
registered and held every connection with zero failures and message p95 stayed ~16 ms.
The only ceiling hit on a single box was the OS ephemeral-port limit — connections
climbed to ~15,300 before the *client host* ran out of loopback ports (`ENOBUFS`), not
any server-side failure. Registration is the ramp bottleneck, not steady state: the hub
registers clients on a single goroutine that does one MongoDB lookup + Redis subscribe
per connect (`hub.go`), capping the ramp at ~1,750 conn/s while established connections
keep messaging fast.

Test conditions: one server instance, Dockerized MongoDB + Redis, all over loopback on
an Intel i5-10400F (12 threads) / 16 GB Windows 11 host. These are development-hardware
figures for regression tracking, not a production capacity guarantee — throughput scales
horizontally across instances via Redis pub/sub.

The numbers rest on: an in-process WebSocket hub for connection management, user-info
caching with a 5-minute TTL, batched user-data lookups, `sync.RWMutex`-guarded shared
state, automatic cleanup of inactive connections, and Redis pub/sub for cross-instance
fan-out.

## Architecture

### Core Components

1. **WebSocket Hub** (`internal/realtime/hub.go`)
   - Centralized connection management with channel-based registration
   - Real-time message broadcasting via `BroadcastToChat()`
   - Client lifecycle handling (Register/Unregister channels)
   - Thread-safe operations with sync.RWMutex
   - User info cache with 5-min TTL

2. **Chat Service** (`internal/services/chat.service.go`)
   - Message persistence and retrieval
   - Chat room management (direct, group, channel)
   - Permission handling per chat type
   - Real-time broadcasting integration

3. **User & Contact Services** (`internal/services/user.service.go`, `contact.service.go`)
   - Profiles, user suggestions and batched user lookups (`GetUsersByIDs`)
   - Contact requests, favorites and blocking
   - JWT revocation on logout (Redis blacklist with the token's remaining lifetime)
   - The 5-minute user display-info cache lives in the hub (`UserInfoCache`)

4. **PubSub Manager** (`internal/realtime/pubsub.go`)
   - Redis Pub/Sub for cross-instance messaging
   - Channel management (user:, chat:, presence:global, system:global)
   - Multi-instance coordination

5. **Presence Service** (`internal/realtime/presence.go`)
   - Real-time user status tracking
   - Activity monitoring
   - Automatic cleanup of inactive users

See also:
- [WebSocket Protocol Reference](./WEBSOCKET_PROTOCOL.md)
- [Pub/Sub Architecture](./PUBSUB_ARCHITECTURE.md)

## Getting Started

### Prerequisites

- Go 1.24.3 or higher
- MongoDB
- Redis (required: it backs token revocation on logout and WebSocket pub/sub;
  the server exits at startup if it can't connect)
- Docker (optional, to run MongoDB and Redis locally)

### Installation

1. **Clone the Repository**

   ```bash
   git clone https://github.com/shoonya0/ECHO.git
   cd ECHO
   ```

2. **Start MongoDB and Redis**

   ```bash
   docker compose -f docker/docker-compose.yml up -d
   ```

   This starts `echo-mongodb` (user/password `echo`/`echo`) on 27017 and `echo-redis`
   on 6379.

3. **Configure Environment**

   ```bash
   cp .env.example .env
   ```

   | Variable     | Default (`.env.example`)              | Notes                                  |
   | ------------ | ------------------------------------- | -------------------------------------- |
   | `PORT`       | `8080`                                |                                        |
   | `DB_URI`     | `mongodb://echo:echo@localhost:27017` | Matches the docker-compose credentials |
   | `REDIS_URI`  | `localhost:6379`                      | `host:port` or a `redis://`/`rediss://` URL |
   | `REDIS_PASS` | empty                                 |                                        |
   | `JWT_SECRET` | `echo-dev-secret`                     | Use a long random value outside local dev |
   | `LOG_FILE`   | `cmd/server/logs/server.log`          | Leave empty to log to stdout           |
   | `LOG_LEVEL`  | `debug`                               | `info` for deployment                  |

   Real environment variables override `.env`.

4. **Run the Server**

   ```bash
   go run ./cmd/server
   # or build a binary
   go build -o build/echo-server ./cmd/server && ./build/echo-server
   ```

   MongoDB indexes are created in the background on startup (see
   [Performance Optimizations](#performance-optimizations)).

5. **Reset local data (optional)**

   ```bash
   go run ./cmd/cleandb -dry-run   # show what would be wiped
   go run ./cmd/cleandb            # wipe every MongoDB collection and flush Redis
   ```

   Local development only.

### Deployment

`Dockerfile` builds the server image, and `render.yaml` is a Render Blueprint (web service
plus Render Key Value for Redis; MongoDB on Atlas). The health check is `GET /health`.

## API Endpoints

Every route except `login`, `signup` and `/health` requires
`Authorization: Bearer <jwt>`.

### Authentication

```
POST /echo/v1/signup        - User registration (409 if the email is taken)
POST /echo/v1/login         - User login, returns a 7-day JWT
POST /echo/v1/auth/logout   - Revoke the current JWT (Redis blacklist)
```

### Health

```
GET /health                 - Liveness check, returns {"status":"ok"} (no auth)
```

### WebSocket (Real-Time Chat)

```
GET /echo/v1/ws/chat        - Main WebSocket endpoint (JWT via Authorization header)
```

### Profile

```
GET    /echo/v1/profile/         - Current user's profile
PUT    /echo/v1/profile/         - Update current user's profile (whitelisted fields only)
DELETE /echo/v1/profile/delete   - Delete current user's profile
```

### User Discovery

```
GET /echo/v1/users/suggestions   - User suggestions (paginated)
GET /echo/v1/users/nearby        - Nearby users (stub, needs location)
GET /echo/v1/users/popular       - Popular users (stub)
GET /echo/v1/users/:id           - A user's public profile
```

### Contacts

```
GET    /echo/v1/users/contacts/                     - Contact list
GET    /echo/v1/users/contacts/requests             - Incoming contact requests
GET    /echo/v1/users/contacts/sent-requests        - Sent contact requests
GET    /echo/v1/users/contacts/blocked              - Blocked users
GET    /echo/v1/users/contacts/favorites            - Favorite contacts
POST   /echo/v1/users/contacts/:targetUserId        - Send contact request
PUT    /echo/v1/users/contacts/:requestId?action=accepted|declined - Answer a request
DELETE /echo/v1/users/contacts/:contactId           - Remove contact
POST   /echo/v1/users/contacts/blockUnblock/:userId - Block or unblock a user
POST   /echo/v1/users/contacts/favorite/:userId     - Add to favorites
DELETE /echo/v1/users/contacts/favorite/:userId     - Remove from favorites
```

### Chat Management

```
POST   /echo/v1/chats/direct/:userId    - Create direct chat
POST   /echo/v1/chats/group             - Create group chat
GET    /echo/v1/chats/hub/stats         - Get hub statistics (monitoring)
```

### Messages

```
GET /echo/v1/messages/:chatID/messages   - Get chat messages (cursor pagination)
```

Messages come newest first. Query params: `limit` (1-100, default 50) and
`before` (a message ID; omit it for the newest page). The response's
`pagination` is `{ limit, count, hasMore, nextCursor }`: to load older
messages, pass `nextCursor` back as `before`. `nextCursor` is `null` on the
last page. Cursors stay stable while new messages arrive, unlike offsets.
Only participants of the chat can read it; anyone else gets `404 Chat not found`,
the same as for a chat that doesn't exist.

### Group Management

```
POST   /echo/v1/groups/:groupID/add-members   - Add group members (owner/admin)
POST   /echo/v1/groups/:groupID/invites       - Create invite code
GET    /echo/v1/groups/:groupID/invites       - List invite codes
DELETE /echo/v1/groups/invites/:inviteID      - Delete invite code
GET    /echo/v1/groups/invites/:inviteID/joined       - Get joined users by invite
POST   /echo/v1/groups/invites/:inviteID/:userID/send - Send invite to user
POST   /echo/v1/groups/join/:inviteCode       - Join group via invite code
GET    /echo/v1/groups/invites                - Get all invites of user
```

How group membership works today:

- The creator is the `owner` with `requestStatus: accepted`. Members added at creation
  or through `add-members` / `invite_user` are stored in `participants` with
  `requestStatus: pending` and read/write permissions.
- `add-members` takes `{ "userIDs": ["..."] }`. The caller must be an `owner` or
  `admin` of the group (`403` otherwise). Users already in the group are skipped,
  so their role and status are kept. If every user is already in the group, the
  call returns `409`.
- Nothing reads `requestStatus` yet, and there is no endpoint to accept or decline a
  group invite, so pending members already have full access. See
  [Known Issues](#known-issues).
- Invite codes expire after 24 hours. Joining by code adds the user as `accepted`.
  The code must be one the group created and hasn't deleted or let expire;
  anything else gets `400 Invalid invite`. Anyone already in `participants`,
  including pending members, gets `409`.

## WebSocket Message Types (Summary)

For full protocol details, see [WEBSOCKET_PROTOCOL.md](./WEBSOCKET_PROTOCOL.md).

### Request Types (Client → Server)
- `send_message` — Send a chat message
- `join_chat` — Join a chat room
- `leave_chat` — Leave a chat room
- `set_typing` — Set typing indicator
- `mark_read` — Mark messages as read
- `add_reaction` — Add emoji reaction
- `remove_reaction` — Remove emoji reaction
- `invite_user` — Add users to a group (owner/admin only)
- `remove_user` — Remove users from a group (owner/admin only)
- `edit_message` — Edit a message (returns `NOT_IMPLEMENTED`)
- `delete_message` — Delete a message (returns `NOT_IMPLEMENTED`)
- `update_chat` — Update chat settings (returns `NOT_IMPLEMENTED`)

`set_presence` is defined in the models but not handled; sending it returns
`UNKNOWN_REQUEST`. Message history is loaded over REST, not the socket.

### Response Types (Server → Client)
- `message` — New chat message
- `typing` — Typing indicator
- `join` / `leave` — User join/leave events
- `presence` — User presence update
- `delivery` — Message delivery status
- `reaction` — Message reaction event
- `chat_updated` — Group membership changed (users invited or removed)
- `response` — Request acknowledgment
- `error` — Error notification

## Security Features

- **JWT Authentication** (HS256, `Authorization: Bearer` header) for every route except
  login, signup and `/health`; logout revokes the token through a Redis blacklist
- **Chat membership checks**: message history is returned only to participants
  (non-members get `404`, so chat IDs can't be probed)
- **Role-based group management**: owner, admin, member (see
  [Known Issues](#known-issues) for gaps)
- **Input Validation** for requests (content length, type, attachments; profile updates
  limited to whitelisted fields)
- **Passwords** hashed with bcrypt
- **CORS**: any origin, credentials off (auth uses Bearer tokens, not cookies)

There is no rate limiting.

## Error Codes

| Code                      | Description                           |
|---------------------------|---------------------------------------|
| `INVALID_REQUEST`         | Missing or invalid request parameters |
| `INVALID_CHAT_ID`         | Chat ID format is invalid             |
| `INVALID_MESSAGE_ID`      | Message ID format is invalid          |
| `PERMISSION_DENIED`       | User lacks required permissions       |
| `PERMISSION_CHECK_FAILED` | Error validating permissions          |
| `INVALID_CONTENT`         | Message content validation failed     |
| `MESSAGE_FAILED`          | Failed to send message                |
| `REACTION_FAILED`         | Failed to add/remove reaction         |
| `READ_FAILED`             | Failed to mark messages as read       |
| `INVITE_FAILED`           | Failed to add users to a group        |
| `REMOVE_FAILED`           | Failed to remove users from a group   |
| `NOT_IMPLEMENTED`         | Feature not yet implemented           |
| `UNKNOWN_REQUEST`         | Unrecognized request type             |
| `PARSE_ERROR`             | Failed to parse WebSocket message     |
| `CHANNEL_FULL`            | Client send channel is full           |

## Testing

### Go Tests

```bash
go test ./internal/... -count=1
```

Some service tests use a real MongoDB: they create a throwaway database and drop it
afterwards. They use `TEST_DB_URI` (default: the docker-compose MongoDB) and are skipped
when MongoDB isn't reachable.

### API Contract Test

`scripts/contract-test` exercises the REST and WebSocket API end to end with two
throwaway users and records the shape of every response, so you can diff two runs:

```bash
cd scripts/contract-test && npm install
node run.mjs http://localhost:8080 before.json
node diff.mjs before.json after.json
```

### Load Test

`scripts/load-test` is the WebSocket load harness behind the
[performance figures](#performance--scalability); see its README.

### Manual WebSocket Testing

```bash
wscat -c "ws://localhost:8080/echo/v1/ws/chat" -H "Authorization: Bearer <jwt_token>"
```

## Monitoring

### Health

```bash
curl http://localhost:8080/health
```

### Hub Statistics

```bash
curl -H "Authorization: Bearer <jwt_token>" http://localhost:8080/echo/v1/chats/hub/stats
```

Response:
```json
{
  "totalClients": 150,
  "totalRooms": 45,
  "totalUsers": 120,
  "timestamp": "2024-01-01T12:00:00Z"
}
```

## Performance Optimizations

1. **Caching Layer** — User information cached with 5-minute TTL
2. **Database Efficiency** — Minimal embedded data, and indexes created on startup by
   `database.EnsureIndexes` (`internal/database/indexes.go`):
   - `messages {chatId: 1, _id: -1}`: message history and its cursor
   - `users {email: 1}` (unique): login and signup

   They build in the background (10-minute timeout), so startup isn't blocked. Creating
   them is idempotent; a failure on one collection doesn't stop the others, and is
   logged while the server keeps running (for example duplicate emails blocking the
   unique index). A concurrent duplicate signup returns `409`, not `500`.
3. **Cursor Pagination** — Message history pages by `_id < before` on the index above,
   so every page costs O(limit) and pages don't shift as new messages arrive
4. **WebSocket Optimizations** — Efficient connection pooling, automatic cleanup of inactive connections, thread-safe operations

## Known Issues

Open on GitHub; details are in each issue.

| Issue | Summary | Status |
| ----- | ------- | ------ |
| [#5](https://github.com/shoonya0/ECHO/issues/5) | No way to accept/decline a group invite; pending members have full access | Open |

Also not implemented yet: message editing/deletion, chat settings updates, the
`set_presence` request, and the nearby/popular user lists.

## Development Guidelines

- Clean Architecture principles
- Interface-driven development
- Proper error handling and wrapping
- Follow Go idiomatic patterns
- Thread-safe operations with proper mutex usage

## License

This project is part of the Echo Chat application. Please refer to the main project license.