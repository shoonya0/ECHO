# ECHO Android — WebSocket Client

## Connection

### Endpoint

```
ws://{host}/echo/v1/ws/chat      # Debug (Android emulator: ws://10.0.2.2:8080)
wss://{host}/echo/v1/ws/chat     # Release
```

> **Note:** The backend docs mention `/echo/v1/websocket/chat` but the code (`routes.go`) registers `ws/chat`. The Postman collection confirms `ws/chat`. Use the code-verified path.

### Authentication

JWT Bearer token in `Authorization` header during HTTP upgrade:

```kotlin
val request = Request.Builder()
    .url("${BuildConfig.WS_BASE_URL}/echo/v1/ws/chat")
    .header("Authorization", "Bearer $token")
    .build()
```

## Request Types (Client → Server)

### 1. send_message — Send a Chat Message

```json
{
  "type": "send_message",
  "chatId": "507f1f77bcf86cd799439011",
  "content": "Hello everyone!",
  "messageType": "text",
  "attachments": [],
  "mentions": [],
  "requestId": "req_1"
}
```

**Valid messageType:** `text`, `image`, `file`, `audio`, `video`
**Validation:** content ≤ 4000 chars, max 10 attachments (50MB each), max 20 mentions

### 2. join_chat — Join/Create Chat Room

```json
{
  "type": "join_chat",
  "chatId": "507f1f77bcf86cd799439011",
  "senderId": "target_user_id",
  "requestId": "req_2"
}
```

Creates a direct chat if none exists between authenticated user and `senderId`, then joins.

### 3. leave_chat — Leave Chat Room

```json
{
  "type": "leave_chat",
  "chatId": "507f1f77bcf86cd799439011",
  "requestId": "req_3"
}
```

### 4. set_typing — Typing Indicator

```json
{
  "type": "set_typing",
  "chatId": "507f1f77bcf86cd799439011",
  "metadata": { "isTyping": true },
  "requestId": "req_4"
}
```

### 5. mark_read — Mark Messages as Read

```json
{
  "type": "mark_read",
  "chatId": "507f1f77bcf86cd799439011",
  "metadata": { "messageIds": ["msg_id_1", "msg_id_2"] },
  "requestId": "req_5"
}
```

### 6. add_reaction — Add Reaction

```json
{
  "type": "add_reaction",
  "chatId": "507f1f77bcf86cd799439011",
  "metadata": { "messageId": "msg_id_1", "emoji": "👍" },
  "requestId": "req_6"
}
```

### 7. remove_reaction — Remove Reaction

```json
{
  "type": "remove_reaction",
  "chatId": "507f1f77bcf86cd799439011",
  "metadata": { "messageId": "msg_id_1", "emoji": "👍" },
  "requestId": "req_7"
}
```

### NOT_IMPLEMENTED (Out of Scope)

The following request types return `NOT_IMPLEMENTED` errors from the server. The Android app does **not** build UI actions or command types for these. If a `NOT_IMPLEMENTED` error is ever received (e.g., from a future server version), it's mapped to "This feature is coming soon" via `ErrorMapper`:

- `edit_message`
- `delete_message`
- `invite_user`
- `remove_user`
- `update_chat`

These will be added in a future phase once backend support is complete.

---

## Response Types (Server → Client)

### message — Incoming Chat Message

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

### typing — Typing Indicator

```json
{
  "type": "typing",
  "chatId": "...",
  "userId": "user_id",
  "data": { "userId": "user_id", "username": "username", "isTyping": true, "timestamp": "..." },
  "timestamp": "..."
}
```

### join / leave — User Join/Leave

```json
{
  "type": "join",
  "chatId": "...",
  "userId": "user_id",
  "data": { "userId": "user_id", "username": "username", "action": "join", "timestamp": "..." },
  "timestamp": "..."
}
```

### presence — Presence Update

```json
{
  "type": "presence",
  "chatId": "...",
  "userId": "user_id",
  "data": { "userId": "user_id", "status": "online", "lastSeen": "..." },
  "timestamp": "..."
}
```

### delivery — Read Receipt

```json
{
  "type": "delivery",
  "chatId": "...",
  "userId": "user_id",
  "data": { "messageIds": [...], "userId": "...", "status": "read" },
  "timestamp": "..."
}
```

### reaction — Reaction Event

