# Clean Architecture — ECHO Android

## Rule

The ECHO Android app follows a three-layer flow per feature, organized under a shared `core/`:

```
core/                         # Shared, feature-agnostic
├── data/
│   ├── local/                # DataStore, Room, encrypted prefs
│   └── remote/               # Retrofit services, OkHttp client
├── di/                       # Hilt modules (global singletons)
├── theme/                    # Material 3 Color, Type, Theme
└── util/                     # Extensions, formatters, validators

features/
└── chat/
    ├── data/
    │   ├── model/            # DTOs: ChatResponse, MessageResponse
    │   ├── remote/           # ChatApiService, WebSocketClient
    │   └── repository/       # ChatRepositoryImpl
    ├── domain/
    │   ├── model/            # Clean domain: Chat, Message, MessageType
    │   ├── repository/       # ChatRepository interface
    │   └── usecase/          # SendMessageUseCase, GetChatListUseCase
    └── presentation/
        ├── ChatListScreen.kt # Composable
        ├── ChatListUiState.kt# Data class for UI state
        ├── ChatListEvent.kt  # Sealed interface for user actions
        └── ChatListViewModel.kt
```

**Dependency direction:** presentation → domain ← data. Domain has zero Android dependencies. Data implements domain interfaces. Core is imported by features — never the reverse.

## Why

- This layout mirrors the backend's established pattern (`controller/` → `services/` → `models/`), so developers can trace a message from the Android UI all the way to the Go backend without mental context switching.
- Feature-first separation prevents one feature's UI change from breaking another feature's network layer.
- A domain layer free of Android dependencies is instantly testable with JUnit — no Robolectric needed.

## How to apply

### Layer responsibilities

| Layer | Owns | Must not |
| --- | --- | --- |
| `core/data/` | OkHttpClient (singleton), Retrofit instance, DataStore for tokens, Room database. | Contain feature-specific endpoints or models. |
| `core/di/` | @Module providing CoreOkHttpClient, CoreRetrofit, CoreDataStore. | Reference any feature package. |
| `core/theme/` | Material 3 Color scheme, Typography, Theme composable, Shape. | Contain business logic or feature strings. |
| `core/util/` | Extensions (`String.isValidEmail()`), date formatters, error mappers. | Import feature models. |
| `feature/*/data/` | Retrofit interface for that feature, DTOs with `@SerializedName`, repository impl. | Import Android View/Compose types. |
| `feature/*/domain/` | Clean data classes, repository interface, use cases. | Import Android framework, Retrofit, Room. |
| `feature/*/presentation/` | Composables, ViewModel, UiState, UiEvent. | Call Retrofit directly; hold Context references in ViewModel. |

### Example: Sending a message (chat feature)

```
ChatScreen.kt             // user taps send → ViewModel.onEvent(SendMessage)
ChatViewModel.kt          // viewModelScope.launch → SendMessageUseCase
SendMessageUseCase.kt     // chatRepository.sendMessage(chatId, content)
ChatRepositoryImpl.kt     // chatApi.sendMessage(request) or wsClient.send(frame)
ChatApiService.kt         // @POST("chats/{chatId}/messages") suspend fun sendMessage(...)
```

### Core vs feature: decision boundary

| Code | Where it lives | Why |
| --- | --- | --- |
| OkHttpClient with auth interceptor | `core/data/remote/` | Every feature uses the same HTTP client. |
| `@Serializable data class LoginRequest` | `features/auth/data/model/` | Only auth feature needs it. |
| `fun String.isValidEmail(): Boolean` | `core/util/` | Many features validate email. |
| `data class Chat(val id: String, ...)` | `features/chat/domain/model/` | Chat domain model is chat-specific. |
| Hilt `@Module` providing Retrofit | `core/di/` | One Retrofit instance shared across features. |
| `ChatApiService` interface | `features/chat/data/remote/` | Chat endpoints are feature-specific. |

### What goes in the composition root (Application.onCreate)

```kotlin
@HiltAndroidApp
class EchoApplication : Application() {
    // Hilt handles DI wiring.
    // onCreate is for one-time initialization: StrictMode, Timber plant, etc.
    override fun onCreate() {
        super.onCreate()
        if (BuildConfig.DEBUG) {
            StrictMode.enableDefaults()
        }
    }
}
```

### No cross-feature imports

```kotlin
// ✗ BAD — chat feature importing auth internals
import com.shoonya.echo.features.auth.data.remote.AuthApiService

// ✓ GOOD — chat needs auth data? Use a core-level interface or shared utils
import com.shoonya.echo.core.util.getCurrentUserId
```

Features share via `core/` — never by reaching into each other's packages.

### Package naming

```kotlin
com.shoonya.echo.core.data.remote     // shared networking
com.shoonya.echo.features.chat.data.remote  // chat-specific endpoints
com.shoonya.echo.features.auth.data.remote  // auth-specific endpoints
```

The applicationId is `com.Shoonya.Echo` (from build.gradle.kts), but Kotlin package names use lowercase by convention: `com.shoonya.echo`. Configure `namespace` in build.gradle.kts to match.