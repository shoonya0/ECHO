# Modularity — ECHO Android

## Rule

1. **Feature-first boundaries.** Each feature (`auth`, `chat`, `contacts`, `settings`) is an independent module with its own `data/`, `domain/`, `presentation/` layers.
2. **Core is shared infrastructure only** — DI modules, theme, networking client, token store, utility extensions. Never contains feature-specific business logic.
3. **Cross-feature communication** happens via navigation arguments (primitive types: chat ID, user ID). Never via shared ViewModels or direct class imports.
4. **New features start with 3 files:** a domain model, a use case interface, and a UiState data class. Data layer and UI flesh out from there.
5. **No circular dependencies.** The dependency graph is a DAG: `feature:*` depends on `core`, but `core` does not depend on any feature.

## Why

- Feature-first boundaries let multiple developers work on different features without merge conflicts.
- It's the mirror of the backend's `features/` layout in the Go codebase — the mental model transfers directly.
- Preventing cross-feature imports eliminates "spaghetti dependencies" where changing the auth screen breaks the chat list.

## How to apply

### 1. Feature directory structure

```
features/
├── auth/
│   ├── data/
│   │   ├── model/AuthRequest.kt, AuthResponse.kt
│   │   ├── remote/AuthApiService.kt
│   │   ├── di/AuthDataModule.kt
│   │   └── repository/AuthRepositoryImpl.kt
│   ├── domain/
│   │   ├── model/User.kt, AuthState.kt
│   │   ├── repository/AuthRepository.kt (interface)
│   │   └── usecase/LoginUseCase.kt, SignupUseCase.kt, LogoutUseCase.kt
│   └── presentation/
│       ├── LoginScreen.kt
│       ├── LoginViewModel.kt
│       ├── LoginUiState.kt
│       └── LoginEvent.kt
│
├── chat/
│   ├── data/ (same structure)
│   ├── domain/ (same structure)
│   └── presentation/ (same structure)
│
├── contacts/
│   └── ...
│
└── settings/
    └── ...
```

### 2. Core directory structure

```
core/
├── data/
│   ├── local/SecureTokenStore.kt
│   └── remote/
│       ├── NetworkModule.kt
│       ├── AuthInterceptor.kt
│       └── ApiResponse.kt
├── di/CoreModule.kt
├── theme/
│   ├── Color.kt
│   ├── Type.kt
│   ├── Shape.kt
│   └── Theme.kt
└── util/
    ├── ErrorMapper.kt
    ├── Extensions.kt
    └── DateFormatter.kt
```

### 3. Cross-feature navigation — no direct imports

```kotlin
// ✓ GOOD — navigate via route with arguments
// From ChatScreen:
navController.navigate("contacts/picker?returnTo=chat/${chatId}")

// In ContactsPickerScreen:
val chatId = navController.currentBackStackEntry
    ?.arguments?.getString("returnTo")?.removePrefix("chat/")
// ContactPicker adds the selected user to the chat, then navigates back.

// ✗ BAD — importing another feature's internals
import com.shoonya.echo.features.contacts.presentation.ContactListViewModel
// This creates a hard dependency between features.
```

### 4. Shared navigation graph

```kotlin
// app/MainNavigation.kt — the single source of truth for routes
sealed class Route(val path: String) {
    data object Login : Route("auth/login")
    data object Signup : Route("auth/signup")
    data object ChatList : Route("chat/list")
    data class Chat(val chatId: String) : Route("chat/{chatId}")
    data class ContactPicker(val returnTo: String) : Route("contacts/picker?returnTo={returnTo}")
    data object Settings : Route("settings")
}
```

### 5. Adding a new feature — the 3-file starter

When adding a new feature, start with these 3 files before any implementation:

```kotlin
// 1. Domain model
// features/newfeature/domain/model/NewFeature.kt
data class NewFeatureItem(
    val id: String,
    val title: String,
)

// 2. Repository interface
// features/newfeature/domain/repository/NewFeatureRepository.kt
interface NewFeatureRepository {
    suspend fun getItems(): Result<List<NewFeatureItem>>
}

// 3. UI State
// features/newfeature/presentation/NewFeatureUiState.kt
data class NewFeatureUiState(
    val isLoading: Boolean = false,
    val items: List<NewFeatureItem> = emptyList(),
    val error: String? = null,
)
```

These three files establish the contract before anyone writes implementation code. They serve as a spec document that can be reviewed.

### 6. DI module per feature

```kotlin
// features/contacts/data/di/ContactsModule.kt
@Module
@InstallIn(SingletonComponent::class)
abstract class ContactsModule {
    @Binds @Singleton
    abstract fun bindContactsRepository(impl: ContactsRepositoryImpl): ContactsRepository
}

// Also provide the API service:
@Module
@InstallIn(SingletonComponent::class)
object ContactsNetworkModule {
    @Provides @Singleton
    fun provideContactsApi(retrofit: Retrofit): ContactsApiService =
        retrofit.create(ContactsApiService::class.java)
}
```

### 7. What NOT to put in core

```
✗ DON'T put these in core:
  - Chat data models → belongs in features/chat/domain/model/
  - Auth API service → belongs in features/auth/data/remote/
  - Message validation logic → belongs in features/chat/domain/
  - User profile screen → belongs in features/settings/presentation/

✓ DO put these in core:
  - MaterialTheme → used by every feature
  - OkHttpClient, Retrofit → used by every feature
  - SecureTokenStore → used by auth + WebSocket
  - ErrorMapper (JWT/network errors → user strings) → used by every feature
  - String.truncate(), Date.toRelativeTime() → used by every feature