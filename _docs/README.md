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

- **Efficient WebSocket Hub** for connection management
- **User Info Caching** with 5-minute TTL
- **Batch Operations** for user data lookups
- **Thread-Safe Operations** with proper mutex usage
- **Automatic Cleanup** of inactive connections
- **Redis Pub/Sub** for cross-instance scaling

## Architecture

### Core Components

1. **WebSocket Hub** (`internal/services/websocket.hub.go`)
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

3. **User Lookup Service** (`internal/services/user_lookup.service.go`) — NEW
   - Efficient user display info caching
   - Batch lookup operations
   - 5-minute TTL cache
   - Automatic cache invalidation

4. **PubSub Manager** (`internal/services/pubsub_manager.go`)
   - Redis Pub/Sub for cross-instance messaging
   - Channel management (user:, chat:, presence:global, system:global)
   - Multi-instance coordination

5. **Presence Service**
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
- Redis (optional, for enhanced presence and pub/sub features)

### Installation

1. **Clone the Repository**

   ```bash
   git clone <repository-url>
   cd Echo
   ```

2. **Build the Application**

   ```bash
   go build -o build/echo-chat .
   ```

3. **Configure Environment**

   ```bash
   # Required Environment Variables
   PORT=8080
   DB_URI=mongodb://localhost:27017
   JWT_SECRET=your-secret-key
   REDIS_URI=redis://localhost:6379  # Optional
   ```

4. **Run the Server**
   ```bash
   ./build/echo-chat
   ```

## API Endpoints

### Authentication

```
POST /echo/v1/auth/login    - User login
POST /echo/v1/auth/signup   - User registration
```

### WebSocket (Real-Time Chat)

```
GET /echo/v1/websocket/chat    - Main WebSocket endpoint (JWT via Authorization header)
GET /echo/v1/websocket/notifications   - Notifications WebSocket (planned)
GET /echo/v1/websocket/presence        - Presence WebSocket (planned)
```

### Chat Management

```
POST   /echo/v1/chats/direct/:userId    - Create direct chat
POST   /echo/v1/chats/group             - Create group chat
GET    /echo/v1/chats/hub/stats         - Get hub statistics (monitoring)
```

### Messages

```
GET /echo/v1/messages/:chatID/messages   - Get chat messages with pagination
```

### Group Management

```
POST   /echo/v1/groups/:groupID/add-members   - Add group members (admin)
POST   /echo/v1/groups/:groupID/invites       - Create invite code
GET    /echo/v1/groups/:groupID/invites       - List invite codes
DELETE /echo/v1/groups/invites/:inviteID/     - Delete invite code
GET    /echo/v1/groups/invites/:inviteID/joined       - Get joined users by invite
POST   /echo/v1/groups/invites/:inviteID/:userID/send - Send invite to user
POST   /echo/v1/groups/join/:inviteCode       - Join group via invite code
GET    /echo/v1/groups/invites                - Get all invites of user
```

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
- `edit_message` — Edit a message (planned)
- `delete_message` — Delete a message (planned)
- `invite_user` — Invite user to chat (planned)
- `remove_user` — Remove user from chat (planned)
- `update_chat` — Update chat settings (planned)
- `set_presence` — Set user presence

### Response Types (Server → Client)
- `message` — New chat message
- `typing` — Typing indicator
- `join` / `leave` — User join/leave events
- `presence` — User presence update
- `delivery` — Message delivery status
- `reaction` — Message reaction event
- `response` — Request acknowledgment
- `error` — Error notification

## Security Features

- **JWT Authentication** (via Authorization header) for all connections
- **Permission-Based Access Control** (role-based: owner, admin, member)
- **Input Validation** for all requests (content length, type, attachments)
- **Rate Limiting** protection
- **CORS Support** (configurable)

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
| `NOT_IMPLEMENTED`         | Feature not yet implemented           |
| `UNKNOWN_REQUEST`         | Unrecognized request type             |
| `PARSE_ERROR`             | Failed to parse WebSocket message     |
| `CHANNEL_FULL`            | Client send channel is full           |

## Testing

### Web Client Testing

1. Open `test-websocket-chat.html` or `test-group-chat.html` in your browser
2. Connect using your JWT token
3. Test real-time messaging features

### Manual WebSocket Testing

```bash
wscat -c "ws://localhost:8080/echo/v1/websocket/chat" -H "Authorization: Bearer <jwt_token>"
```

## Monitoring

### Hub Statistics

```bash
curl http://localhost:8080/echo/v1/chats/hub/stats
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
2. **Database Efficiency** — Optimized queries with proper indexing, minimal embedded data
3. **WebSocket Optimizations** — Efficient connection pooling, automatic cleanup of inactive connections, thread-safe operations

## Development Guidelines

- Clean Architecture principles
- Interface-driven development
- Proper error handling and wrapping
- Follow Go idiomatic patterns
- Thread-safe operations with proper mutex usage

## License

This project is part of the Echo Chat application. Please refer to the main project license.