---
name: compose-best-practices
description: "Jetpack Compose best practices for the ECHO Android app. State hoisting, UDF, theming, LazyColumn keys, previews. Triggers on 'composable', 'UI', 'state hoisting', 'jetpack compose', 'layout', 'preview'."
user-invocable: true
metadata:
  version: "1.0.0"
allowed-tools: Read Edit Write Bash Globbing Grep
---

**Persona:** You are a Compose UI specialist. Every composable is stateless where possible, uses Material 3 tokens, and has previews. State flows down, events flow up. No `var` for business state in composables.

> **Project rule:** `Android/rules and skills/rules/compose-ui.md` overrides this skill on conflicts.

# Compose Best Practices Checklist

## UDF Pattern (Must Follow)

```
State → Composable → Event → ViewModel → State
```

```kotlin
// ViewModel: owns state, exposes StateFlow
@HiltViewModel
class ChatViewModel @Inject constructor(...) : ViewModel() {
    private val _state = MutableStateFlow(ChatUiState())
    val state: StateFlow<ChatUiState> = _state.asStateFlow()

    fun onEvent(event: ChatEvent) { ... }
}

// Screen: observes state, emits events
@Composable
fun ChatScreen(viewModel: ChatViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    ChatContent(state = state, onEvent = viewModel::onEvent)
}

// Content: stateless, testable
@Composable
private fun ChatContent(state: ChatUiState, onEvent: (ChatEvent) -> Unit) {
    // Pure rendering — no ViewModel access
}
```

## State Hoisting Rules

| State type | Where it lives | Example |
|---|---|---|
| Business data | ViewModel → StateFlow | messages list, user info, isLoading |
| UI-internal | `remember` in composable | scroll position, text field focus, expanded state |
| Config-surviving | `rememberSaveable` | text input while typing, tab selection |

```kotlin
// ✓ GOOD — text field state hoisted to ViewModel
TextField(value = state.messageText, onValueChange = { onEvent(ChatEvent.MessageChanged(it)) })

// ✓ GOOD — scroll state local to composable
val scrollState = rememberLazyListState()

// ✓ GOOD — survives config change
var searchQuery by rememberSaveable { mutableStateOf("") }
```

## Material 3 Theming

```kotlin
// ✓ Use theme tokens — never hardcode
Text(text = title, style = MaterialTheme.typography.titleLarge)
Text(text = body, color = MaterialTheme.colorScheme.onSurface)
Surface(color = MaterialTheme.colorScheme.surface) { ... }
Divider(color = MaterialTheme.colorScheme.outlineVariant)

// ✗ BAD
Text(text = title, fontSize = 20.sp, color = Color.Black)
```

## LazyColumn Best Practices

```kotlin
// ✓ GOOD — stable key
LazyColumn {
    items(messages, key = { it.id }) { message ->
        MessageBubble(message)
    }
}

// ✓ GOOD — content type for optimization
LazyColumn {
    items(messages, key = { it.id }, contentType = { it.isMine }) { ... }
}

// ✗ BAD — no key, no contentType
LazyColumn { items(messages) { MessageBubble(it) } }
```

## Previews (Required for Screen Composable)

```kotlin
@Preview(name = "Light", showBackground = true)
@Preview(name = "Dark", uiMode = UI_MODE_NIGHT_YES)
@Composable
private fun ChatContentPreview() {
    EchoTheme {
        ChatContent(
            state = ChatUiState(chats = listOf(Chat("1", "Alice", "Hello"))),
            onEvent = {}
        )
    }
}
```

## Compose + ViewModel Checklist

- [ ] State exposed as `StateFlow<UiState>`, collected with `collectAsStateWithLifecycle()`
- [ ] Events as sealed interface, emitted via `viewModel.onEvent(event)`
- [ ] Content composable is `private` and stateless
- [ ] No `var` for business state in composables — only `remember` for UI state
- [ ] LazyColumn items have stable `key`
- [ ] Material 3 tokens used (no hardcoded colors/sizes)
- [ ] Light + dark previews present
- [ ] `Modifier.testTag()` on interactive elements for UI testing
- [ ] No `Context` references held in ViewModel

## Common Anti-Patterns

```kotlin
// ✗ BAD — ViewModel knows about Context
class BadViewModel(private val context: Context) : ViewModel() { ... }

// ✓ GOOD — Context-free, use AndroidViewModel only if absolutely needed
class GoodViewModel(private val repo: ChatRepository) : ViewModel() { ... }

// ✗ BAD — reading StateFlow in composable without lifecycle awareness
val state = viewModel.state.collectAsState()

// ✓ GOOD
val state by viewModel.state.collectAsStateWithLifecycle()
```

## Cross-References

- → `Android/rules and skills/rules/compose-ui.md` — Full Compose conventions
- → `Android/rules and skills/rules/coroutines-flow.md` — StateFlow and SharedFlow patterns