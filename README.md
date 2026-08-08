# ECHO — Real-Time Chat System

A powerful, scalable real-time chat system built with Go, featuring WebSocket support, group messaging, and comprehensive presence tracking.

> 📚 **Full documentation is in [`_docs/`](./_docs/README.md)** — including the WebSocket protocol reference and pub/sub architecture.

## Quick Start

```bash
# Build
go build -o build/echo-chat .

# Configure (environment variables)
PORT=8080
DB_URI=mongodb://localhost:27017
JWT_SECRET=your-secret-key
REDIS_URI=redis://localhost:6379  # Optional

# Run
./build/echo-chat
```

## Documentation Index

| Document                                                | Description                                   |
| ------------------------------------------------------- | --------------------------------------------- |
| [`_docs/README.md`](./_docs/README.md)                  | Full project overview, API endpoints, features |
| [`_docs/WEBSOCKET_PROTOCOL.md`](./_docs/WEBSOCKET_PROTOCOL.md) | Complete WebSocket message protocol, types, error codes |
| [`_docs/PUBSUB_ARCHITECTURE.md`](./_docs/PUBSUB_ARCHITECTURE.md) | Redis Pub/Sub architecture for scaling |
| [`_docs/CHANGELOG/`](./_docs/CHANGELOG/)                | Historical change logs                       |

## Testing

Open `test-websocket-chat.html` or `test-group-chat.html` in your browser to test WebSocket functionality.

```bash
# Manual WebSocket testing
wscat -c "ws://localhost:8080/echo/v1/websocket/chat" -H "Authorization: Bearer <jwt_token>"