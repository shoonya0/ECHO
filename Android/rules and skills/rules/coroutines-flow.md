# Coroutines & Flow — ECHO Android

## Rule

1. **`viewModelScope` for UI-bound coroutines**, `CoroutineScope(Dispatchers.IO) + SupervisorJob` for long-lived background work.
2. **`StateFlow` for UI state**, `SharedFlow` for one-shot events (navigation, snackbars, WebSocket frames).
3. **Main-safety:** suspend functions are safe to call from `Dispatchers.Main` — any thread switching happens inside the callee.
4. **`SupervisorJob`** for parallel work where one failure shouldn't cancel siblings.
5. **`Dispatchers.IO`** for network/DB calls, **`Dispatchers.Default`** for CPU-heavy transforms, **`Dispatchers.Main`** for UI updates.

## Why

- `viewModelScope` auto-cancels when the ViewModel is cleared — no leaked coroutines updating a dead UI.
- `StateFlow` is lifecycle-aware when collected with `collectAsStateWithLifecycle()` — no updates while the composable is off-screen.
- Misusing `Job` instead of `SupervisorJob` causes "one failure kills all" bugs that are hard to debug.

## How to apply

### 1. ViewModel coroutine scope

```kotlin
// ✓ GOOD — viewModelScope is a SupervisorScope, auto-cancelled on clear
@HiltViewModel
class ChatViewModel @Inject constructor(
    private val wsRepo: WebSocketRepository
) : ViewModel() {
    init {
        viewModelScope.launch {
            wsRepo.events.collect { event ->
                _state.update { reduceState(it, event) }
            }
        }
    }

    fun sendMessage(chatId: String, content: String) {
        viewModelScope.launch {
            _state.update { it.copy(isSending = true) }
            wsRepo.send(SendMessage(chatId, content))
                .onSuccess { _state.update { it.copy(isSending = false) } }
                .onFailure { e -> _state.update { it.copy(error = e.toUserMessage()) } }
        }
    }
}
```

### 2. Dispatchers — main-safety

```kotlin
// ✓ GOOD — use case calls withContext internally
class GetChatListUseCase(
    private val repo: ChatRepository
) {
    suspend operator fun invoke(): Result<List<Chat>> = withContext(Dispatchers.IO) {
        repo.getChats()
    }
}

// In ViewModel — call from Main dispatcher safely
viewModelScope.launch {  // Main
    val chats = getChatList()  // Switches to IO internally, returns to Main
    _state.update { it.copy(chats = chats.getOrDefault(emptyList())) }
}

// ✗ BAD — blocking the Main thread
viewModelScope.launch(Dispatchers.Main) {
    val result = repo.getChats()  // Network call on Main = jank
}
```

### 3. StateFlow vs SharedFlow

```kotlin
// StateFlow — holds latest value, always has a value
class ChatViewModel : ViewModel() {
    private val _state = MutableStateFlow(ChatUiState())
    val state: StateFlow<ChatUiState> = _state.asStateFlow()
}

// SharedFlow — one-shot events, no replay by default
class AuthViewModel : ViewModel() {
    private val _events = MutableSharedFlow<AuthEvent>()
    val events: SharedFlow<AuthEvent> = _events.asSharedFlow()

    fun onLoginSuccess() {
        viewModelScope.launch {
            _events.emit(AuthEvent.NavigateToHome)
            // ^^ only collected if someone is actively collecting
        }
    }
}
```

### 4. SupervisorScope for parallel work

```kotlin
// ✓ GOOD — load chats AND presence in parallel; one failure doesn't block the other
fun loadData() {
    viewModelScope.launch {
        supervisorScope {
            launch { loadChats() }
            launch { loadPresence() }
            launch { loadContacts() }
        }
    }
}

// ✗ BAD — coroutineScope cancels all children on first failure
fun loadData() {
    viewModelScope.launch {
        coroutineScope {
            launch { loadChats() }      // if this fails,
            launch { loadPresence() }   // this gets cancelled too
        }
    }
}
```

### 5. Structured concurrency — no GlobalScope

```kotlin
// ✓ GOOD — structured, cancellable
viewModelScope.launch { doWork() }

// ✗ BAD — GlobalScope leaks, can't be cancelled
GlobalScope.launch { doWork() }

// ✗ BAD — fire-and-forget without scope
CoroutineScope(Dispatchers.IO).launch { doWork() }  // never cancelled
```

### 6. Flow collection in Compose

```kotlin
@Composable
fun ChatScreen(viewModel: ChatViewModel = hiltViewModel()) {
    // ✓ GOOD — lifecycle-aware
    val state by viewModel.state.collectAsStateWithLifecycle()

    // One-shot events with LaunchedEffect
    LaunchedEffect(Unit) {
        viewModel.events.collect { event ->
            when (event) {
                is AuthEvent.NavigateToHome -> onNavigateToHome()
            }
        }
    }
}
```

### 7. Combining flows

```kotlin
// ✓ GOOD — combine multiple flows
val combinedState: StateFlow<CombinedUiState> = combine(
    chatListState,
    presenceMap,
) { chats, presence ->
    CombinedUiState(
        chats = chats,
        onlineUserIds = presence.filter { it.value == "online" }.keys
    )
}.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5000), CombinedUiState())
```

### 8. Debounce and throttle

```kotlin
// Debounce typing indicator — wait 300ms after last keystroke before sending
viewModelScope.launch {
    textFlow
        .debounce(300)
        .collect { text ->
            if (text.isNotBlank()) wsRepo.setTyping(chatId, true)
            else wsRepo.setTyping(chatId, false)
        }
}
```

### 9. Cancellation

```kotlin
// ✓ GOOD — isActive check for long loops
suspend fun processBatch(items: List<Item>) = withContext(Dispatchers.Default) {
    for (item in items) {
        if (!isActive) break  // respect cancellation
        process(item)
    }
}
```

### 10. Error boundary — catch per launch

```kotlin
// ✓ GOOD — each child handles its own errors
viewModelScope.launch {
    launch {
        try { loadChats() }
        catch (e: Exception) { Timber.tag("VM").e(e, "chats failed") }
    }
    launch {
        try { loadPresence() }
        catch (e: Exception) { Timber.tag("VM").e(e, "presence failed") }
    }
}