
## Overview

This document describes the WebSocket architecture using Redis Pub/Sub for efficient single and group chat messaging across multiple server instances.

## Architecture Components

### 1. WebSocket Hub (`internal/services/websocket.hub.go`)

Centralized WebSocket connection hub managing all client connections.

#### Key Features:

- **Client Management**: Maps clients by ID, chat membership, and user ID
- **Channel-Based Registration**: Uses buffered Go channels (`Register <- client`, `Unregister <- client`) for thread-safe client lifecycle
- **Message Broadcasting**: `BroadcastToChat()` sends messages to all clients in a chat room
- **User Info Caching**: 5-minute TTL cache for user display information
- **Thread Safety**: sync.RWMutex for concurrent access
- **Auto-Join**: Clients auto-join their active chats on connection

### 2. PubSubManager (`internal/services/pubsub_manager.go`)

Handles all Redis pub/sub operations for cross-instance communication.

#### Key Features:

- **Channel Management**: Manages subscriptions to user, chat, and system channels
- **Message Routing**: Routes messages based on channel type
- **Instance Coordination**: Enables communication between server instances
- **Presence Synchronization**: Keeps user presence consistent across instances

#### Channel Types:

```
user:<userID>          - Direct messages to specific users
chat:<chatID>          - Group chat messages
presence:global        - Global presence updates
system:global          - System-wide announcements
instance:<instanceID>  - Instance-specific messages
```

### 3. Chat Service (`internal/services/chat.service.go`)

Handles message persistence, chat management, and integration with the hub for real-time broadcasting.

#### Key Features:

- **Message Persistence**: Saves messages to MongoDB before broadcasting
- **Chat Creation**: Creates direct, group, and channel chats
- **Permission Validation**: Checks participant roles and permissions
- **Real-Time Integration**: Calls `BroadcastToChat()` after message persistence

### 4. Presence Service (`internal/services/presence.go`)

Manages real-time user presence and status tracking.

## Message Flow

### Single User Chat (Direct Message)

```
User A → Server 1 → Redis (publish to user:userB)
                    Redis → Server 2 → User B
```

If both users are on the same server, the message is delivered locally without Redis involvement.

### Group Chat

```
User A → Server 1 → Mongo (persist) → Redis (publish to chat:chatX)
                                       Redis → Server 1 → local clients in chatX
                                       Redis → Server 2 → remote clients in chatX
```

### Presence Updates

```
User A → Server 1 (connect/disconnect)
         Server 1 → Redis (publish presence:global)
         Redis → All Servers → All Users (presence update)
```

## Implementation Details

### Hub Initialization

```go
// In main.go or service initialization
hub := services.GetHubInstance()
hub.Start() // Launches the hub run loop with channel-based client management
```

The hub uses a singleton pattern accessed via `services.GetHubInstance()`.

### Connecting a Client

```go
// When a client connects (in websocket.controller.go)
client := &models.Client{
    ID:         uuid.New().String(),
    UserID:     user.ID,
    Connection: conn,
    Send:       make(chan models.WebSocketMessage, 10),
}

// Update user info cache
hub.UpdateUserInfoCache(user.ID.Hex(), models.UserDisplayInfo{
    Email:       user.Email,
    Username:    user.Username,
    DisplayName: user.Profile.DisplayName,
})

// Register with hub (via channel)
hub.Register <- client

// Update presence
services.GetPresenceInstance().Set(services.UserPresence{...})
```

### Sending Messages

```go
// Send a message (chat.service.go calls this after persisting)
err := services.BroadcastToChat(chatID, models.WebSocketMessage{
    Type:      models.WSMessageTypeChat,
    ChatID:    chatID,
    UserID:    senderID.Hex(),
    Data:      messageData,
    Timestamp: time.Now(),
})
```

### Handling Typing Indicators

```go
// Broadcast typing status
err := services.UpdateTypingStatus(ctx, chatID, userID, isTyping)
// This internally persists typing state and broadcasts via BroadcastToChat()
```

## Scalability Benefits

### 1. Horizontal Scaling

- Add more server instances without code changes
- Load balance WebSocket connections across instances
- Messages automatically routed to correct instances via Redis Pub/Sub

### 2. Efficient Resource Usage

- Only subscribe to relevant channels
- Minimize message duplication
- Local delivery for same-instance clients

### 3. Fault Tolerance

- If one instance fails, others continue operating
- Clients can reconnect to any available instance
- Presence updates ensure accurate online status

### 4. Performance Optimizations

- Batch presence updates every 30 seconds
- Cache user display info with TTL
- Clean up inactive connections automatically

## Redis Channel Patterns

### User Channels

- **Purpose**: Direct messages, notifications
- **Pattern**: `user:<userID>`
- **Subscribers**: All instances where user has active connections

### Chat Channels

- **Purpose**: Group chat messages
- **Pattern**: `chat:<chatID>`
- **Subscribers**: All instances with clients in that chat

### Global Channels

- **Purpose**: System-wide events
- **Patterns**:
  - `presence:global` - User online/offline status
  - `system:global` - Maintenance, announcements

## Configuration

### Redis Connection

```go
rdb := redis.NewClient(&redis.Options{
    Addr:     "localhost:6379",
    Password: "", // Set if required
    DB:       0,
})
```

See `internal/db/redis.go` and `config/main.config.go` for actual Redis configuration via environment variables (`REDIS_URI`, `REDIS_PASS`).

### Hub Initialization

```go
func main() {
    // Initialize Redis client
    // ...
    
    // Get hub instance
    hub := services.GetHubInstance()

    // Start hub with pub/sub
    hub.Start()

    // Graceful shutdown
    defer hub.Stop()
}
```

## Best Practices

### 1. Channel Naming

- Use consistent prefixes (`user:`, `chat:`, etc.)
- Include relevant IDs in channel names
- Keep channel names short for performance

### 2. Message Size

- Keep messages under 1MB
- Use pagination for message history
- Compress large payloads if needed

### 3. Subscription Management

- Subscribe only to necessary channels
- Unsubscribe when no longer needed
- Monitor subscription count

### 4. Error Handling

- Implement retry logic for failed publishes
- Log subscription/unsubscription errors
- Monitor Redis connection health

### 5. Security

- Validate channel access permissions
- Sanitize message content
- Implement rate limiting

## Monitoring

### Key Metrics

- Active connections per instance
- Messages per second (pub/sub)
- Channel subscription count
- Redis memory usage
- Message delivery latency

### Health Checks

```bash
curl http://localhost:8080/echo/v1/chats/hub/stats
```

```json
{
  "totalClients": 150,
  "totalRooms": 45,
  "totalUsers": 120,
  "timestamp": "2024-01-01T12:00:00Z"
}
```

## Troubleshooting

### Common Issues

1. **Messages not delivered**
   - Check Redis connectivity
   - Verify channel subscriptions
   - Check client connection status

2. **High memory usage**
   - Monitor subscription count
   - Check for subscription leaks
   - Review message size and frequency

3. **Presence not updating**
   - Verify Redis pub/sub working
   - Check presence sync interval
   - Review cleanup routines

## Future Enhancements

1. **Message Persistence**
   - Store messages in MongoDB before publishing (already implemented)
   - Implement message history API (already implemented)
   - Add offline message delivery

2. **Advanced Features**
   - Message encryption
   - File transfer support
   - Voice/video signaling

3. **Performance Improvements**
   - Connection pooling
   - Message batching
   - Compression algorithms

## Related Documentation

- [WebSocket Protocol Reference](./WEBSOCKET_PROTOCOL.md) — Full message types and API
- [README](./README.md) — Project overview and getting started