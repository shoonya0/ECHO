# Testing — ECHO Android

## Rule

1. **Unit tests for use cases and ViewModels** using JUnit + coroutines test dispatcher. No device/emulator needed.
2. **Compose UI tests** for critical flows (login, send message). Minimal — test behavior, not pixel-perfect layout.
3. **Fake repositories** for unit tests — no mock framework needed. `FakeChatRepository` returns canned data.
4. **Tests are deterministic.** No `System.currentTimeMillis()`, no `Random`, no `Thread.sleep`. Use `TestDispatcher.advanceTimeBy`.
5. **Given-When-Then** structure for readability. Every test method name describes the scenario.

## Why

- A chat app has complex async state (messages arriving, typing indicators, reconnection). Tests catch regressions before users do.
- Fake repositories over mocking frameworks make tests readable and maintainable — they look like documentation.
- Compose UI tests verify the full stack (ViewModel → Composable → user interaction) in one flow.

## How to apply

### 1. Use case unit test

```kotlin
class GetChatListUseCaseTest {
    private val fakeRepo = FakeChatRepository()
    private val useCase = GetChatListUseCase(fakeRepo)

    @Test
    fun `given chats exist, when invoked, then returns chat list`() = runTest {
        // Given
        fakeRepo.setChats(listOf(TestData.chat1, TestData.chat2))

        // When
        val result = useCase()

        // Then
        assertTrue(result.isSuccess)
        assertEquals(2, result.getOrNull()?.size)
    }

    @Test
    fun `given network error, when invoked, then returns failure`() = runTest {
        // Given
        fakeRepo.setError(RuntimeException("Network error"))

        // When
        val result = useCase()

        // Then
        assertTrue(result.isFailure)
        assertEquals("Network error", result.exceptionOrNull()?.message)
    }
}
```

### 2. Fake repository

```kotlin
// test/fakes/FakeChatRepository.kt
class FakeChatRepository : ChatRepository {
    private var chats: List<Chat> = emptyList()
    private var error: Throwable? = null

    fun setChats(chats: List<Chat>) { this.chats = chats }
    fun setError(error: Throwable) { this.error = error }

    override suspend fun getChats(): Result<List<Chat>> {
        error?.let { return Result.failure(it) }
        return Result.success(chats)
    }

    override suspend fun getMessages(chatId: String, page: Int): Result<List<Message>> {
        error?.let { return Result.failure(it) }
        return Result.success(emptyList())
    }
}
```

### 3. ViewModel test with TestDispatcher

```kotlin
@OptIn(ExperimentalCoroutinesApi::class)
class ChatListViewModelTest {
    private val testDispatcher = StandardTestDispatcher()
    private val fakeRepo = FakeChatRepository()
    private val fakeGetChats = GetChatListUseCase(fakeRepo)

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(testDispatcher)
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun `given successful load, state contains chats`() = runTest {
        // Given
        fakeRepo.setChats(listOf(TestData.chat1))
        val viewModel = ChatListViewModel(fakeGetChats)

        // When
        viewModel.loadChats()
        advanceUntilIdle()  // Process all coroutines

        // Then
        val state = viewModel.state.value
        assertFalse(state.isLoading)
        assertEquals(1, state.chats.size)
        assertNull(state.error)
    }

    @Test
    fun `given load failure, state contains error message`() = runTest {
        // Given
        fakeRepo.setError(RuntimeException("Network error"))
        val viewModel = ChatListViewModel(fakeGetChats)

        // When
        viewModel.loadChats()
        advanceUntilIdle()

        // Then
        val state = viewModel.state.value
        assertFalse(state.isLoading)
        assertNotNull(state.error)
    }
}
```

### 4. Compose UI test

```kotlin
@RunWith(AndroidJUnit4::class)
class ChatListScreenTest {
    @get:Rule val composeTestRule = createComposeRule()

    @Test
    fun `given loading state, shows loading indicator`() {
        composeTestRule.setContent {
            EchoTheme {
                ChatListContent(
                    chats = emptyList(),
                    isLoading = true,
                    error = null,
                    onRefresh = {},
                    onChatClicked = {}
                )
            }
        }

        composeTestRule
            .onNodeWithTag("loading_indicator")
            .assertIsDisplayed()
    }

    @Test
    fun `given error state, shows error message with retry`() {
        var retryClicked = false
        composeTestRule.setContent {
            EchoTheme {
                ChatListContent(
                    chats = emptyList(),
                    isLoading = false,
                    error = "Something went wrong",
                    onRefresh = { retryClicked = true },
                    onChatClicked = {}
                )
            }
        }

        composeTestRule
            .onNodeWithText("Something went wrong")
            .assertIsDisplayed()

        composeTestRule
            .onNodeWithTag("retry_button")
            .performClick()

        assertTrue(retryClicked)
    }
}
```

### 5. Test data factory

```kotlin
// test/TestData.kt
object TestData {
    val chat1 = Chat(
        id = "507f1f77bcf86cd799439011",
        name = "Alice",
        lastMessage = "Hello!",
        unreadCount = 2,
    )
}
```

### 6. Determinism rules

```kotlin
// ✓ GOOD — use test dispatcher to control time
advanceTimeBy(5000)  // Jump 5 seconds

// ✗ BAD — real sleep makes tests slow and flaky
Thread.sleep(5000)

// ✓ GOOD — fixed IDs
val chatId = "test-chat-1"

// ✗ BAD — random IDs change every run
val chatId = UUID.randomUUID().toString()
```

### 7. What to test vs what not to test

| Test | Priority |
| --- | --- |
| Use cases (business logic) | **Must test** — every use case |
| ViewModels (state transitions) | **Must test** — every public method |
| Repository implementations | **Integration test** (against real API/DB) |
| DTO mapping (toDomain()) | **Unit test** — pure, fast |
| Composable rendering | **Smoke test** — key screens only |
| Navigation | **Skip** — tested by manual QA |
| DI wiring | **Skip** — Hilt validates at compile time |

### 8. Integration tests

```kotlin
// Run against a real server instance
@HiltAndroidTest
class AuthIntegrationTest {
    @get:Rule val hiltRule = HiltAndroidRule(this)

    @Test
    fun `login with valid credentials returns token`() = runTest {
        val api = Retrofit.Builder()
            .baseUrl("http://10.0.2.2:8080/echo/v1/")
            .client(OkHttpClient())
            .addConverterFactory(Json.asConverterFactory("application/json".toMediaType()))
            .build()
            .create(AuthApiService::class.java)

        val result = api.login(LoginRequest("test@echo.com", "password123"))
        assertTrue(result.success)
        assertNotNull(result.data?.token)
    }
}