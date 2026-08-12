# ECHO Android — Testing Strategy

## Testing Pyramid

```
         ┌─────────┐
         │  E2E    │  ← Manual QA / exploratory
         ├─────────┤
         │  UI     │  ← Compose UI smoke tests (key flows)
         ├─────────┤
         │  Unit   │  ← ViewModels + Use Cases (bulk of tests)
         └─────────┘
```

| Layer | Framework | Target | When |
|---|---|---|---|
| Unit (domain) | JUnit 4 + `kotlinx-coroutines-test` | Use cases, mappers, validators | On every push |
| Unit (presentation) | JUnit 4 + `StandardTestDispatcher` | ViewModels state transitions | On every push |
| UI smoke | Compose UI test (`createComposeRule`) | Key screens: Login, ChatList, ChatDetail | Before release |
| Integration | Manual / `@HiltAndroidTest` | Real API against dev server | Weekly / per milestone |

---

## Test Environment Setup

### Unit Tests (`app/src/test/`)

```kotlin
// Generic ViewModel test setup
@OptIn(ExperimentalCoroutinesApi::class)
abstract class ViewModelTest {
    protected val testDispatcher = StandardTestDispatcher()

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(testDispatcher)
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }
}
```

### Fake Repositories

Instead of mocking frameworks, use simple fakes:

```kotlin
class FakeAuthRepository : AuthRepository {
    private var loginResult: Result<User> = Result.failure(Exception("Not set"))
    private var savedToken: String? = null

    fun setLoginSuccess(user: User, token: String) {
        loginResult = Result.success(user)
        savedToken = token
    }

    fun setLoginError(message: String) {
        loginResult = Result.failure(Exception(message))
    }

    override suspend fun login(email: String, password: String): Result<User> = loginResult
    override suspend fun getStoredToken(): String? = savedToken
    override suspend fun logout() { savedToken = null }
}

class FakeChatRepository : ChatRepository {
    private var chats: List<Chat> = emptyList()
    private var messages: Map<String, List<Message>> = emptyMap()
    private var error: Throwable? = null

    fun setChats(chats: List<Chat>) { this.chats = chats }
    fun setMessages(chatId: String, msgs: List<Message>) { messages = messages + (chatId to msgs) }
    fun setError(error: Throwable) { this.error = error }

    override suspend fun getChats(): Result<List<Chat>> =
        error?.let { Result.failure(it) } ?: Result.success(chats)

    override suspend fun getMessages(chatId: String, page: Int): Result<List<Message>> =
        error?.let { Result.failure(it) } ?: Result.success(messages[chatId] ?: emptyList())

    override suspend fun createDirectChat(userId: String): Result<Chat> =
        error?.let { Result.failure(it) } ?: Result.success(TestData.chat1)
}
```

### TestData Factory

```kotlin
object TestData {
    val user = User(
        id = "507f1f77bcf86cd799439011",
        email = "test@echo.com",
        username = "testuser",
        displayName = "Test User",
        avatar = "",
        statusMessage = "Hello",
        bio = "Test bio",
        presence = Presence(PresenceStatus.ONLINE, true, null),
        isActive = true,
        isVerified = true,
        isBanned = false,
    )

    val chat1 = Chat(
        id = "6a775700f7feecc064dc54f6",
        type = ChatType.DIRECT,
        name = "Jane Doe",
        description = "",
        avatar = "",
        participants = listOf(
            Participant("user1", "testuser", "Test User", "", ChatRole.MEMBER, true, false, null),
            Participant("user2", "janedoe", "Jane Doe", "", ChatRole.MEMBER, true, false, null),
        ),
        ownerId = null,
        participantCount = 2,
        messageCount = 5,
        unreadCount = 0,
        lastMessage = null,
        settings = ChatSettings(isPrivate = false, allowInvites = true, allowFileSharing = true),
        createdAt = Instant.now(),
    )

    val message1 = Message(
        id = "msg_001",
        chatId = chat1.id,
        senderId = "user2",
        senderName = "janedoe",
        senderDisplayName = "Jane Doe",
        senderAvatar = "",
        content = "Hello!",
        type = MessageType.TEXT,
        attachments = emptyList(),
        reactions = emptyMap(),
        isEdited = false,
        readByUserIds = setOf("user1"),
        createdAt = Instant.now(),
    )

    val contact1 = Contact(
        id = "user2",
        username = "janedoe",
        displayName = "Jane Doe",
        avatar = "",
        presence = Presence(PresenceStatus.ONLINE, true, null),
        isFavorite = false,
    )
}
```

---

## Use Case Tests

### Auth Use Cases

