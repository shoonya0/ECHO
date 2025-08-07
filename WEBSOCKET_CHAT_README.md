# 🚀 Echo WebSocket Chat - Real-Time Chat System

A comprehensive WebSocket-based real-time chat system built with Go, Gin, MongoDB, and Gorilla WebSocket.

## ✨ Features

### Real-Time Communication

- **WebSocket-based messaging**: Instant message delivery with persistent connections
- **Multiple user support**: Chat rooms can handle 2 or more users simultaneously
- **Message broadcasting**: Real-time message distribution to all chat participants
- **Typing indicators**: Live typing status updates
- **Read receipts**: Message delivery and read status tracking
- **User presence**: Online/offline status tracking and notifications

### Chat Management

- **Direct chats**: One-on-one conversations
- **Group chats**: Multi-user chat rooms with up to 100 participants (configurable)
- **Join/Leave functionality**: Dynamic room membership management
- **Chat persistence**: Messages stored in MongoDB for history
- **Message types**: Support for text, image, file, audio, and video messages

### Advanced Features

- **User authentication**: JWT-based authentication for secure connections
- **Connection management**: Automatic cleanup of inactive connections
- **Error handling**: Comprehensive error reporting and connection recovery
- **Scalable architecture**: Hub-based connection management for high concurrency

## 🏗️ Architecture

### Core Components

1. **WebSocket Hub** (`internal/services/websocket.hub.go`)

   - Centralized connection management
   - Message broadcasting
   - Room management
   - Client lifecycle handling

2. **WebSocket Models** (`internal/models/websocket.model.go`)

   - Message structures
   - Client representation
   - Chat room models
   - Event types

3. **WebSocket Controller** (`internal/controller/websocket.controller.go`)

   - Connection upgrading
   - Message processing
   - Request handling
   - Client communication

4. **Chat Service** (`internal/services/chat.service.go`)
   - Message persistence
   - Chat creation and management
   - Real-time broadcasting integration

## 🚀 Getting Started

### Prerequisites

- Go 1.24.3 or higher
- MongoDB
- Redis (optional, for enhanced presence features)

### Installation

1. **Clone and build the application:**

   ```bash
   cd Echo
   go build -o build/echo-chat .
   ```

2. **Configure your environment:**

   - Set up MongoDB connection in your config
   - Configure JWT secret for authentication
   - Optional: Set up Redis for enhanced features

3. **Run the server:**
   ```bash
   ./build/echo-chat
   ```

The server will start on port 8080 by default with WebSocket endpoints available.

## 🔌 API Endpoints

### WebSocket Connection

```
GET /echo/v1/ws/chat
```

- **Authentication**: Required (JWT token)
- **Protocol**: WebSocket
- **Purpose**: Main real-time chat connection

### HTTP Endpoints

#### Chat Management

```
POST /echo/v1/chats/direct     # Create direct chat
POST /echo/v1/chats/group      # Create group chat
GET  /echo/v1/chats/messages   # Get chat messages
GET  /echo/v1/chats/hub/stats  # Get hub statistics
```

## 📡 WebSocket Protocol

### Message Types

#### Client → Server

- `send_message`: Send a chat message
- `join_room`: Join a chat room
- `leave_room`: Leave a chat room
- `set_typing`: Update typing status
- `mark_read`: Mark messages as read

#### Server → Client

- `message`: New chat message
- `typing`: Typing indicator update
- `join`: User joined room
- `leave`: User left room
- `presence`: User presence update
- `delivery`: Message delivery status
- `response`: Request acknowledgment
- `error`: Error notification

### Example Messages

#### Send Message

```json
{
  "type": "send_message",
  "chatId": "chat-room-id",
  "content": "Hello, world!",
  "messageType": "text",
  "requestId": "unique-request-id"
}
```

#### Join Room

```json
{
  "type": "join_room",
  "chatId": "chat-room-id",
  "requestId": "unique-request-id"
}
```

#### Typing Indicator

```json
{
  "type": "set_typing",
  "chatId": "chat-room-id",
  "metadata": {
    "isTyping": true
  }
}
```

## 🧪 Testing

### Web Client

Open `test-websocket-chat.html` in your browser for a complete test interface:

1. **Connect**: Enter WebSocket URL and auth token
2. **Join Room**: Specify a chat room ID
3. **Send Messages**: Type and send real-time messages
4. **Test Features**: Try typing indicators, read receipts, etc.

### Manual Testing

```bash
# Using wscat (install with: npm install -g wscat)
wscat -c "ws://localhost:8080/echo/v1/ws/chat" -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

## 🔧 Configuration

### Environment Variables

```bash
PORT=8080
DB_URI=mongodb://localhost:27017
JWT_SECRET=your-secret-key
REDIS_URI=redis://localhost:6379  # Optional
```

### WebSocket Hub Settings

- **Max Clients per Room**: 100 (configurable)
- **Connection Timeout**: 60 seconds
- **Ping Interval**: 54 seconds
- **Cleanup Interval**: 30 seconds
- **Inactive Timeout**: 5 minutes

## 📊 Monitoring

### Hub Statistics

```bash
curl http://localhost:8080/echo/v1/chats/hub/stats
```

Returns:

```json
{
  "totalClients": 25,
  "totalRooms": 5,
  "totalUsers": 20,
  "timestamp": "2025-01-07T17:00:00Z"
}
```

## 🔒 Security Features

- **JWT Authentication**: All WebSocket connections require valid tokens
- **CORS Support**: Configurable cross-origin resource sharing
- **Input Validation**: Comprehensive message and request validation
- **Rate Limiting**: Protection against message flooding
- **Connection Limits**: Per-room participant limits

## 🏃‍♂️ Performance

### Optimizations

- **Connection Pooling**: Efficient WebSocket connection management
- **Message Batching**: Optimized broadcast operations
- **Database Indexing**: Efficient message and chat queries
- **Memory Management**: Automatic cleanup of inactive connections
- **Concurrency**: Goroutine-based concurrent processing

### Scaling Considerations

- **Horizontal Scaling**: Design supports multiple server instances
- **Database Sharding**: Messages can be partitioned by chat ID
- **Load Balancing**: WebSocket-aware load balancing recommended
- **Caching**: Redis integration for session and presence data

## 🚨 Error Handling

### Common Error Codes

- `JOIN_DENIED`: Room is full or permission denied
- `INVALID_REQUEST`: Malformed message format
- `INVALID_CHAT_ID`: Invalid chat room identifier
- `MESSAGE_FAILED`: Message could not be sent
- `TYPING_FAILED`: Typing status update failed

### Connection Recovery

- Automatic reconnection on network failures
- Message queue during temporary disconnections
- State synchronization on reconnection

## 🤝 Contributing

1. Follow Go best practices and project conventions
2. Add tests for new features
3. Update documentation for API changes
4. Ensure proper error handling and logging
5. Test WebSocket functionality thoroughly

## 📝 License

This project is part of the Echo Chat application. Please refer to the main project license.

---

**Happy Chatting! 🎉**

For more information, issues, or contributions, please refer to the main Echo project repository.
