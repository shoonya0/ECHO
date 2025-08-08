# Enhanced WebSocket Chat System

This document outlines the enhanced WebSocket chat functionality that supports both group and single (direct) chat with proper chatID-based routing.

## Features

### ✅ Core Chat Functionality

- **Direct Chats**: 1-on-1 private conversations
- **Group Chats**: Multi-participant group conversations
- **Channel Support**: Large-scale broadcast channels
- **Auto-Join**: Users automatically join their active chats upon connection
- **Permission System**: Role-based permissions for different chat types

### ✅ Real-time Events

- **Message Sending/Receiving**: Real-time message delivery
- **Typing Indicators**: Show when users are typing
- **Read Receipts**: Track message read status
- **User Presence**: Online/offline status tracking
- **Join/Leave Events**: Notification when users join or leave chats
- **Message Reactions**: Add/remove emoji reactions to messages

### ✅ Advanced Features

- **Message Validation**: Content length, type, and attachment validation
- **Permission Checks**: Comprehensive permission system for different actions
- **Error Handling**: Detailed error responses with specific error codes
- **Rate Limiting**: Built-in protection against spam
- **Scalable Architecture**: Hub pattern with efficient message routing

## WebSocket Connection

### Endpoint

```
GET /ws/chat
```

### Authentication

- Requires JWT token in Authorization header or as query parameter
- User must be authenticated before WebSocket upgrade

### Connection Flow

1. Client connects to WebSocket endpoint
2. JWT token is validated
3. WebSocket connection is upgraded
4. Client is registered with the hub
5. Client auto-joins their active chats
6. Ready to send/receive messages

## Message Types

### Request Types (Client → Server)

#### 1. Send Message

```json
{
  "type": "send_message",
  "chatId": "chat_object_id",
  "content": "Hello everyone!",
  "messageType": "text",
  "attachments": [],
  "mentions": ["user_id_1", "user_id_2"],
  "requestId": "unique_request_id"
}
```

#### 2. Join Room

```json
{
  "type": "join_room",
  "chatId": "chat_object_id",
  "requestId": "unique_request_id"
}
```

#### 3. Leave Room

```json
{
  "type": "leave_room",
  "chatId": "chat_object_id",
  "requestId": "unique_request_id"
}
```

#### 4. Set Typing Status

```json
{
  "type": "set_typing",
  "chatId": "chat_object_id",
  "metadata": {
    "isTyping": true
  },
  "requestId": "unique_request_id"
}
```

#### 5. Mark Messages as Read

```json
{
  "type": "mark_read",
  "chatId": "chat_object_id",
  "metadata": {
    "messageIds": ["msg_id_1", "msg_id_2"]
  },
  "requestId": "unique_request_id"
}
```

#### 6. Add Reaction

```json
{
  "type": "add_reaction",
  "chatId": "chat_object_id",
  "metadata": {
    "messageId": "message_object_id",
    "emoji": "👍"
  },
  "requestId": "unique_request_id"
}
```

#### 7. Remove Reaction

```json
{
  "type": "remove_reaction",
  "chatId": "chat_object_id",
  "metadata": {
    "messageId": "message_object_id",
    "emoji": "👍"
  },
  "requestId": "unique_request_id"
}
```

### Response Types (Server → Client)

#### 1. Incoming Message

```json
{
  "type": "message",
  "chatId": "chat_object_id",
  "userId": "sender_user_id",
  "username": "sender_username",
  "data": {
    "id": "message_id",
    "chatId": "chat_object_id",
    "senderId": "sender_user_id",
    "content": "Hello everyone!",
    "messageType": "text",
    "attachments": [],
    "createdAt": "2024-01-01T12:00:00Z"
  },
  "timestamp": "2024-01-01T12:00:00Z"
}
```

#### 2. Typing Indicator

```json
{
  "type": "typing",
  "chatId": "chat_object_id",
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

#### 3. User Join/Leave

```json
{
  "type": "join", // or "leave"
  "chatId": "chat_object_id",
  "userId": "user_id",
  "data": {
    "userId": "user_id",
    "username": "username",
    "action": "join", // or "leave"
    "timestamp": "2024-01-01T12:00:00Z"
  },
  "timestamp": "2024-01-01T12:00:00Z"
}
```

#### 4. Message Reaction

```json
{
  "type": "reaction",
  "chatId": "chat_object_id",
  "userId": "user_id",
  "data": {
    "messageId": "message_id",
    "userId": "user_id",
    "username": "username",
    "emoji": "👍",
    "action": "add", // or "remove"
    "timestamp": "2024-01-01T12:00:00Z"
  },
  "timestamp": "2024-01-01T12:00:00Z"
}
```

#### 5. Success Response

```json
{
  "type": "response",
  "data": {
    "success": true,
    "messageId": "created_message_id",
    "timestamp": "2024-01-01T12:00:00Z",
    "chatId": "chat_object_id",
    "messageType": "text"
  },
  "requestId": "original_request_id",
  "timestamp": "2024-01-01T12:00:00Z"
}
```

#### 6. Error Response

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

## Chat Types and Permissions

### Direct Chats

- **Participants**: Exactly 2 users
- **Permissions**: Both users can send messages
- **Auto-created**: When users first message each other
- **Privacy**: Private by default

### Group Chats

- **Participants**: 3+ users (configurable limit)
- **Roles**: Owner, Admin, Member
- **Permissions**: Configurable per role
- **Features**: Name, description, avatar, custom settings

### Channels

- **Participants**: Large number of users (up to 1000)
- **Posting**: Usually restricted to admins/owners
- **Read Receipts**: Disabled for performance
- **Use Case**: Announcements, broadcasts

## Permission System

### Chat-level Permissions

- `send_message`: Can send messages
- `edit_message`: Can edit own messages
- `delete_message`: Can delete own messages
- `invite_user`: Can invite new users
- `remove_user`: Can remove users
- `edit_chat`: Can edit chat settings
- `admin`: Has all permissions
- `owner`: Has all permissions + ownership rights

### Permission Checks

- Users must be participants in a chat to perform actions
- Blocked or muted users cannot send messages
- Role-based permissions are enforced for all actions
- Default permissions vary by chat type

## Error Codes

| Code                      | Description                           |
| ------------------------- | ------------------------------------- |
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

## Client Implementation Example

### JavaScript WebSocket Client

```javascript
class ChatClient {
  constructor(token) {
    this.token = token;
    this.ws = null;
    this.requestId = 0;
    this.pendingRequests = new Map();
  }