```json
{
  "type": "reaction",
  "chatId": "...",
  "userId": "user_id",
  "data": { "messageId": "...", "userId": "...", "username": "...", "emoji": "👍", "action": "add", "timestamp": "..." },
  "timestamp": "..."
}
```

### response — Success Acknowledgment

```json
{
  "type": "response",
  "data": { "success": true, "messageId": "created_msg", "timestamp": "...", "chatId": "...", "messageType": "text" },
  "requestId": "original_request_id",
  "timestamp": "..."
}
```

### error — Error Response

```json
{
  "type": "error",
  "data": { "code": "PERMISSION_DENIED", "message": "You don't have permission to send messages in this chat", "details": "..." },
  "requestId": "original_request_id",
  "timestamp": "..."
}
```

---

## Implementation Architecture

### 1. EchoWebSocketClient

```kotlin
// features/chat/data/remote/EchoWebSocketClient.kt
@Singleton
class EchoWebSocketClient @Inject constructor(
    @WsClient private val okHttpClient: OkHttpClient,
    private val tokenStore: SecureTokenStore,
) {
    private var webSocket: WebSocket? = null
    private val _events = MutableSharedFlow<WebSocketEvent>(replay = 0)
    val events: SharedFlow<WebSocketEvent> = _events.asSharedFlow()

    fun connect() {
        val token = tokenStore.getAccessToken() ?: run {
            _events.tryEmit(WebSocketEvent.Error("No auth token"))
            return
        }
        val request = Request.Builder()
            .url("${BuildConfig.WS_BASE_URL}/echo/v1/ws/chat")
            .header("Authorization", "Bearer $token")
            .build()

        webSocket = okHttpClient.newWebSocket(request, object : WebSocketListener() {
            override fun onOpen(ws: WebSocket, response: Response) {
                Timber.tag("WS").i("connected")
                _events.tryEmit(WebSocketEvent.Connected)
            }

            override fun onMessage(ws: WebSocket, text: String) {
                _events.tryEmit(WebSocketEvent.Message(text))
            }

            override fun onFailure(ws: WebSocket, t: Throwable, response: Response?) {
                Timber.tag("WS").e(t, "connection failed, code=%d", response?.code ?: -1)
                _events.tryEmit(WebSocketEvent.Error(t.message ?: "Connection failed"))
            }

            override fun onClosing(ws: WebSocket, code: Int, reason: String) {
                ws.close(1000, null)
            }

            override fun onClosed(ws: WebSocket, code: Int, reason: String) {
                _events.tryEmit(WebSocketEvent.Disconnected(code, reason))
            }
        })
    }

    fun send(text: String): Boolean {
        return webSocket?.send(text) ?: false
    }

    fun disconnect() {
        webSocket?.close(1000, "User disconnected")
        webSocket = null
    }
}
```

### 2. ReconnectManager

```kotlin
// features/chat/data/remote/ReconnectManager.kt
class ReconnectManager(
    private val wsClient: EchoWebSocketClient,
    private val scope: CoroutineScope,
) {
    private var retryCount = 0
    private val maxRetries = 5
    private val baseDelay = 1_000L

    fun onDisconnected() {
        if (retryCount >= maxRetries) {
            Timber.tag("Reconnect").w("max retries reached")
            _events.tryEmit(WebSocketEvent.ReconnectFailed)
            return
        }
        val delay = (baseDelay * (1L shl retryCount)) + Random.nextLong(0, 1000)
        retryCount++
        scope.launch {
            delay(delay)
            wsClient.connect()
        }
    }

    fun reset() { retryCount = 0 }
}
```

### 3. WebSocketRepository — requestId Correlation

```kotlin
// features/chat/data/repository/WebSocketRepository.kt
@Singleton
class WebSocketRepository @Inject constructor(
    private val wsClient: EchoWebSocketClient,
    private val json: Json,
) {
    private val pendingRequests = ConcurrentHashMap<String, CompletableDeferred<ServerFrame>>()

    suspend fun send(command: WebSocketCommand): ServerFrame {
        val deferred = CompletableDeferred<ServerFrame>()
        pendingRequests[command.requestId] = deferred
        val text = json.encodeToString(WebSocketCommand.serializer(), command)
        wsClient.send(text)
        return withTimeout(10_000) { deferred.await() }
    }

    fun handleFrame(frame: ServerFrame) {
        val requestId = frame.requestId ?: return
        pendingRequests.remove(requestId)?.complete(frame)
    }
}
```

