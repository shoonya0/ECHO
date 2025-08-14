# Echo - Real-Time Chat System

A powerful, scalable real-time chat system built with Go, featuring WebSocket support, group messaging, and comprehensive presence tracking.

## 🌟 Key Features

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

### Performance & Scalability

- **Efficient WebSocket Hub** for connection management
- **User Info Caching** with 5-minute TTL
- **Batch Operations** for user data lookups
- **Thread-Safe Operations** with proper mutex usage
- **Automatic Cleanup** of inactive connections

## 🏗️ Architecture

### Core Components

1. **WebSocket Hub** (`internal/services/websocket.hub.go`)

   - Centralized connection management
   - Real-time message broadcasting
   - Client lifecycle handling
   - Thread-safe operations

2. **Chat Service** (`internal/services/chat.service.go`)

   - Message persistence
   - Chat room management
   - Permission handling
   - Real-time updates

3. **User Lookup Service** (`internal/services/user_lookup.service.go`)

   - Efficient user info caching
   - Batch lookup operations
   - 5-minute TTL cache
   - Automatic cache invalidation

4. **Presence Service**
   - Real-time user status tracking
   - Activity monitoring
   - Automatic cleanup of inactive users

## 🚀 Getting Started

### Prerequisites

- Go 1.24.3 or higher
- MongoDB
- Redis (optional, for enhanced presence features)

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

## 📡 API Endpoints

### Authentication

```
POST /api/v1/login    - User login
POST /api/v1/signup   - User registration
```

### WebSocket

```
GET /ws/chat?token=<jwt_token>  - WebSocket connection endpoint
```

### Chat Management

```
POST /api/v1/chats/direct     - Create direct chat
POST /api/v1/chats/group      - Create group chat
GET  /api/v1/chats/messages   - Get chat messages
```

## 💬 Message Types

### Client → Server

```json
{
  "type": "send_message",
  "chatId": "chat_id",
  "content": "Hello!",
  "messageType": "text",
  "requestId": "unique_id"
}
```

### Server → Client

```json
{
  "type": "message",
  "chatId": "chat_id",
  "userId": "sender_id",
  "data": {
    "content": "Hello!",
    "messageType": "text",
    "timestamp": "2024-01-01T12:00:00Z"
  }
}
```

## 🔐 Security Features

- **JWT Authentication** for all connections
- **Permission-Based Access Control**
- **Input Validation** for all requests
- **Rate Limiting** protection
- **CORS Support**

## 📊 Performance Optimizations

1. **Caching Layer**

   - User information cached with 5-minute TTL
   - Batch operations for multiple user queries
   - Automatic cache invalidation on user updates

2. **Database Efficiency**

   - Optimized queries with proper indexing
   - Minimal embedded data storage
   - Efficient participant tracking

3. **WebSocket Optimizations**
   - Efficient connection pooling
   - Automatic cleanup of inactive connections
   - Thread-safe operations

## 🧪 Testing

### Web Client Testing

1. Open `test-websocket-chat.html` in your browser
2. Connect using your JWT token
3. Test real-time messaging features

### Manual WebSocket Testing

```bash
wscat -c "ws://localhost:8080/ws/chat?token=<jwt_token>"
```

## 🔍 Monitoring

### Hub Statistics

```bash
curl http://localhost:8080/api/v1/chats/hub/stats
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

## 🚨 Error Handling

### Common Error Codes

- `INVALID_REQUEST` - Missing or invalid parameters
- `PERMISSION_DENIED` - User lacks required permissions
- `INVALID_CHAT_ID` - Chat ID format is invalid
- `MESSAGE_FAILED` - Failed to send message
- `PERMISSION_CHECK_FAILED` - Error validating permissions

## 📝 Development Guidelines

### Code Structure

- Clean Architecture principles
- Interface-driven development
- Proper error handling
- Comprehensive logging
- Test coverage for core functionality

### Best Practices

- Use proper error wrapping
- Implement proper context handling
- Follow Go idiomatic patterns
- Maintain comprehensive documentation
- Regular performance monitoring

## 🤝 Contributing

1. Fork the repository
2. Create your feature branch
3. Commit your changes
4. Push to the branch
5. Create a Pull Request

## 📄 License

This project is licensed under the [MIT License](LICENSE).

---

Built with ❤️ using Go, MongoDB, Redis, and WebSockets.
