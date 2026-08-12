# Error Handling — ECHO Android

## Rule

1. **Errors are values.** Return `Result<T>` from use cases and repositories. Never `throw` across layer boundaries.
2. **Every error maps to a user-visible message** before reaching a Composable. Raw exceptions never leak to the UI.
3. **Backend error codes** (from the WebSocket protocol) are mapped to local string resources.
4. **Crash loops:** a repeated unhandled exception in the same component must trigger fallback UI, not a restart.
5. **Structured logging** via Timber — `Timber.tag("ChatRepo").e(err, "failed to send message")`, never `printStackTrace()`.

## Why

- Consistent error handling prevents the app from crashing on every network hiccup.
- Mapping backend error codes (`PERMISSION_DENIED`, `INVALID_CONTENT`) to user-friendly strings means the user always sees a meaningful message, not "java.net.ConnectException".
- Crash loops (e.g. a composable that crashes on every recomposition) are the #1 cause of ANR dialogs. A fallback UI is a safety net.

## How to apply

### 1. Use `Result<T>` at repository boundaries

```kotlin
// ✓ GOOD — Result wraps outcome, caller decides what to do
class ChatRepositoryImpl(
    private val api: ChatApiService
) : ChatRepository {
    override suspend fun getChatMessages(chatId: String, page: Int): Result<List<Message>> {
        return runCatching {
            api.getMessages(chatId, page = page, limit = 50)
                .messages
                .map { it.toDomain() }
        }
    }
}

// ✗ BAD — throw leaks implementation detail to caller
override suspend fun getChatMessages(chatId: String): List<Message> {
    return api.getMessages(chatId).messages.map { it.toDomain() }
    // What if api throws? Caller doesn't know.
}
```

### 2. Map errors to UI state in the ViewModel

```kotlin
// ✓ GOOD — catch Result, map to UiState
class ChatListViewModel(
    private val getChats: GetChatListUseCase
) : ViewModel() {
    private val _state = MutableStateFlow(ChatListUiState())
    val state: StateFlow<ChatListUiState> = _state.asStateFlow()

    fun loadChats() {
        viewModelScope.launch {
            _state.update { it.copy(isLoading = true, error = null) }
            getChats()
                .onSuccess { chats ->
                    _state.update { it.copy(isLoading = false, chats = chats) }
                }
                .onFailure { err ->
                    Timber.tag("ChatListVM").e(err, "failed to load chats")
                    _state.update {
                        it.copy(
                            isLoading = false,
                            error = err.toUserMessage()
                        )
                    }
                }
        }
    }
}
```

### 3. Sealed UiState with error

```kotlin
// Never pass Exception to Composables. Map to a string first.
data class ChatListUiState(
    val isLoading: Boolean = false,
    val chats: List<Chat> = emptyList(),
    val error: String? = null  // Human-readable, already mapped
)

// ✓ GOOD — sealed hierarchy for complex states
sealed interface ChatUiState {
    data object Loading : ChatUiState
    data class Success(val messages: List<Message>) : ChatUiState
    data class Error(val message: String, val retry: () -> Unit) : ChatUiState
}
```

### 4. Map backend error codes to user messages

```kotlin
// core/util/ErrorMapper.kt
fun Exception.toUserMessage(): String = when {
    this is java.net.ConnectException -> "No internet connection. Check your network."
    this is java.net.SocketTimeoutException -> "Server is taking too long. Try again."
    this is BackendError && code == "PERMISSION_DENIED" -> "You don't have permission to do that."
    this is BackendError && code == "INVALID_CONTENT" -> "Message contains invalid content."
    this is BackendError && code == "MESSAGE_FAILED" -> "Failed to send message. Tap to retry."
    this is BackendError && code == "RATE_LIMIT_EXCEEDED" -> "Too many requests. Slow down."
    this is BackendError && code == "CHANNEL_FULL" -> "Server is busy. Please wait."
    else -> "Something went wrong. Please try again."
}
```

### 5. Backend error codes table (from `_docs/WEBSOCKET_PROTOCOL.md`)

| Code | User message |
| --- | --- |
| `INVALID_REQUEST` | "Invalid request. Please try again." |
| `PERMISSION_DENIED` | "You don't have permission to do that." |
| `INVALID_CHAT_ID` | "Chat not found." |
| `INVALID_CONTENT` | "Message contains invalid content." |
| `MESSAGE_FAILED` | "Failed to send message. Tap to retry." |
| `REACTION_FAILED` | "Failed to add reaction." |
| `PARSE_ERROR` | "Server couldn't process the request." |
| `RATE_LIMIT_EXCEEDED` | "Too many requests. Slow down." |
| `CHANNEL_FULL` | "Server is busy. Please wait." |
| `TOKEN_VERIFICATION_FAILED` | "Session expired. Please log in again." |
| `USER_NOT_AUTHENTICATED` | "Please log in to continue." |
| `NOT_IMPLEMENTED` | "This feature is coming soon." |
| `UNKNOWN_REQUEST` | "Unknown request type." |

### 6. Never `printStackTrace()`, always Timber

```kotlin
// ✓ GOOD — structured, tagged, searchable
Timber.tag("WebSocketClient").e(err, "send frame failed: %s", frameType)

// ✗ BAD — raw stack trace to logcat, no tag, no context
err.printStackTrace()
```

### 7. Crash loop prevention in Compose

```kotlin
// If a screen crashes on every recomposition, show fallback instead of restarting.
@Composable
fun SafeScreen(content: @Composable () -> Unit) {
    var crashCount by remember { mutableIntStateOf(0) }

    if (crashCount > 2) {
        // Stop retrying — show recovery UI
        FallbackErrorScreen(
            message = "Something went wrong.",
            onRetry = { crashCount = 0 }  // Reset and retry once user taps
        )
        return
    }

    try {
        content()
    } catch (e: Exception) {
        Timber.tag("SafeScreen").e(e, "composable crash #%d", crashCount + 1)
        crashCount++
    }
}
```

### 8. Coroutine exception handling

```kotlin
// ✓ GOOD — supervisorScope: one child failure doesn't cancel siblings
viewModelScope.launch {
    supervisorScope {
        launch { loadChats() }       // if this fails,
        launch { updatePresence() }  // this still runs
    }
}

// ✓ GOOD — catch in launch for fire-and-forget
viewModelScope.launch {
    try {
        markAsRead(messageIds)
    } catch (e: Exception) {
        Timber.tag("ChatVM").e(e, "failed to mark as read")
    }
}
```

### 9. Crash reporting (production)

```kotlin
// core/util/CrashTree.kt — Timber tree that reports to Firebase Crashlytics
class CrashReportingTree : Timber.Tree() {
    override fun log(priority: Int, tag: String?, message: String, t: Throwable?) {
        if (priority == Log.ERROR || priority == Log.WARN) {
            t?.let { Firebase.crashlytics.recordException(it) }
        }
    }
}

// Plant in Application.onCreate
if (!BuildConfig.DEBUG) {
    Timber.plant(CrashReportingTree())
}
```

### 10. Error vs nil distinction in data layer

```kotlin
// ✓ GOOD — null = not found (expected), exception = failure (unexpected)
override suspend fun findChatById(chatId: String): Result<Chat?> {
    return runCatching {
        try {
            api.getChat(chatId).toDomain()
        } catch (e: HttpException) {
            if (e.code() == 404) return Result.success(null)  // not found = ok
            throw e  // other HTTP errors = failure
        }
    }
}