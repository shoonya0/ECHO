# WebSocket Chat Protocol

This document defines the WebSocket protocol for Echo's real-time chat system — supporting direct messages, group chats, and channels with permission-based routing.

## Connection

### Endpoint

```
GET /echo/v1/websocket/chat
```

### Authentication

- Requires JWT token via Authorization header (Bearer token)
- User is authenticated by middleware before WebSocket upgrade
- User info (ID, username, email, display name) is extracted from JWT claims and passed via Gin context

### Connection Flow

1. Client connects to WebSocket endpoint with valid JWT
2. JWT is validated by auth middleware
3. HTTP connection is upgraded to WebSocket via Gorilla WebSocket
4. A `Client` struct is created with unique UUID and buffered Send channel (capacity 10)
5. Hub checks if it's running; restarts if needed
6. Client is registered with the hub via `hub.Register <- client` channel
7. User presence is updated
8. Write goroutine starts (handles outgoing messages + ping/pong keepalive)
9. Read loop starts in main goroutine (blocking)

### Keepalive

- Server sends WebSocket ping every 54 seconds
- Client must respond with pong within 60 seconds
- Inactive connections are automatically cleaned up

## Request Types (Client → Server)

All requests are JSON objects sent as WebSocket text frames. The `type` field determines the action.

### 1. `send_message` — Send a Chat Message

```json
{
  "type": "send_message",
  "chatId": "507f1f77bcf86cd799439011",
  "content": "Hello everyone!",
  "messageType": "text",
  "attachments": [],
  "mentions": ["user_id_1", "user_id_2"],
  "requestId": "unique_request_id"
}
```

**Valid messageType values**: `text`, `image`, `file`, `audio`, `video`

**Validation**:
- Content max 4000 characters
- Max 10 attachments, each max 50MB
- Max 20 mentions per message

### 2. `join_chat` — Join a Chat Room

```json
{
  "type": "join_chat",
  "chatId": "507f1f77bcf86cd799439011",
  "senderId": "target_user_id",
  "requestId": "unique_request_id"
}
```

Note: Creates a direct chat if one doesn't exist between the authenticated user and `senderId`, then joins it.

### 3. `leave_chat` — Leave a Chat Room

```json
{
  "type": "leave_chat",
  "chatId": "507f1f77bcf86cd799439011",
  "requestId": "unique_request_id"
}
```

### 4. `set_typing` — Set Typing Indicator

```json
{
  "type": "set_typing",
  "chatId": "507f1f77bcf86cd799439011",
  "metadata": {
    "isTyping": true
  },
  "requestId": "unique_request_id"
}
```

### 5. `mark_read` — Mark Messages as Read

```json
{
  "type": "mark_read",
  "chatId": "507f1f77bcf86cd799439011",
  "metadata": {
    "messageIds": ["msg_id_1", "msg_id_2"]
  },
  "requestId": "unique_request_id"
}
```

### 6. `add_reaction` — Add Emoji Reaction

```json
{
  "type": "add_reaction",
  "chatId": "507f1f77bcf86cd799439011",
  "metadata": {
    "messageId": "msg_id_1",
    "emoji": "👍"
  },
  "requestId": "unique_request_id"
}
```

### 7. `remove_reaction` — Remove Emoji Reaction

```json
{
  "type": "remove_reaction",
  "chatId": "507f1f77bcf86cd799439011",
  "metadata": {
    "messageId": "msg_id_1",
    "emoji": "👍"
  },
  "requestId": "unique_request_id"
}
```

### 8. `edit_message` — Edit a Message (PLANNED)

```json
{
  "type": "edit_message",
  "chatId": "507f1f77bcf86cd799439011",
  "metadata": {
    "messageId": "msg_id_1",
    "newContent": "Updated content"
  },
  "requestId": "unique_request_id"
}
```

Status: Returns `NOT_IMPLEMENTED` error.

### 9. `delete_message` — Delete a Message (PLANNED)

```json
{
  "type": "delete_message",
  "chatId": "507f1f77bcf86cd799439011",
  "metadata": {
    "messageId": "msg_id_1"
  },
  "requestId": "unique_request_id"
}
```

Status: Returns `NOT_IMPLEMENTED` error.

### 10. `invite_user` — Invite User to Chat (PLANNED)

Status: Returns `NOT_IMPLEMENTED` error.

### 11. `remove_user` — Remove User from Chat (PLANNED)

Status: Returns `NOT_IMPLEMENTED` error.

### 12. `update_chat` — Update Chat Settings (PLANNED)

Status: Returns `NOT_IMPLEMENTED` error.

## Response Types (Server → Client)