```kotlin
class LoginUseCaseTest {
    private val fakeRepo = FakeAuthRepository()
    private val loginUseCase = LoginUseCase(fakeRepo)

    @Test
    fun `given valid credentials, when login, then returns user`() = runTest {
        // Given
        fakeRepo.setLoginSuccess(TestData.user, "fake-jwt-token")

        // When
        val result = loginUseCase("test@echo.com", "password123")

        // Then
        assertTrue(result.isSuccess)
        assertEquals("Test User", result.getOrNull()?.displayName)
    }

    @Test
    fun `given invalid credentials, when login, then returns failure`() = runTest {
        // Given
        fakeRepo.setLoginError("Invalid credentials")

        // When
        val result = loginUseCase("wrong@echo.com", "wrongpass")

        // Then
        assertTrue(result.isFailure)
        assertEquals("Invalid credentials", result.exceptionOrNull()?.message)
    }
}

class SignupUseCaseTest {
    private val fakeRepo = FakeAuthRepository()
    private val signupUseCase = SignupUseCase(fakeRepo)

    @Test
    fun `given valid signup data, when signup, then returns success`() = runTest {
        fakeRepo.setSignupSuccess()

        val result = signupUseCase("new@echo.com", "newuser", "password123")

        assertTrue(result.isSuccess)
    }
}
```

### Chat Use Cases

```kotlin
class GetChatListUseCaseTest {
    private val fakeRepo = FakeChatRepository()
    private val useCase = GetChatListUseCase(fakeRepo)

    @Test
    fun `given chats exist, when invoked, then returns chat list`() = runTest {
        fakeRepo.setChats(listOf(TestData.chat1))

        val result = useCase()

        assertTrue(result.isSuccess)
        assertEquals(1, result.getOrNull()?.size)
    }

    @Test
    fun `given network error, when invoked, then returns failure`() = runTest {
        fakeRepo.setError(RuntimeException("Network error"))

        val result = useCase()

        assertTrue(result.isFailure)
        assertEquals("Network error", result.exceptionOrNull()?.message)
    }
}

class GetChatMessagesUseCaseTest {
    private val fakeRepo = FakeChatRepository()
    private val useCase = GetChatMessagesUseCase(fakeRepo)

    @Test
    fun `given messages exist, when invoked, then returns ordered messages`() = runTest {
        fakeRepo.setMessages(TestData.chat1.id, listOf(TestData.message1))

        val result = useCase(TestData.chat1.id, page = 1)

        assertTrue(result.isSuccess)
        assertEquals(1, result.getOrNull()?.size)
    }

    @Test
    fun `given empty chat, when invoked, then returns empty list`() = runTest {
        fakeRepo.setMessages(TestData.chat1.id, emptyList())

        val result = useCase(TestData.chat1.id, page = 1)

        assertTrue(result.isSuccess)
        assertTrue(result.getOrNull()?.isEmpty() == true)
    }
}
```

### Contact Use Cases

```kotlin
class GetContactsUseCaseTest {
    private val fakeRepo = FakeContactsRepository()
    private val useCase = GetContactsUseCase(fakeRepo)

    @Test
    fun `given contacts exist, when invoked, then returns contact list`() = runTest {
        fakeRepo.setContacts(listOf(TestData.contact1))

        val result = useCase()

        assertTrue(result.isSuccess)
        assertEquals("janedoe", result.getOrNull()?.firstOrNull()?.username)
    }

    @Test
    fun `given no contacts, when invoked, then returns empty list`() = runTest {
        fakeRepo.setContacts(emptyList())

        val result = useCase()

        assertTrue(result.isSuccess)
        assertTrue(result.getOrNull()?.isEmpty() == true)
    }
}
```

---

## ViewModel Tests

### LoginViewModel

```kotlin
class LoginViewModelTest : ViewModelTest() {
    private val fakeRepo = FakeAuthRepository()
    private val fakeLogout = LogoutUseCase(fakeRepo)
    private lateinit var viewModel: LoginViewModel

    @BeforeEach
    fun setUpViewModel() {
        val loginUseCase = LoginUseCase(fakeRepo)
        viewModel = LoginViewModel(loginUseCase, fakeLogout)
    }

    @Test
    fun `given valid credentials, on login, state transitions to Authenticated`() = runTest {
        fakeRepo.setLoginSuccess(TestData.user, "token")
        assertEquals(LoginUiState(), viewModel.state.value) // Idle

        viewModel.onLogin("test@echo.com", "password123")
        advanceUntilIdle()

        assertEquals(false, viewModel.state.value.isLoading)
        assertEquals(null, viewModel.state.value.error)
    }

    @Test
    fun `given invalid credentials, on login, state contains error`() = runTest {
        fakeRepo.setLoginError("Invalid credentials")

        viewModel.onLogin("wrong@echo.com", "wrong")
        advanceUntilIdle()

        assertEquals(false, viewModel.state.value.isLoading)
        assertNotNull(viewModel.state.value.error)
    }

    @Test
    fun `given empty email, on login, state contains validation error`() = runTest {
        viewModel.onLogin("", "password123")
        // Should not call repository — validation fails early
        assertEquals("Please enter your email", viewModel.state.value.emailError)
    }

    @Test
    fun `given short password, on login, state contains validation error`() = runTest {
        viewModel.onLogin("test@echo.com", "123")
        assertEquals("Password must be at least 8 characters", viewModel.state.value.passwordError)
    }
}
```

