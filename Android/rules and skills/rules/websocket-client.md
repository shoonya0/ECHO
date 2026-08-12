# WebSocket Client — ECHO Android

## Rule

Real-time communication with the ECHO backend uses an OkHttp WebSocket client that mirrors the [backend protocol](../../../_docs/WEBSOCKET_PROTOCOL.md):

1. **Connection URL:** `ws://{host}/echo/v1/websocket/chat` (wss:// for production).
2. **Authentication:** JWT Bearer token in the Authorization header during the HTTP upgrade.
3. **Message protocol:** typed JSON frames with `requestId` correlation (request/response matching).
4. **Keepalive:** server sends ping every 54s; client must respond with pong within 60s.
5. **Reconnection:** exponential backoff with jitter on disconnect; max 5 retries before user notification.

## Why

- The WebSocket is the primary communication channel — messages, presence, typing indicators all flow through it. A flaky client creates a broken chat experience.
- OkHttp handles the WebSocket lifecycle (ping/pong, close frames) correctly; a raw `java.net` socket implementation would miss these.
- `requestId` correlation ensures the UI can match "message sent" confirmations to exact outgoing frames, even when messages are sent rapidly.

## How to apply

### 1. Connection setup with OkHttp

```kotlin
// features/chat/data/remote/EchoWebSocketClient.kt
class EchoWebSocketClient @Inject constructor(
    private val okHttpClient: OkHttpClient,
    private val tokenProvider: TokenProvider
) {
    private var webSocket: WebSocket? = null
    private val _events = MutableSharedFlow<WebSocketEvent>(replay = 0)
    val events: SharedFlow<WebSocketEvent> = _events.asSharedFlow()

    fun connect() {
        val token = tokenProvider.getAccessToken() ?: run {
            _events.tryEmit(WebSocketEvent.Error("No auth token"))
            return
        }
        val request = Request.Builder()
            .url("${BuildConfig.WS_BASE_URL}/echo/v1/websocket/chat")
            .header("Authorization", "Bearer $token")
            .build()

        webSocket = okHttpClient.newWebSocket(request, object : WebSocketListener() {
            override fun onOpen(ws: WebSocket, response: Response) {
                Timber.tag("WS").i("connected")
                _events.tryEmit(WebSocketEvent.Connected)
            }

            override fun onMessage(ws: WebSocket, text: String) {
                Timber.tag("WS").d("received: %s", text.take(200))
                _events.tryEmit(WebSocketEvent.Message(text))
            }

            override fun onFailure(ws: WebSocket, t: Throwable, response: Response?) {
                Timber.tag("WS").e(t, "connection failed, code=%d", response?.code ?: -1)
                _events.tryEmit(WebSocketEvent.Error(t.message ?: "Connection failed"))
            }

            override fun onClosing(ws: WebSocket, code: Int, reason: String) {
                Timber.tag("WS").i("closing: %d %s", code, reason)
                ws.close(1000, null)
            }

            override fun onClosed(ws: WebSocket, code: Int, reason: String) {
                Timber.tag("WS").i("closed: %d %s", code, reason)
                _events.tryEmit(WebSocketEvent.Disconnected(code, reason))
            }
        })
    }

    fun disconnect() {
        webSocket?.close(1000, "User disconnected")
        webSocket = null
    }
}
```

### 2. Reconnection with exponential backoff

```kotlin
// features/chat/data/remote/ReconnectManager.kt
class ReconnectManager(
    private val wsClient: EchoWebSocketClient
) {
    private var retryCount = 0
    private val maxRetries = 5
    private val baseDelay = 1_000L  // 1 second

    fun onDisconnected() {
        if (retryCount >= maxRetries) {
            Timber.tag("Reconnect").w("max retries reached, stopping")
            _events.tryEmit(WebSocketEvent.ReconnectFailed)
            return
        }
        val delay = (baseDelay * (1L shl retryCount)) + Random.nextLong(0, 1000)
        Timber.tag("Reconnect").i("retry %d in %dms", retryCount + 1, delay)
        retryCount++
        // Schedule via coroutine, not Thread.sleep
        scope.launch {
            delay(delay)
            wsClient.connect()
        }
    }

    fun reset() {
        retryCount = 0
    }
}
```

### 3. Message protocol — request types

Map all backend request types (from `_docs/WEBSOCKET_PROTOCOL.md`) to sealed classes:

```kotlin
// features/chat/domain/model/WebSocketCommand.kt
@Serializable
sealed interface WebSocketCommand {
    val requestId: String

    @Serializable
    @SerialName("send_message")
    data class SendMessage(
        val chatId: String,
        val content: String,
        val messageType: String = "text",
        val attachments: List<Attachment> = emptyList(),
        val mentions: List<String> = emptyList(),
        override val requestId: String = generateRequestId()
    ) : WebSocketCommand

    @Serializable
    @SerialName("join_chat")
    data class JoinChat(
        val chatId: String,
        val senderId: String,
        override val requestId: String = generateRequestId()
    ) : WebSocketCommand

    @Serializable
    @SerialName("leave_chat")
    data class LeaveChat(
        val chatId: String,
        override val requestId: String = generateRequestId()
    ) : WebSocketCommand

    @Serializable
    @SerialName("set_typing")
    data class SetTyping(
        val chatId: String,
        val metadata: TypingMetadata,
        override val requestId: String = generateRequestId()
    ) : WebSocketCommand

    @Serializable
    @SerialName("mark_read")
    data class MarkRead(
        val chatId: String,
        val metadata: MarkReadMetadata,
        override val requestId: String = generateRequestId()
    ) : WebSocketCommand

    @Serializable
    @SerialName("add_reaction")
    data class AddReaction(
        val chatId: String,
        val metadata: ReactionMetadata,
        override val requestId: String = generateRequestId()
    ) : WebSocketCommand

    @Serializable
    @SerialName("remove_reaction")
    data class RemoveReaction(
        val chatId: String,
        val metadata: ReactionMetadata,
        override val requestId: String = generateRequestId()
    ) : WebSocketCommand
}
```

### 4. Message protocol — response types

```kotlin
@Serializable
sealed interface WebSocketEvent {
    data object Connected : WebSocketEvent
    data class Message(val raw: String) : WebSocketEvent
    data class Disconnected(val code: Int, val reason: String) : WebSocketEvent
    data class Error(val message: String) : WebSocketEvent
    data object ReconnectFailed : WebSocketEvent
}

// Parsed server frames:
@Serializable
sealed interface ServerFrame {
    val type: String
    val timestamp: String?
    val requestId: String?

    @Serializable @SerialName("message")
    data class ChatMessage(
        val chatId: String,
        val userId: String,
        val username: String,
        val data: MessageData
    ) : ServerFrame {
        override val type = "message"
        override val timestamp = null
        override val requestId = null
    }

    @Serializable @SerialName("response")
    data class Response(
        val data: ResponseData,
        override val requestId: String?,
        override val timestamp: String?
    ) : ServerFrame {
        override val type = "response"
    }

    @Serializable @SerialName("error")
    data class Error(
        val data: ErrorData,
        override val requestId: String?,
        override val timestamp: String?
    ) : ServerFrame {
        override val type = "error"
    }

    @Serializable @SerialName("typing")
    data class Typing(
        val chatId: String,
        val userId: String,
        val data: TypingData
    ) : ServerFrame {
        override val type = "typing"
        override val timestamp = null
        override val requestId = null
    }

    @Serializable @SerialName("presence")
    data class Presence(
        val chatId: String,
        val userId: String,
        val data: PresenceData
    ) : ServerFrame {
        override val type = "presence"
        override val timestamp = null
        override val requestId = null
    }

    @Serializable @SerialName("reaction")
    data class Reaction(
        val chatId: String,
        val userId: String,
        val data: ReactionData
    ) : ServerFrame {
        override val type = "reaction"
        override val timestamp = null
        override val requestId = null
    }
}
```

### 5. Send with requestId and await response

```kotlin
class WebSocketRepository @Inject constructor(
    private val wsClient: EchoWebSocketClient,
    private val json: Json
) {
    private val pendingRequests = ConcurrentHashMap<String, CompletableDeferred<ServerFrame>>()

    suspend fun send(command: WebSocketCommand): ServerFrame {
        val deferred = CompletableDeferred<ServerFrame>()
        pendingRequests[command.requestId] = deferred
        val text = json.encodeToString(WebSocketCommand::class, command)
        wsClient.send(text)
        return withTimeout(10_000) { deferred.await() }
    }

    fun handleFrame(frame: ServerFrame) {
        val requestId = frame.requestId ?: return
        pendingRequests.remove(requestId)?.complete(frame)
    }
}
```

### 6. Frame routing

```kotlin
// Map raw ServerFrame to domain events observable by ViewModels
fun ServerFrame.toDomainEvent(): DomainEvent = when (this) {
    is ServerFrame.ChatMessage -> DomainEvent.NewMessage(
        chatId = chatId,
        message = data.toMessage()
    )
    is ServerFrame.Typing -> DomainEvent.TypingUpdate(
        chatId = chatId,
        userId = userId,
        isTyping = data.isTyping
    )
    is ServerFrame.Presence -> DomainEvent.PresenceUpdate(
        userId = userId,
        status = data.status
    )
    is ServerFrame.Reaction -> DomainEvent.ReactionUpdate(
        chatId = chatId,
        messageId = data.messageId,
        emoji = data.emoji,
        action = data.action
    )
    is ServerFrame.Response -> DomainEvent.ResponseAck(data)
    is ServerFrame.Error -> DomainEvent.BackendError(data.code, data.message)
}
```

### 7. Thread safety

- OkHttp's `WebSocket.send()` is thread-safe — can be called from any thread.
- The `ConcurrentHashMap` for pending requests handles concurrent send operations safely.
- `SharedFlow` delivery from `webSocket.onMessage` to UI is on the OkHttp dispatcher thread. Collect flow on `Dispatchers.Main` for UI updates.

### 8. Lifecycle binding

```kotlin
// In ChatViewModel — connect on active, disconnect on cleared
@HiltViewModel
class ChatViewModel @Inject constructor(
    private val wsClient: EchoWebSocketClient
) : ViewModel() {
    init {
        wsClient.connect()
        viewModelScope.launch {
            wsClient.events.collect { event ->
                when (event) {
                    is WebSocketEvent.Message -> handleRawMessage(event.raw)
                    is WebSocketEvent.Error -> handleConnectionError(event)
                    is WebSocketEvent.Disconnected -> reconnect()
                    is WebSocketEvent.Connected -> onConnected()
                    else -> { /* no-op */ }
                }
            }
        }
    }

    override fun onCleared() {
        super.onCleared()
        wsClient.disconnect()
    }
}
```

### 9. Configuration

```kotlin
// core/data/remote/OkHttpProvider.kt
@Module
@InstallIn(SingletonComponent::class)
object NetworkModule {
    @Provides @Singleton
    fun provideOkHttpClient(): OkHttpClient {
        return OkHttpClient.Builder()
            .pingInterval(54, TimeUnit.SECONDS)  // Aligns with server keepalive
            .readTimeout(0, TimeUnit.MILLISECONDS)  // No read timeout for WebSocket
            .connectTimeout(10, TimeUnit.SECONDS)
            .build()
    }
}
```

### 10. BuildConfig endpoints

```kotlin
// androidbuild.gradle.kts
buildTypes {
    debug {
        buildConfigField("String", "WS_BASE_URL", "\"ws://10.0.2.2:8080\"")  // Android emulator → localhost
    }
    release {
        buildConfigField("String", "WS_BASE_URL", "\"wss://echo.example.com\"")
    }
}