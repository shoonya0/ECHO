# Kotlin Style — ECHO Android

## Rule

1. **Idiomatic Kotlin:** Use `val` over `var` by default. Prefer expression bodies, destructuring, and scope functions (`let`, `apply`, `also`, `run`) where they improve readability.
2. **Trailing commas** on all multi-line parameter lists (consistent with the Go backend's convention).
3. **`data class` for models, `sealed interface` for ADTs** (UiState, UiEvent, Result types).
4. **Extension functions** for shared utilities — never static util classes.
5. **Naming:** PascalCase for classes, camelCase for functions/properties, UPPER_SNAKE for constants in companion objects.

## Why

- The Go backend uses a pragmatic functional style; the Android side mirrors this with immutability-by-default and expression-oriented code.
- Sealed types encode all possible states — the compiler verifies exhaustive `when` branches, eliminating missing-case bugs.
- Extension functions keep the `core/util` package clean and discoverable via IDE autocomplete.

## How to apply

### 1. `val` by default, expression bodies

```kotlin
// ✓ GOOD
val name = user.name
fun formatMessage(msg: Message): String = "[${msg.timestamp}] ${msg.content}"

// ✗ BAD
var name = user.name  // mutable without reason
fun formatMessage(msg: Message): String {
    return "[${msg.timestamp}] " + msg.content  // block body for single expression
}
```

### 2. Scope functions

```kotlin
// ✓ let for null-safe transformations
val avatarUrl = user.avatar?.let { "$BASE_URL/$it" } ?: DEFAULT_AVATAR

// ✓ apply for initialization
val request = Request.Builder().apply {
    url(wsUrl)
    header("Authorization", "Bearer $token")
}.build()

// ✓ also for side effects (logging)
api.createChat(request).also { Timber.tag("API").d("chat created: ${it.id}") }

// ✗ run/with overused — when let/apply is clearer
user.run {  // harder to read than user.let
    displayName.ifEmpty { username }
}
```

### 3. Sealed interfaces for state

```kotlin
// ✓ GOOD — compiler checks exhaustiveness
sealed interface AuthState {
    data object Loading : AuthState
    data class Authenticated(val user: User) : AuthState
    data class Error(val message: String) : AuthState
    data object NotAuthenticated : AuthState
}

when (state) {
    is AuthState.Loading -> showSpinner()
    is AuthState.Authenticated -> navigateToHome()
    is AuthState.Error -> showError(state.message)
    is AuthState.NotAuthenticated -> showLogin()
}
// Forgetting a branch = compile error. No runtime surprise.
```

### 4. Extension functions over Util classes

```kotlin
// ✓ GOOD — extensions, discoverable and chainable
fun String.isValidEmail(): Boolean =
    android.util.Patterns.EMAIL_ADDRESS.matcher(this).matches()

fun String.truncate(maxLength: Int): String =
    if (length <= maxLength) this else take(maxLength) + "…"

// Usage: "hello@example.com".isValidEmail()

// ✗ BAD — static util class
object StringUtils {
    fun isValidEmail(email: String): Boolean = ...
}
```

### 5. Naming conventions

```kotlin
// Classes: PascalCase
class ChatRepositoryImpl
data class Message(val id: String, val content: String)

// Functions/properties: camelCase
fun sendMessage(chatId: String, content: String)
val isLoading: Boolean

// Constants in companion: UPPER_SNAKE
companion object {
    const val MAX_MESSAGE_LENGTH = 4000
    const val WS_RECONNECT_DELAY_MS = 1000L
}

// Enums: UPPER_SNAKE values, PascalCase class
enum class MessageType { TEXT, IMAGE, FILE, AUDIO, VIDEO }
```

### 6. Trailing commas

```kotlin
// ✓ GOOD — trailing comma on every multi-line list
data class Chat(
    val id: String,
    val name: String,
    val lastMessage: String?,
    val unreadCount: Int,
)

val items = listOf(
    "chat",
    "groups",
    "settings",
)

// ✓ Even last parameter gets a trailing comma — cleaner diffs
```

### 7. Default parameters over overloads

```kotlin
// ✓ GOOD
fun fetchMessages(
    chatId: String,
    page: Int = 1,
    limit: Int = 50,
    before: String? = null,
): Result<List<Message>>

// ✗ BAD
fun fetchMessages(chatId: String): Result<List<Message>> = fetchMessages(chatId, 1)
fun fetchMessages(chatId: String, page: Int): Result<List<Message>> = fetchMessages(chatId, page, 50)
```

### 8. Smart casting and type-safe builders

```kotlin
// ✓ GOOD — smart cast after type check
fun handleResult(result: Result<Any>) {
    when {
        result.isSuccess -> {
            val value = result.getOrNull()  // already checked
            processValue(value)
        }
        result.isFailure -> {
            val err = result.exceptionOrNull()
            logError(err)
        }
    }
}

// ✓ GOOD — apply for builder patterns
val retrofit = Retrofit.Builder().apply {
    baseUrl(BuildConfig.API_BASE_URL)
    client(okHttpClient)
    addConverterFactory(Json.asConverterFactory("application/json".toMediaType()))
}.build()
```

### 9. String templates

```kotlin
// ✓ GOOD
val greeting = "Hello, ${user.displayName}"
val url = "$BASE_URL/echo/v1/chats/$chatId/messages"

// ✓ GOOD — complex expressions go in ${}
val info = "Unread: ${chats.filter { it.unreadCount > 0 }.size}"

// ✗ BAD — concatenation
val greeting = "Hello, " + user.displayName
```

### 10. Collections — prefer immutable

```kotlin
// ✓ GOOD
val tags = listOf("chat", "update")       // List (immutable)
val emptyMessages = emptyList<Message>()  // Explicit empty
val userMap = mapOf("id" to user.id)      // Map (immutable)

// ✗ BAD — return types should be List, not MutableList
fun getChats(): MutableList<Chat>