### ChatListViewModel

```kotlin
class ChatListViewModelTest : ViewModelTest() {
    private val fakeRepo = FakeChatRepository()
    private lateinit var viewModel: ChatListViewModel

    @BeforeEach
    fun setUpViewModel() {
        viewModel = ChatListViewModel(GetChatListUseCase(fakeRepo))
    }

    @Test
    fun `given successful load, state contains chats`() = runTest {
        fakeRepo.setChats(listOf(TestData.chat1))

        viewModel.onEvent(ChatListEvent.Refresh)
        advanceUntilIdle()

        val state = viewModel.state.value
        assertFalse(state.isLoading)
        assertEquals(1, state.chats.size)
        assertNull(state.error)
    }

    @Test
    fun `given load failure, state contains error`() = runTest {
        fakeRepo.setError(RuntimeException("Network error"))

        viewModel.onEvent(ChatListEvent.Refresh)
        advanceUntilIdle()

        val state = viewModel.state.value
        assertFalse(state.isLoading)
        assertNotNull(state.error)
    }

    @Test
    fun `initial state shows loading`() {
        val state = viewModel.state.value
        assertTrue(state.isLoading)
        assertTrue(state.chats.isEmpty())
    }
}
```

### ChatDetailViewModel (WebSocket events)

```kotlin
class ChatDetailViewModelTest : ViewModelTest() {
    private val fakeWsRepo = FakeWebSocketRepository()
    private val fakeChatRepo = FakeChatRepository()

    @Test
    fun `given NewMessage event, message appended to state`() = runTest {
        fakeChatRepo.setMessages("chat1", emptyList())
        val viewModel = ChatDetailViewModel("chat1", fakeWsRepo, fakeChatRepo)

        fakeWsRepo.emitEvent(DomainEvent.NewMessage("chat1", TestData.message1))
        advanceUntilIdle()

        val state = viewModel.state.value
        assertEquals(1, state.messages.size)
        assertEquals("Hello!", state.messages.first().content)
    }

    @Test
    fun `given TypingUpdate, typing state updated`() = runTest {
        fakeChatRepo.setMessages("chat1", emptyList())
        val viewModel = ChatDetailViewModel("chat1", fakeWsRepo, fakeChatRepo)

        fakeWsRepo.emitEvent(DomainEvent.TypingUpdate("chat1", "user2", true))
        advanceUntilIdle()

        assertTrue(viewModel.state.value.typingUsers.contains("user2"))
    }
}
```

---

## Compose UI Tests

### Login Screen Smoke Test

```kotlin
@RunWith(AndroidJUnit4::class)
class LoginScreenTest {
    @get:Rule val composeTestRule = createComposeRule()

    @Test
    fun `given empty fields, login button is disabled`() {
        composeTestRule.setContent {
            EchoTheme {
                LoginContent(
                    email = "",
                    password = "",
                    isLoading = false,
                    error = null,
                    emailError = null,
                    passwordError = null,
                    onEmailChange = {},
                    onPasswordChange = {},
                    onLogin = {},
                    onSignupClick = {},
                )
            }
        }

        composeTestRule
            .onNodeWithTag("login_button")
            .assertIsNotEnabled()
    }

    @Test
    fun `given error state, error message is displayed`() {
        composeTestRule.setContent {
            EchoTheme {
                LoginContent(
                    email = "test@echo.com",
                    password = "password123",
                    isLoading = false,
                    error = "Invalid credentials",
                    emailError = null,
                    passwordError = null,
                    onEmailChange = {},
                    onPasswordChange = {},
                    onLogin = {},
                    onSignupClick = {},
                )
            }
        }

        composeTestRule
            .onNodeWithText("Invalid credentials")
            .assertIsDisplayed()
    }

    @Test
    fun `given loading state, loading indicator is displayed`() {
        composeTestRule.setContent {
            EchoTheme {
                LoginContent(
                    email = "test@echo.com",
                    password = "password123",
                    isLoading = true,
                    error = null,
                    emailError = null,
                    passwordError = null,
                    onEmailChange = {},
                    onPasswordChange = {},
                    onLogin = {},
                    onSignupClick = {},
                )
            }
        }

        composeTestRule
            .onNodeWithTag("loading_indicator")
            .assertIsDisplayed()
    }
}
```