### 4. ServerFrame Models

```kotlin
// features/chat/domain/model/ServerFrame.kt
@Serializable
sealed interface ServerFrame {
    val type: String
    val timestamp: String?
    val requestId: String?

    @Serializable @SerialName("message")
    data class ChatMessage(
        val chatId: String, val userId: String, val username: String, val data: MessageData,
        override val type: String = "message", override val timestamp: String? = null, override val requestId: String? = null,
    ) : ServerFrame

    @Serializable @SerialName("typing")
    data class Typing(
        val chatId: String, val userId: String, val data: TypingData,
        override val type: String = "typing", override val timestamp: String? = null, override val requestId: String? = null,
    ) : ServerFrame

    @Serializable @SerialName("presence")
    data class Presence(
        val chatId: String? = null, val userId: String, val data: PresenceData,
        override val type: String = "presence", override val timestamp: String? = null, override val requestId: String? = null,
    ) : ServerFrame

    @Serializable @SerialName("reaction")
    data class Reaction(
        val chatId: String, val userId: String, val data: ReactionData,
        override val type: String = "reaction", override val timestamp: String? = null, override val requestId: String? = null,
    ) : ServerFrame

    @Serializable @SerialName("delivery")
    data class Delivery(
        val chatId: String, val userId: String, val data: DeliveryData,
        override val type: String = "delivery", override val timestamp: String? = null, override val requestId: String? = null,
    ) : ServerFrame

    @Serializable @SerialName("join")
    data class Join(
        val chatId: String, val userId: String, val data: JoinLeaveData,
        override val type: String = "join", override val timestamp: String? = null, override val requestId: String? = null,
    ) : ServerFrame

    @Serializable @SerialName("leave")
    data class Leave(
        val chatId: String, val userId: String, val data: JoinLeaveData,
        override val type: String = "leave", override val timestamp: String? = null, override val requestId: String? = null,
    ) : ServerFrame

    @Serializable @SerialName("response")
    data class Response(
        val data: ResponseData,
        override val type: String = "response", override val timestamp: String? = null, override val requestId: String? = null,
    ) : ServerFrame

    @Serializable @SerialName("error")
    data class Error(
        val data: ErrorData,
        override val type: String = "error", override val timestamp: String? = null, override val requestId: String? = null,
    ) : ServerFrame
}

// Sub-models
@Serializable data class MessageData(val id: String, val chatId: String, val senderId: String, val content: String, val messageType: String = "text", val attachments: List<AttachmentDto> = emptyList(), val createdAt: String? = null)
@Serializable data class TypingData(val userId: String, val username: String? = null, val isTyping: Boolean, val timestamp: String? = null)
@Serializable data class PresenceData(val userId: String, val status: String, val lastSeen: String? = null)
@Serializable data class ReactionData(val messageId: String, val userId: String, val username: String? = null, val emoji: String, val action: String, val timestamp: String? = null)
@Serializable data class DeliveryData(val messageIds: List<String>, val userId: String, val status: String)
@Serializable data class JoinLeaveData(val userId: String, val username: String? = null, val action: String, val timestamp: String? = null)
@Serializable data class ResponseData(val success: Boolean, val messageId: String? = null, val timestamp: String? = null, val chatId: String? = null, val messageType: String? = null)
@Serializable data class ErrorData(val code: String, val message: String, val details: String? = null)
```

### 5. Domain Events (for ViewModel consumption)

