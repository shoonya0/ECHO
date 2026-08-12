---
name: error-handling-feedback
description: "Handle errors with a structured feedback loop. Diagnose, fix, verify build, and prevent crash loops. Maps backend error codes to user messages. Use when handling errors, fixing crashes, or implementing error UI in the ECHO Android app."
user-invocable: true
metadata:
  version: "1.0.0"
allowed-tools: Read Edit Write Bash Globbing Grep
---

**Persona:** You are an Android reliability engineer. You treat every error as an event that must either be handled gracefully or logged with enough context to fix. Silent failures and crash loops are equally unacceptable.

**Modes:**

- **Coding mode** — writing new error handling code. Follow the checklist sequentially; map errors to user messages, wrap at layer boundaries, and add Timber logging.
- **Fix mode** — a crash or error was reported. Diagnose via logcat/grep, identify root cause, apply fix, verify with build, then log the resolution.
- **Audit mode** — review existing error handling across a codebase. Search for `printStackTrace()`, bare `throw`, and uncaught exceptions.

> **Project rule:** The `error-handling.md` rule file overrides this skill on conflicts. Always read `Android/rules and skills/rules/error-handling.md` first.

# Error Handling + Feedback Loop

This skill guides structured error handling, crash prevention, and the build-verify feedback cycle for the ECHO Android app.

## The 3-Strike Error Protocol

```
ATTEMPT 1: Diagnose & Fix
  → Read the error carefully (stack trace, Timber tag, error code)
  → Identify root cause (network? parsing? auth? lifecycle?)
  → Apply targeted fix

ATTEMPT 2: Alternative Approach
  → Same error? Try a different method
  → Different Result wrapper? Different exception type?
  → NEVER repeat the exact same failing action

ATTEMPT 3: Broader Rethink
  → Question assumptions about the error source
  → Consider upstream causes (server response shape changed?)
  → Update the plan if needed

AFTER 3 FAILURES: Escalate
  → Document what you tried
  → Share the specific error with the error code
  → Ask for guidance
```

## Error Handling Checklist

Follow these steps when implementing error handling:

### Step 1: Wrap at layer boundaries
- [ ] Data layer: `runCatching { api.call() }` returns `Result<T>`
- [ ] Domain layer: use cases propagate `Result` without catching (unless transforming)
- [ ] Presentation layer: ViewModel catches `Result.failure`, maps to UI state

### Step 2: Map to user message
- [ ] Every exception type has a user-facing string (see table below)
- [ ] Raw exception message never reaches a Composable
- [ ] Use the `Exception.toUserMessage()` extension from `core/util/ErrorMapper.kt`

### Step 3: Log with Timber
- [ ] `Timber.tag("TagName").e(throwable, "context: %s", detail)`
- [ ] No `printStackTrace()` anywhere
- [ ] No sensitive data in log messages (tokens, passwords, message content)

### Step 4: Present in UI
- [ ] `UiState.error: String?` field in every screen state
- [ ] Error composable shows message + retry button
- [ ] `Snackbar` for transient errors, `ErrorBanner` for persistent ones

### Step 5: Verify
- [ ] `./gradlew assembleDebug` passes
- [ ] Test the error path: disconnect network → verify error UI appears
- [ ] Test recovery: reconnect → verify retry works

## Error Code → User Message Table

| Backend Code | User Message |
|---|---|
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
| Network error | "No internet connection. Check your network." |
| Timeout | "Server is taking too long. Try again." |
| Unknown | "Something went wrong. Please try again." |

## Crash Loop Prevention

When a Composable crashes on every recomposition:

```kotlin
// 1. Wrap the screen in SafeScreen
@Composable
fun SafeScreen(content: @Composable () -> Unit) {
    var crashCount by remember { mutableIntStateOf(0) }
    if (crashCount > 2) {
        FallbackErrorScreen(
            message = "Something went wrong.",
            onRetry = { crashCount = 0 }
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

## Common Error Patterns

### Pattern 1: Network call + Result

```kotlin
override suspend fun getChats(): Result<List<Chat>> = runCatching {
    api.getChats().toResult().getOrThrow().chats.map { it.toDomain() }
}
```

### Pattern 2: ViewModel error handling

```kotlin
fun loadChats() {
    viewModelScope.launch {
        _state.update { it.copy(isLoading = true, error = null) }
        getChats()
            .onSuccess { chats -> _state.update { it.copy(isLoading = false, chats = chats) } }
            .onFailure { e ->
                Timber.tag("ChatListVM").e(e, "failed")
                _state.update { it.copy(isLoading = false, error = e.toUserMessage()) }
            }
    }
}
```

### Pattern 3: One-shot error event (Snackbar)

```kotlin
private val _events = MutableSharedFlow<UiEvent>()
val events: SharedFlow<UiEvent> = _events.asSharedFlow()

fun deleteMessage(messageId: String) {
    viewModelScope.launch {
        deleteMessageUseCase(messageId)
            .onFailure { _events.emit(UiEvent.ShowSnackbar("Failed to delete message")) }
    }
}
```

## Feedback Loop: Code → Build → Test → Fix

1. Write error handling code using patterns above.
2. Run `./gradlew assembleDebug`.
3. If build fails, read the error, fix, rebuild — never guess.
4. Run unit tests: `./gradlew testDebugUnitTest`.
5. If tests fail, diagnose with the 3-strike protocol.
6. Document the fix in the PR description.

## Cross-References

- → `Android/rules and skills/rules/error-handling.md` — Project error handling conventions
- → `Android/rules and skills/rules/coroutines-flow.md` — Coroutine exception handling
- → `Android/rules and skills/rules/security.md` — No sensitive data in logs