### 1. `message` — Incoming Chat Message

```json
{
  "type": "message",
  "chatId": "507f1f77bcf86cd799439011",
  "userId": "sender_user_id",
  "username": "sender_username",
  "data": {
    "id": "message_id",
    "chatId": "507f1f77bcf86cd799439011",
    "senderId": "sender_user_id",
    "content": "Hello everyone!",
    "messageType": "text",
    "attachments": [],
    "createdAt": "2024-01-01T12:00:00Z"
  },
  "timestamp": "2024-01-01T12:00:00Z"
}
```

### 2. `typing` — Typing Indicator

```json
{
  "type": "typing",
  "chatId": "507f1f77bcf86cd799439011",
  "userId": "user_id",
  "data": {
    "userId": "user_id",
    "username": "username",
    "isTyping": true,
    "timestamp": "2024-01-01T12:00:00Z"
  },
  "timestamp": "2024-01-01T12:00:00Z"
}
```

### 3. `join` / `leave` — User Join/Leave Events

```json
{
  "type": "join",
  "chatId": "507f1f77bcf86cd799439011",
  "userId": "user_id",
  "data": {
    "userId": "user_id",
    "username": "username",
    "action": "join",
    "timestamp": "2024-01-01T12:00:00Z"
  },
  "timestamp": "2024-01-01T12:00:00Z"
}
```

### 4. `presence` — User Presence Update

```json
{
  "type": "presence",
  "chatId": "507f1f77bcf86cd799439011",
  "userId": "user_id",
  "data": {
    "userId": "user_id",
    "status": "online",
    "lastSeen": "2024-01-01T12:00:00Z"
  },
  "timestamp": "2024-01-01T12:00:00Z"
}
```

### 5. `reaction` — Message Reaction Event

```json
{
  "type": "reaction",
  "chatId": "507f1f77bcf86cd799439011",
  "userId": "user_id",
  "data": {
    "messageId": "message_id",
    "userId": "user_id",
    "username": "username",
    "emoji": "👍",
    "action": "add",
    "timestamp": "2024-01-01T12:00:00Z"
  },
  "timestamp": "2024-01-01T12:00:00Z"
}
```

### 6. `response` — Success Acknowledgment

```json
{
  "type": "response",
  "data": {
    "success": true,
    "messageId": "created_message_id",
    "timestamp": "2024-01-01T12:00:00Z",
    "chatId": "507f1f77bcf86cd799439011",
    "messageType": "text"
  },
  "requestId": "original_request_id",
  "timestamp": "2024-01-01T12:00:00Z"
}
```

### 7. `error` — Error Response

```json
{
  "type": "error",
  "data": {
    "code": "PERMISSION_DENIED",
    "message": "You don't have permission to send messages in this chat",
    "details": "Additional error details if available"
  },
  "requestId": "original_request_id",
  "timestamp": "2024-01-01T12:00:00Z"
}
```

## Chat Types & Permissions

### Direct Chats (`direct`)

- **Participants**: 2 users
- **Permissions**: Both users can send messages (unrestricted)
- **Auto-created**: When users first message each other via `join_chat`
- **Privacy**: Private by default

### Group Chats (`group`)

- **Participants**: 3+ users (configurable)
- **Roles**: `owner`, `admin`, `member`
- **Permissions**: Role-based; defaults allow all members to send messages
- **Features**: Name, description, avatar, invite codes

### Channels (`channel`)

- **Participants**: Large number of users
- **Posting**: Restricted to `admin` or `owner` roles only
- **Use Case**: Announcements, broadcasts

### Permission Checks (per chat type)

Messages are validated as follows:
- **Direct**: All participants can send (unless blocked/muted)
- **Group**: Based on participant permissions (`send_message`, `admin`, `owner`); defaults to allow if no permissions set
- **Channel**: Only `admin`/`owner` roles can send

Blocked or muted participants cannot send messages regardless of chat type.

## Error Codes

| Code                      | Description                                 |
| ------------------------- | ------------------------------------------- |
| `INVALID_REQUEST`         | Missing or invalid request parameters       |
| `INVALID_CHAT_ID`         | Chat ID format is invalid                   |
| `INVALID_MESSAGE_ID`      | Message ID format is invalid                |
| `INVALID_USER_ID`         | User ID format is invalid                   |
| `PERMISSION_DENIED`       | User lacks required permissions             |
| `PERMISSION_CHECK_FAILED` | Error validating permissions                |
| `INVALID_CONTENT`         | Message content validation failed           |
| `MESSAGE_FAILED`          | Failed to send message                      |
| `REACTION_FAILED`         | Failed to add/remove reaction               |
| `READ_FAILED`             | Failed to mark messages as read             |
| `TYPING_FAILED`           | Failed to update typing status              |
| `CHAT_CREATION_FAILED`    | Failed to create chat                       |
| `NOT_IMPLEMENTED`         | Feature not yet implemented                 |
| `UNKNOWN_REQUEST`         | Unrecognized request type                   |
| `PARSE_ERROR`             | Failed to parse WebSocket message           |
| `CHANNEL_FULL`            | Client send channel is full (message dropped)|
| `USER_NOT_AUTHENTICATED`  | User not found in context                   |