```kotlin
// features/chat/domain/model/DomainEvent.kt
sealed interface DomainEvent {
    data class NewMessage(val chatId: String, val message: Message) : DomainEvent
    data class TypingUpdate(val chatId: String, val userId: String, val isTyping: Boolean) : DomainEvent
    data class PresenceUpdate(val userId: String, val status: PresenceStatus) : DomainEvent
    data class ReactionUpdate(val chatId: String, val messageId: String, val emoji: String, val action: String) : DomainEvent
    data class ReadReceipt(val chatId: String, val messageIds: List<String>, val userId: String) : DomainEvent
    data class UserJoined(val chatId: String, val userId: String) : DomainEvent
    data class UserLeft(val chatId: String, val userId: String) : DomainEvent
    data class ResponseAck(val requestId: String, val messageId: String?) : DomainEvent
    data class BackendError(val code: String, val message: String) : DomainEvent
}

fun ServerFrame.toDomainEvent(): DomainEvent = when (this) {
    is ServerFrame.ChatMessage -> DomainEvent.NewMessage(chatId, data.toMessage(chatId, userId))
    is ServerFrame.Typing -> DomainEvent.TypingUpdate(chatId, userId, data.isTyping)
    is ServerFrame.Presence -> DomainEvent.PresenceUpdate(userId, try { PresenceStatus.valueOf(data.status.uppercase()) } catch (_: Exception) { PresenceStatus.OFFLINE })
    is ServerFrame.Reaction -> DomainEvent.ReactionUpdate(chatId, data.messageId, data.emoji, data.action)
    is ServerFrame.Delivery -> DomainEvent.ReadReceipt(chatId, data.messageIds, data.userId)
    is ServerFrame.Join -> DomainEvent.UserJoined(chatId, userId)
    is ServerFrame.Leave -> DomainEvent.UserLeft(chatId, userId)
    is ServerFrame.Response -> DomainEvent.ResponseAck(requestId ?: "", data.messageId)
    is ServerFrame.Error -> DomainEvent.BackendError(data.code, data.message)
}
```

---

## ViewModel Integration

```kotlin
@HiltViewModel
class ChatDetailViewModel @Inject constructor(
    private val wsRepository: WebSocketRepository,
    private val chatRepository: ChatRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {
    private val chatId: String = savedStateHandle["chatId"]!!
    private val _state = MutableStateFlow(ChatDetailUiState())
    val state: StateFlow<ChatDetailUiState> = _state.asStateFlow()

    init {
        // Connect and start listening
        viewModelScope.launch {
            wsRepository.events.collect { event ->
                when (event) {
                    is DomainEvent.NewMessage -> onNewMessage(event)
                    is DomainEvent.TypingUpdate -> onTypingUpdate(event)
                    is DomainEvent.PresenceUpdate -> onPresenceUpdate(event)
                    is DomainEvent.ReactionUpdate -> onReactionUpdate(event)
                    is DomainEvent.ReadReceipt -> onReadReceipt(event)
                    is DomainEvent.BackendError -> onError(event)
                    else -> { /* no-op */ }
                }
            }
        }
        joinChat()
    }

    private fun joinChat() {
        viewModelScope.launch {
            wsRepository.send(WebSocketCommand.JoinChat(chatId, senderId = myUserId))
        }
    }

    fun sendMessage(content: String) {
        viewModelScope.launch {
            _state.update { it.copy(isSending = true) }
            wsRepository.send(WebSocketCommand.SendMessage(chatId, content))
                .let { response ->
                    _state.update { it.copy(isSending = false) }
                }
        }
    }

    fun setTyping(isTyping: Boolean) {
        wsRepository.sendRaw(WebSocketCommand.SetTyping(chatId, TypingMetadata(isTyping)))
    }

    override fun onCleared() {
        super.onCleared()
        viewModelScope.launch {
            wsRepository.send(WebSocketCommand.LeaveChat(chatId))
        }
    }
}
```

---

## Connection Lifecycle

```
App Start
  └── Login → token stored
       └── HomeScreen (ChatsTab)
            └── ChatDetailScreen init → WebSocket connect()
                 ├── onOpen → send join_chat(chatId)
                 ├── onMessage → parse → emit events
                 ├── onFailure → ReconnectManager.backoff()
                 └── onCleared → send leave_chat → disconnect()
```

### Keepalive

- Server pings every 54 seconds
- Client configured with `pingInterval(54, TimeUnit.SECONDS)` in OkHttp
- Read timeout set to `0` (infinite) for WebSocket

### Reconnection

- Exponential backoff: 1s → 2s → 4s → 8s → 16s
- Random jitter (0-1000ms)
- Max 5 retries, then show "Connection lost" banner

---

## Permissions (per Chat Type)

| Chat Type | Who Can Send |
|---|---|
| `direct` | All participants (unless blocked/muted) |
| `group` | Based on participant permissions; defaults to allow all |
| `channel` | Only `admin`/`owner` roles |

---

## Configuration

```kotlin
// In NetworkModule
@Provides @Singleton @WsClient
fun provideWsOkHttpClient(): OkHttpClient {
    return OkHttpClient.Builder()
        .pingInterval(54, TimeUnit.SECONDS)
        .readTimeout(0, TimeUnit.MILLISECONDS)
        .connectTimeout(10, TimeUnit.SECONDS)
        .build()
}