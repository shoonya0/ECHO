# Dependency Injection — ECHO Android

## Rule

1. **Hilt** is the DI framework. `@HiltAndroidApp` on Application, `@HiltViewModel` on ViewModels, `@Inject constructor` on everything else.
2. **Modules are organized by layer:** `core/di/` for global singletons (OkHttpClient, Retrofit, SecureTokenStore), `features/*/data/di/` for feature-specific bindings.
3. **`@Singleton` for global scope**, `@ViewModelScoped` for per-screen dependencies (rare), default (unscoped) for stateless dependencies.
4. **Interfaces are bound to implementations** via `@Binds` in abstract modules. Concrete dependencies via `@Provides`.
5. **Never use `ServiceLoader`, manual factories, or `object` singletons for injectable dependencies.**

## Why

- Hilt integrates with ViewModel, Compose, and Android lifecycle — it handles lifecycle-aware instantiation and cleanup automatically.
- Scoped singletons prevent multiple OkHttpClient/Retrofit instances which would leak connections.
- `@Binds` modules make it trivial to swap implementations for tests (just replace the module).

## How to apply

### 1. Application setup

```kotlin
@HiltAndroidApp
class EchoApplication : Application() {
    override fun onCreate() {
        super.onCreate()
        if (BuildConfig.DEBUG) {
            StrictMode.enableDefaults()
        }
        Timber.plant(Timber.DebugTree())
    }
}
```

### 2. Core module — global singletons

```kotlin
// core/di/CoreModule.kt
@Module
@InstallIn(SingletonComponent::class)
object CoreModule {

    @Provides @Singleton
    fun provideSecureTokenStore(@ApplicationContext context: Context): SecureTokenStore =
        SecureTokenStore(context)
}
```

### 3. Interface binding with `@Binds`

```kotlin
// features/chat/domain/repository/ChatRepository.kt
interface ChatRepository {
    suspend fun getChats(): Result<List<Chat>>
    suspend fun getMessages(chatId: String, page: Int): Result<List<Message>>
}
```

```kotlin
// features/chat/data/repository/ChatRepositoryImpl.kt
class ChatRepositoryImpl @Inject constructor(
    private val api: ChatApiService,
    private val wsRepo: WebSocketRepository
) : ChatRepository { ... }
```

```kotlin
// features/chat/data/di/ChatModule.kt
@Module
@InstallIn(SingletonComponent::class)
abstract class ChatModule {

    @Binds @Singleton
    abstract fun bindChatRepository(impl: ChatRepositoryImpl): ChatRepository
}
```

### 4. ViewModel injection

```kotlin
// ✓ GOOD — Hilt injects the ViewModel via @HiltViewModel
@HiltViewModel
class ChatListViewModel @Inject constructor(
    private val getChats: GetChatListUseCase,
    private val logout: LogoutUseCase,
) : ViewModel() {
    // ViewModel-scoped lifecycle, constructor injection
}

// In composable:
@Composable
fun ChatListScreen(
    viewModel: ChatListViewModel = hiltViewModel(),
    onChatClicked: (String) -> Unit
) {
    // ...
}

// ✗ BAD — manual ViewModel factory
class ChatListViewModel(
    private val getChats: GetChatListUseCase,
) : ViewModel() {
    class Factory(private val getChats: GetChatListUseCase) : ViewModelProvider.Factory { ... }
}
```

### 5. Use case injection

```kotlin
// features/chat/domain/usecase/GetChatListUseCase.kt
class GetChatListUseCase @Inject constructor(
    private val repo: ChatRepository
) {
    suspend operator fun invoke(): Result<List<Chat>> = repo.getChats()
}
```

### 6. Scoping rules

| Scope | When to use |
| --- | --- |
| `@Singleton` | OkHttpClient, Retrofit, SecureTokenStore, WebSocket client, JSON instance — exactly one per app |
| `@ViewModelScoped` | Caching a state holder tied to one screen's lifecycle — rare, prefer ViewModel |
| Unscoped (default) | Stateless dependencies (use cases, mappers, converters) — recreated each time |

### 7. Qualifiers for multiple implementations

```kotlin
// When you need two OkHttpClients (one for WebSocket, one for REST):
@Qualifier @Retention(AnnotationRetention.BINARY) annotation class RestClient
@Qualifier @Retention(AnnotationRetention.BINARY) annotation class WsClient

@Module @InstallIn(SingletonComponent::class)
object NetworkModule {
    @Provides @Singleton @RestClient
    fun provideRestOkHttpClient(tokenStore: SecureTokenStore): OkHttpClient { ... }

    @Provides @Singleton @WsClient
    fun provideWsOkHttpClient(): OkHttpClient { ... }
}
```

### 8. Test doubles

```kotlin
// In tests, replace modules:
@HiltAndroidTest
@UninstallModules(ChatModule::class)
class ChatListViewModelTest {
    @Module @InstallIn(SingletonComponent::class)
    abstract class FakeChatModule {
        @Binds @Singleton
        abstract fun bindChatRepository(fake: FakeChatRepository): ChatRepository
    }
    // FakeChatRepository returns mock data
}
```

### 9. No service locator pattern

```kotlin
// ✗ BAD — manual dependency graph
object ServiceLocator {
    val chatRepo: ChatRepository = ChatRepositoryImpl(...)
}

// ✓ GOOD — let Hilt wire it
@Inject lateinit var chatRepo: ChatRepository  // Only in entry points
```

### 10. Entry points (for non-Hilt-managed classes)

```kotlin
// If a class can't use constructor injection (e.g., Activity not yet Hilt-managed):
@EntryPoint @InstallIn(SingletonComponent::class)
interface EchoEntryPoint {
    fun secureTokenStore(): SecureTokenStore
}

// Usage:
val entryPoint = EntryPointAccessors.fromApplication(context, EchoEntryPoint::class.java)
val tokenStore = entryPoint.secureTokenStore()