## Client Implementation Example (JavaScript)

```javascript
class ChatClient {
  constructor(token) {
    this.token = token;
    this.ws = null;
    this.requestId = 0;
    this.pendingRequests = new Map();
  }

  connect() {
    this.ws = new WebSocket(`ws://localhost:8080/echo/v1/websocket/chat`);
    // Note: JWT is sent via Authorization header during HTTP upgrade,
    // handled by the browser automatically if using cookie-based auth,
    // or set via custom headers if the WebSocket client supports it.
    // For manual testing, use wscat with -H flag.

    this.ws.onopen = () => console.log("Connected");
    this.ws.onmessage = (event) => this.handleMessage(JSON.parse(event.data));
    this.ws.onclose = () => console.log("Disconnected");
  }

  handleMessage(message) {
    switch (message.type) {
      case "message":   this.onNewMessage(message); break;
      case "typing":    this.onTypingIndicator(message); break;
      case "join":
      case "leave":     this.onUserJoinLeave(message); break;
      case "reaction":  this.onReaction(message); break;
      case "response":  this.onResponse(message); break;
      case "error":     this.onError(message); break;
    }
  }

  sendMessage(chatId, content, messageType = "text") {
    return this.sendRequest({
      type: "send_message", chatId, content, messageType,
      requestId: this.generateRequestId()
    });
  }

  joinChat(chatId, targetUserId) {
    return this.sendRequest({
      type: "join_chat", chatId, senderId: targetUserId,
      requestId: this.generateRequestId()
    });
  }

  setTyping(chatId, isTyping) {
    this.ws.send(JSON.stringify({
      type: "set_typing", chatId,
      metadata: { isTyping }
    }));
  }

  addReaction(chatId, messageId, emoji) {
    return this.sendRequest({
      type: "add_reaction", chatId,
      metadata: { messageId, emoji },
      requestId: this.generateRequestId()
    });
  }

  sendRequest(request) {
    return new Promise((resolve, reject) => {
      this.pendingRequests.set(request.requestId, { resolve, reject });
      this.ws.send(JSON.stringify(request));
      setTimeout(() => {
        if (this.pendingRequests.has(request.requestId)) {
          this.pendingRequests.delete(request.requestId);
          reject(new Error("Request timeout"));
        }
      }, 10000);
    });
  }

  generateRequestId() {
    return `req_${++this.requestId}_${Date.now()}`;
  }

  onResponse(message) {
    const pending = this.pendingRequests.get(message.requestId);
    if (pending) {
      this.pendingRequests.delete(message.requestId);
      pending.resolve(message.data);
    }
  }

  onError(message) {
    console.error("Error:", message.data);
    const pending = this.pendingRequests.get(message.requestId);
    if (pending) {
      this.pendingRequests.delete(message.requestId);
      pending.reject(new Error(message.data.message));
    }
  }
}
```

## Database Collections

| Collection  | Purpose                                               |
| ----------- | ----------------------------------------------------- |
| `users`     | User profiles (id, username, email, avatar, presence) |
| `chats`     | Chat metadata, participants, settings, active clients |
| `messages`  | All messages with embedded sender info and reactions  |

## Monitoring

### Hub Statistics

```http
GET /echo/v1/chats/hub/stats
```

```json
{
  "totalClients": 150,
  "totalRooms": 45,
  "totalUsers": 120,
  "timestamp": "2024-01-01T12:00:00Z"
}
```

## Performance Notes

- **Room-based broadcasting**: Messages only sent to clients subscribed to the relevant chat
- **Channel-based hub**: Uses buffered Go channels (`Register`, `Unregister`) for thread-safe client management
- **Auto-cleanup**: Inactive connections (no pong within 60s) are automatically removed
- **User info caching**: 5-minute TTL cache with batch lookups via `UserLookupService`
- **Redis Pub/Sub**: Optional cross-instance messaging via `PubSubManager` (see [PUBSUB_ARCHITECTURE.md](./PUBSUB_ARCHITECTURE.md))