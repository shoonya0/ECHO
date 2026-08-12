---
name: websocket-integration
description: "Integrate real-time WebSocket chat following the ECHO backend protocol. Triggers on 'websocket', 'real-time', 'connect to server', 'chat protocol', 'reconnect', or 'WebSocket'."
user-invocable: true
metadata:
  version: "1.0.0"
allowed-tools: Read Edit Write Bash Globbing Grep
---

**Persona:** You are a real-time systems engineer. The WebSocket is the lifeline of the ECHO chat app — it must connect reliably, reconnect intelligently, and handle every frame type from the [backend protocol](../../../../_docs/WEBSOCKET_PROTOCOL.md).

> **Project rule:** `Android/rules and skills/rules/websocket-client.md` overrides this skill on conflicts.

# WebSocket Integration Checklist

## Step 1: OkHttp WebSocket Setup

- [ ] OkHttpClient configured with 54s ping interval (matches server keepalive)
- [ ] JWT token injected via `Authorization: Bearer <token>` header
- [ ] Connection URL: `ws://{host}/echo/v1/websocket/chat`
- [ ] SharedFlow emits connection lifecycle events (Connected, Message, Error, Disconnected)

```kotlin
val request = Request.Builder()
    .url("${BuildConfig.WS_BASE_URL}/echo/v1/websocket/chat")
    .header("Authorization", "Bearer $token")
    .build()

webSocket = okHttpClient.newWebSocket(request, object : WebSocketListener() {
    override fun onOpen(ws: WebSocket, response: Response) {
        _events.tryEmit(WebSocketEvent.Connected)
    }
    override fun onMessage(ws: WebSocket, text: String) {
        _events.tryEmit(WebSocketEvent.Message(text))
    }
    override fun onFailure(ws: WebSocket, t: Throwable, response: Response?) {
        _events.tryEmit(WebSocketEvent.Error(t.message ?: "Connection failed"))
    }
    override fun onClosed(ws: WebSocket, code: Int, reason: String) {
        _events.tryEmit(WebSocketEvent.Disconnected(code, reason))
    }
})
```

## Step 2: Request/Response Correlation

- [ ] Every outgoing command has a unique `requestId`
- [ ] `ConcurrentHashMap<String, CompletableDeferred<ServerFrame>>` maps requestIds to awaiters
- [ ] Incoming `response` and `error` frames resolve the matching deferred
- [ ] 10-second timeout on `withTimeout`

```kotlin
suspend fun send(command: WebSocketCommand): ServerFrame {
    val deferred = CompletableDeferred<ServerFrame>()
    pendingRequests[command.requestId] = deferred
    wsClient.send(json.encodeToString(WebSocketCommand::class, command))
    return withTimeout(10_000) { deferred.await() }
}
```

## Step 3: Frame Routing

- [ ] Parse raw JSON to `ServerFrame` sealed hierarchy
- [ ] Route to domain events: `ChatMessage` → `DomainEvent.NewMessage`, `Typing` → `DomainEvent.TypingUpdate`, etc.
- [ ] Broadcast events, not raw JSON, to ViewModels

## Step 4: Reconnection

- [ ] Exponential backoff: 1s → 2s → 4s → 8s → 16s
- [ ] Random jitter (0–1000ms) on each retry
- [ ] Max 5 retries before notifying user
- [ ] Reset retry count on successful connect

```kotlin
val delay = (baseDelay * (1L shl retryCount)) + Random.nextLong(0, 1000)
scope.launch {
    delay(delay)
    wsClient.connect()
}
```

## Step 5: Lifecycle Binding

- [ ] WebSocket connects in ViewModel `init` (or on user login)
- [ ] WebSocket disconnects in ViewModel `onCleared()`
- [ ] All frame collection happens in `viewModelScope`

## Step 6: Supported Frame Types

### Client → Server (Must Implement)
- [ ] `send_message` — chatId, content, messageType, attachments, mentions
- [ ] `join_chat` — chatId, senderId
- [ ] `leave_chat` — chatId
- [ ] `set_typing` — chatId, metadata.isTyping
- [ ] `mark_read` — chatId, metadata.messageIds
- [ ] `add_reaction` — chatId, metadata.messageId, emoji
- [ ] `remove_reaction` — chatId, metadata.messageId, emoji

### Server → Client (Must Handle)
- [ ] `message` — new chat message broadcast
- [ ] `response` — success ack (resolve pending request)
- [ ] `error` — error with code (resolve pending request, show Snackbar)
- [ ] `typing` — typing indicator update
- [ ] `presence` — user online/offline
- [ ] `reaction` — reaction added/removed
- [ ] `join` / `leave` — user join/leave event

## Step 7: Error Handling

- [ ] `INVALID_REQUEST` → show user message, do not crash
- [ ] `PERMISSION_DENIED` → show "You don't have permission"
- [ ] `TOKEN_VERIFICATION_FAILED` / `USER_NOT_AUTHENTICATED` → navigate to login
- [ ] `CHANNEL_FULL` → retry with fallback UI
- [ ] `NOT_IMPLEMENTED` → ignore gracefully (feature planned)

## Verification

- [ ] Connect to `ws://10.0.2.2:8080/echo/v1/websocket/chat` (emulator)
- [ ] Send a `send_message` frame → verify `response` received
- [ ] Join a chat → verify `message` broadcasts received
- [ ] Kill server → verify reconnect attempts
- [ ] Restart server → verify successful reconnect

## Cross-References

- → `../../../../_docs/WEBSOCKET_PROTOCOL.md` — Full backend protocol
- → `Android/rules and skills/rules/websocket-client.md` — Client implementation rules
- → `Android/rules and skills/rules/error-handling.md` — Error code mapping