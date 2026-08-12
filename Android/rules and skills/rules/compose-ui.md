# Jetpack Compose UI — ECHO Android

## Rule

1. **Unidirectional Data Flow (UDF):** State flows down from ViewModel → Composable. Events flow up from Composable → ViewModel.
2. **No `var` in Composables for business state.** Use `remember` only for UI-internal state (animation, text fields, scroll position).
3. **Stateless composables:** Reusable composables accept state as parameters and emit events via lambdas — never hold their own ViewModel reference.
4. **Material 3** is the design system. Theme tokens (`MaterialTheme.colorScheme`, `MaterialTheme.typography`) are used exclusively — no hardcoded colors or text sizes.
5. **Preview annotations** on every screen-level composable with both light and dark theme variants.

## Why

- UDF makes the UI predictable: given the same state, the UI renders identically.
- Stateless composables are trivially testable with Compose testing APIs — just pass state and verify.
- Material 3 tokens enable one-look dark mode and consistent spacing/typography across the app.
- Previews catch layout regressions instantly without deploying to a device.

## How to apply

### 1. UDF pattern — ViewModel to Composable

```kotlin
// ViewModel owns state and logic
@HiltViewModel
class ChatListViewModel @Inject constructor(
    private val getChats: GetChatListUseCase
) : ViewModel() {
    private val _state = MutableStateFlow(ChatListUiState())
    val state: StateFlow<ChatListUiState> = _state.asStateFlow()

    fun onEvent(event: ChatListEvent) {
        when (event) {
            is ChatListEvent.Refresh -> loadChats()
            is ChatListEvent.ChatClicked -> navigateToChat(event.chatId)
        }
    }
}

// Composable observes state, emits events
@Composable
fun ChatListScreen(
    viewModel: ChatListViewModel = hiltViewModel(),
    onChatClicked: (String) -> Unit
) {
    val state by viewModel.state.collectAsStateWithLifecycle()

    ChatListContent(
        chats = state.chats,
        isLoading = state.isLoading,
        error = state.error,
        onRefresh = { viewModel.onEvent(ChatListEvent.Refresh) },
        onChatClicked = onChatClicked
    )
}

// Stateless content composable — reusable, testable
@Composable
private fun ChatListContent(
    chats: List<Chat>,
    isLoading: Boolean,
    error: String?,
    onRefresh: () -> Unit,
    onChatClicked: (String) -> Unit
) {
    when {
        isLoading -> LoadingIndicator()
        error != null -> ErrorBanner(message = error, onRetry = onRefresh)
        chats.isEmpty() -> EmptyState(message = "No chats yet")
        else -> ChatLazyColumn(chats = chats, onChatClicked = onChatClicked)
    }
}
```

### 2. State hoisting

```kotlin
// ✓ GOOD — state hoisted up, stateless children
@Composable
fun MessageInput(
    text: String,
    onTextChange: (String) -> Unit,
    onSend: () -> Unit,
    enabled: Boolean = true
) {
    Row {
        TextField(
            value = text,
            onValueChange = onTextChange,
            enabled = enabled,
            modifier = Modifier.weight(1f)
        )
        IconButton(onClick = onSend, enabled = text.isNotBlank()) {
            Icon(Icons.Default.Send, contentDescription = "Send")
        }
    }
}

// ✗ BAD — state hidden inside, impossible to test or reuse
@Composable
fun MessageInput() {
    var text by remember { mutableStateOf("") }
    // ... local state, no way to control from outside
}
```

### 3. `remember` rules

```kotlin
// ✓ GOOD — remember for UI-internal state
var isExpanded by remember { mutableStateOf(false) }
val scrollState = rememberScrollState()

// ✗ BAD — remember for business data
var messages by remember { mutableStateOf(emptyList<Message>()) }
// This won't survive config changes or process death. Use ViewModel.
```

### 4. Material 3 theming

```kotlin
// core/theme/Theme.kt
@Composable
fun EchoTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    content: @Composable () -> Unit
) {
    val colorScheme = if (darkTheme) DarkColorScheme else LightColorScheme

    MaterialTheme(
        colorScheme = colorScheme,
        typography = EchoTypography,
        shapes = EchoShapes,
        content = content
    )
}

// Usage — never hardcode colors or text styles
Text(
    text = message.content,
    style = MaterialTheme.typography.bodyLarge,
    color = MaterialTheme.colorScheme.onSurface
)
```

### 5. Previews

```kotlin
@Preview(name = "Light", showBackground = true)
@Preview(name = "Dark", showBackground = true, uiMode = UI_MODE_NIGHT_YES)
@Composable
private fun ChatListContentPreview() {
    EchoTheme {
        ChatListContent(
            chats = previewChats,
            isLoading = false,
            error = null,
            onRefresh = {},
            onChatClicked = {}
        )
    }
}

// Fake preview data
private val previewChats = listOf(
    Chat(id = "1", name = "Alice", lastMessage = "See you soon!"),
    Chat(id = "2", name = "Bob", lastMessage = "Ok 👍")
)
```

### 6. LazyColumn keys

```kotlin
// ✓ GOOD — stable keys for smooth animations and correct diffing
LazyColumn {
    items(
        items = messages,
        key = { it.id }  // ← use message ID, not index
    ) { message ->
        MessageBubble(message = message)
    }
}

// ✗ BAD — no key, UI glitches on insert/delete
LazyColumn {
    items(messages) { message -> MessageBubble(message = message) }
}
```

### 7. `derivedStateOf` for expensive computations

```kotlin
// ✓ GOOD — recomputed only when inputs change
val unreadCount by remember {
    derivedStateOf {
        chats.count { it.unreadCount > 0 }
    }
}

// ✗ BAD — recomputed on every recomposition
val unreadCount = chats.count { it.unreadCount > 0 }
```

### 8. Compose lifecycle awareness

```kotlin
// ✓ GOOD — use lifecycle-aware flow collection
val state by viewModel.state.collectAsStateWithLifecycle()

// Add dependency: androidx.lifecycle:lifecycle-runtime-compose

// ✗ BAD — collectAsState leaks after composable leaves composition
val state by viewModel.state.collectAsState()
```

### 9. Scaffold and navigation

```kotlin
@Composable
fun EchoMainScreen(navController: NavHostController) {
    Scaffold(
        topBar = { EchoTopBar() },
        bottomBar = { EchoBottomBar(navController) }
    ) { paddingValues ->
        NavHost(
            navController = navController,
            startDestination = "chats",
            modifier = Modifier.padding(paddingValues)
        ) {
            composable("chats") { ChatListScreen(onChatClicked = { id -> navController.navigate("chat/$id") }) }
            composable("chat/{chatId}") { backStackEntry ->
                ChatScreen(chatId = backStackEntry.arguments?.getString("chatId") ?: "")
            }
            composable("settings") { SettingsScreen() }
        }
    }
}
```

### 10. Image loading (Coil)

```kotlin
// Use Coil for async image loading — it integrates with Compose natively
AsyncImage(
    model = user.avatarUrl,
    contentDescription = "${user.name}'s avatar",
    modifier = Modifier
        .size(48.dp)
        .clip(CircleShape),
    placeholder = painterResource(R.drawable.ic_default_avatar),
    error = painterResource(R.drawable.ic_default_avatar)
)