  connect() {
    this.ws = new WebSocket(`ws://localhost:8080/ws/chat?token=${this.token}`);

    this.ws.onopen = () => {
      console.log("Connected to chat");
    };

    this.ws.onmessage = (event) => {
      const message = JSON.parse(event.data);
      this.handleMessage(message);
    };

    this.ws.onclose = () => {
      console.log("Disconnected from chat");
    };
  }

  handleMessage(message) {
    switch (message.type) {
      case "message":
        this.onNewMessage(message);
        break;
      case "typing":
        this.onTypingIndicator(message);
        break;
      case "join":
      case "leave":
        this.onUserJoinLeave(message);
        break;
      case "reaction":
        this.onReaction(message);
        break;
      case "response":
        this.onResponse(message);
        break;
      case "error":
        this.onError(message);
        break;
    }
  }

  sendMessage(chatId, content, messageType = "text") {
    const requestId = this.generateRequestId();
    const request = {
      type: "send_message",
      chatId,
      content,
      messageType,
      requestId,
    };

    return this.sendRequest(request);
  }

  joinRoom(chatId) {
    const requestId = this.generateRequestId();
    const request = {
      type: "join_room",
      chatId,
      requestId,
    };

    return this.sendRequest(request);
  }

  setTyping(chatId, isTyping) {
    const request = {
      type: "set_typing",
      chatId,
      metadata: { isTyping },
    };

    this.ws.send(JSON.stringify(request));
  }

  addReaction(chatId, messageId, emoji) {
    const requestId = this.generateRequestId();
    const request = {
      type: "add_reaction",
      chatId,
      metadata: { messageId, emoji },
      requestId,
    };

    return this.sendRequest(request);
  }

  sendRequest(request) {
    return new Promise((resolve, reject) => {
      this.pendingRequests.set(request.requestId, { resolve, reject });
      this.ws.send(JSON.stringify(request));

      // Timeout after 10 seconds
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

  onNewMessage(message) {
    console.log("New message:", message.data);
  }

  onTypingIndicator(message) {
    console.log("Typing:", message.data);
  }

  onUserJoinLeave(message) {
    console.log("User join/leave:", message.data);
  }

  onReaction(message) {
    console.log("Reaction:", message.data);
  }

  onResponse(message) {
    const request = this.pendingRequests.get(message.requestId);
    if (request) {
      this.pendingRequests.delete(message.requestId);
      request.resolve(message.data);
    }
  }

  onError(message) {
    console.error("Error:", message.data);
    const request = this.pendingRequests.get(message.requestId);
    if (request) {
      this.pendingRequests.delete(message.requestId);
      request.reject(new Error(message.data.message));
    }
  }
}

// Usage
const client = new ChatClient("your_jwt_token");
client.connect();

// Send a message
client
  .sendMessage("chat_id", "Hello everyone!")
  .then((response) => console.log("Message sent:", response))
  .catch((error) => console.error("Failed to send:", error));

// Join a room
client
  .joinRoom("chat_id")
  .then(() => console.log("Joined room"))
  .catch((error) => console.error("Failed to join:", error));

// Add reaction
client
  .addReaction("chat_id", "message_id", "👍")
  .then(() => console.log("Reaction added"))
  .catch((error) => console.error("Failed to react:", error));
```

## Database Collections

### Chat Collection

- Stores chat metadata, participants, settings
- Used for permission checks and room creation
- Embedded participant data for quick access

### Message Collection

- Stores all messages with embedded sender info
- Supports reactions, threads, attachments
- Optimized for real-time queries

### User Collection

- Basic user information
- Referenced for participant details

## Performance Considerations

1. **Connection Pooling**: Hub manages all WebSocket connections efficiently
2. **Room-based Broadcasting**: Messages only sent to relevant participants
3. **Auto-cleanup**: Inactive connections and empty rooms are cleaned up
4. **Efficient Queries**: Embedded data reduces database lookups
5. **Rate Limiting**: Built-in protection against spam and abuse

## Monitoring and Debugging

### Hub Statistics Endpoint

```http
GET /api/ws/stats
```

Returns current WebSocket hub statistics:

```json
{
  "totalClients": 150,
  "totalRooms": 45,
  "totalUsers": 120,
  "timestamp": "2024-01-01T12:00:00Z"
}
```

### Logging

- All client connections/disconnections are logged
- Message sending/receiving events are logged
- Error conditions are logged with context
- Performance metrics can be added for monitoring

This enhanced WebSocket chat system provides a robust foundation for real-time messaging with proper support for both group and single chat functionality, comprehensive permission management, and excellent scalability.