### ChatList Screen Smoke Test

```kotlin
@RunWith(AndroidJUnit4::class)
class ChatListScreenTest {
    @get:Rule val composeTestRule = createComposeRule()

    @Test
    fun `given chats, renders chat items`() {
        composeTestRule.setContent {
            EchoTheme {
                ChatListContent(
                    chats = listOf(TestData.chat1),
                    isLoading = false,
                    error = null,
                    onRefresh = {},
                    onChatClicked = {},
                    onCreateGroup = {},
                )
            }
        }

        composeTestRule
            .onNodeWithText("Jane Doe")
            .assertIsDisplayed()
    }

    @Test
    fun `given empty chats, shows empty state`() {
        composeTestRule.setContent {
            EchoTheme {
                ChatListContent(
                    chats = emptyList(),
                    isLoading = false,
                    error = null,
                    onRefresh = {},
                    onChatClicked = {},
                    onCreateGroup = {},
                )
            }
        }

        composeTestRule
            .onNodeWithTag("empty_state")
            .assertIsDisplayed()
    }

    @Test
    fun `given error state, shows error with retry`() {
        var retryClicked = false
        composeTestRule.setContent {
            EchoTheme {
                ChatListContent(
                    chats = emptyList(),
                    isLoading = false,
                    error = "Network error",
                    onRefresh = { retryClicked = true },
                    onChatClicked = {},
                    onCreateGroup = {},
                )
            }
        }

        composeTestRule
            .onNodeWithText("Network error")
            .assertIsDisplayed()

        composeTestRule
            .onNodeWithTag("retry_button")
            .performClick()

        assertTrue(retryClicked)
    }
}
```

---

## Test Coverage Goals

| Layer | Minimum Coverage | What to cover |
|---|---|---|
| Use Cases | 100% | Every use case: success + failure path |
| ViewModels | 80%+ | All public methods, state transitions, event handling |
| Mappers (DTO → Domain) | 100% | Every `toDomain()` function |
| Validators | 100% | `isValidEmail`, `validateMessage`, etc. |
| Composable screens | Smoke (key flows) | Login, ChatList, ChatDetail, Contacts |
| Repository impls | Integration only | Tested manually or via integration tests |

---

## What NOT to Test

| Component | Why |
|---|---|
| DI wiring | Hilt validates at compile time |
| Navigation routes | Compile-time sealed class guarantees |
| Retrofit interfaces | No logic — just annotations |
| Timber logging calls | Side effects, not behavior |
| OkHttp interceptors | Tested via integration |
| Exact pixel positions | Fragile — test behavior, not layout |
| Stub endpoints (`/suggestions`, `/nearby`, `/popular`, WS NOT_IMPLEMENTED) | Backend stubs excluded from app scope |

---

## Test Naming Convention

```
given_{precondition}_when_{action}_then_{expectedResult}
```

Examples:
- `given_valid_credentials_when_login_then_returns_user`
- `given_network_error_when_loadChats_then_state_contains_error`
- `given_empty_message_when_send_then_validation_error`

---

## Running Tests

```bash
# Unit tests only
./gradlew testDebug

# Unit tests for specific module
./gradlew :app:testDebugUnitTest --tests "com.shoonya.echo.features.auth.LoginUseCaseTest"

# UI tests (requires emulator/device)
./gradlew connectedAndroidTest

# All tests (unit + UI)
./gradlew testDebug connectedAndroidTest
```

---

## CI Integration (Recommended)

```yaml
# .github/workflows/android.yml
name: Android CI
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-java@v4
        with:
          distribution: 'temurin'
          java-version: '17'
      - run: ./gradlew testDebug
      - run: ./gradlew lintDebug
```

---

## Crash Loop Prevention

```kotlin
// core/presentation/SafeScreen.kt
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

@Composable
fun FallbackErrorScreen(message: String, onRetry: () -> Unit) {
    Column(
        modifier = Modifier.fillMaxSize(),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center,
    ) {
        Text(message, style = MaterialTheme.typography.bodyLarge)
        Spacer(modifier = Modifier.height(16.dp))
        Button(onClick = onRetry) { Text("Try Again") }
    }
}