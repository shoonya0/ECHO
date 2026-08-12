# ECHO Android — API Integration

## Backend Overview

- **Base URL:** `{host}/echo/v1/`
- **Auth:** JWT Bearer token (from `POST /login`) in `Authorization` header
- **Content-Type:** `application/json`
- **Response Envelope:** `{ success: Boolean, message: String?, data: T?, error: String? }`
- **Pagination Envelope:** `{ items: [...], pagination: { limit, total, total_pages } }`
- **Message Pagination:** `{ chat: String, messages: [...], pagination: { limit, offset, count, totalCount, page, totalPages } }`

## All Endpoints

### Authentication (no auth required)

| Method | Path | Request Body | Response Data | Notes |
|---|---|---|---|---|
| `POST` | `login` | `{ email, password }` | `{ token, user: LoginUserResponse }` | Returns JWT |
| `POST` | `signup` | `{ email, username, password }` | User object | **No JWT returned** — must redirect to Login |

### Profile (auth required)

| Method | Path | Body/Params | Response |
|---|---|---|---|
| `GET` | `profile/` | — | `GetProfileResponse` |
| `PUT` | `profile/` | `UpdateUserRequest` | Updated user |
| `DELETE` | `profile/delete` | — | `{ message }` |

### User Profile (auth required)

| Method | Path | Response |
|---|---|---|
| `GET` | `users/:id` | `GetUserProfileResponse` |

**Note:** `GET /users/suggestions`, `/users/nearby`, and `/users/popular` are backend stubs (return `null` data) and are **not implemented** in the Android app.

### Contacts (auth required)

| Method | Path | Action |
|---|---|---|
| `GET` | `users/contacts/` | List accepted contacts (paginated) |
| `GET` | `users/contacts/requests` | Incoming pending requests |
| `GET` | `users/contacts/sent-requests` | Outgoing pending requests |
| `GET` | `users/contacts/blocked` | Blocked users |
| `GET` | `users/contacts/favorites` | Favorite contacts |
| `POST` | `users/contacts/:targetUserId` | Send contact request |
| `PUT` | `users/contacts/:requestId?action=accepted\|declined` | Accept/decline |
| `DELETE` | `users/contacts/:contactId` | Remove contact |
| `POST` | `users/contacts/blockUnblock/:userId?action=block\|unblock` | Block/unblock |
| `POST` | `users/contacts/favorite/:userId` | Add to favorites |
| `DELETE` | `users/contacts/favorite/:userId` | Remove from favorites |

### Chat (auth required)

| Method | Path | Body | Response |
|---|---|---|---|
| `POST` | `chats/direct/:userId` | — | `ChatDto` |
| `POST` | `chats/group` | `{ name, description, participants: [userIds] }` | `ChatDto` |
| `GET` | `chats/hub/stats` | — | Hub statistics (monitoring) |

### Messages (auth required)

| Method | Path | Query | Response |
|---|---|---|---|
| `GET` | `messages/:chatID/messages` | `?limit=10&offset=0` | `MessageListResponse` |

### Groups (auth required)

| Method | Path | Body/Query | Response |
|---|---|---|---|
| `POST` | `groups/:groupID/add-members` | `{ userIDs: [ids] }` | `{ message }` |
| `POST` | `groups/:groupID/invites` | — | `{ message }` (creates invite) |
| `GET` | `groups/:groupID/invites` | — | `[InviteDto]` |
| `GET` | `groups/invites` | — | `[InviteDto]` (all user's invites) |
| `DELETE` | `groups/invites/:inviteID/` | — | `{ message }` |
| `GET` | `groups/invites/:inviteID/joined` | — | `[JoinedUserDto]` |
| `POST` | `groups/invites/:inviteID/:userID/send` | — | `{ message }` |
| `POST` | `groups/join/:inviteCode` | — | `{ message }` |

---

## Retrofit Service Interfaces

### AuthApiService

```kotlin
// features/auth/data/remote/AuthApiService.kt
interface AuthApiService {
    @POST("login")
    suspend fun login(@Body request: LoginRequest): ApiResponse<AuthResponse>

    @POST("signup")
    suspend fun signup(@Body request: SignupRequest): ApiResponse<LoginUserDto>
}
```

### ProfileApiService

```kotlin
// features/settings/data/remote/ProfileApiService.kt
interface ProfileApiService {
    @GET("profile/")
    suspend fun getProfile(): ApiResponse<ProfileDto>

    @PUT("profile/")
    suspend fun updateProfile(@Body request: UpdateProfileRequest): ApiResponse<ProfileDto>

    @DELETE("profile/delete")
    suspend fun deleteAccount(): ApiResponse<Unit>
}
```

### UsersApiService

```kotlin
// features/contacts/data/remote/UsersApiService.kt
interface UsersApiService {
    @GET("users/{id}")
    suspend fun getUserProfile(@Path("id") userId: String): ApiResponse<UserProfileDto>

    // Note: getUserSuggestions / getNearbyUsers / getPopularUsers are deliberately
    // excluded — these backend endpoints are stubs that return null data.
}
```

### ContactsApiService

```kotlin
// features/contacts/data/remote/ContactsApiService.kt
interface ContactsApiService {
    @GET("users/contacts/")
    suspend fun getContacts(@Query("limit") limit: Int = 50): ApiResponse<PaginatedData<ContactDto>>

    @GET("users/contacts/requests")
    suspend fun getContactRequests(@Query("limit") limit: Int = 50): ApiResponse<PaginatedData<ContactDto>>

    @GET("users/contacts/sent-requests")
    suspend fun getSentContactRequests(@Query("limit") limit: Int = 50): ApiResponse<PaginatedData<ContactDto>>

    @GET("users/contacts/blocked")
    suspend fun getBlockedUsers(@Query("limit") limit: Int = 50): ApiResponse<PaginatedData<ContactDto>>

    @GET("users/contacts/favorites")
    suspend fun getFavorites(@Query("limit") limit: Int = 50): ApiResponse<PaginatedData<ContactDto>>

    @POST("users/contacts/{userId}")
    suspend fun sendContactRequest(@Path("userId") userId: String): ApiResponse<Unit>

    @PUT("users/contacts/{requestId}")
    suspend fun acceptOrDeclineRequest(
        @Path("requestId") requestId: String,
        @Query("action") action: String,   // "accepted" or "declined"
    ): ApiResponse<Unit>

    @DELETE("users/contacts/{contactId}")
    suspend fun removeContact(@Path("contactId") contactId: String): ApiResponse<Unit>

    @POST("users/contacts/blockUnblock/{userId}")
    suspend fun blockUnblockUser(
        @Path("userId") userId: String,
        @Query("action") action: String = "block",
    ): ApiResponse<Unit>

    @POST("users/contacts/favorite/{userId}")
    suspend fun addToFavorites(@Path("userId") userId: String): ApiResponse<Unit>

    @DELETE("users/contacts/favorite/{userId}")
    suspend fun removeFromFavorites(@Path("userId") userId: String): ApiResponse<Unit>
}
```

### ChatApiService

```kotlin
// features/chat/data/remote/ChatApiService.kt
interface ChatApiService {
    @POST("chats/direct/{userId}")
    suspend fun createDirectChat(@Path("userId") userId: String): ApiResponse<ChatDto>

    @POST("chats/group")
    suspend fun createGroupChat(@Body request: CreateGroupRequest): ApiResponse<ChatDto>

    @GET("chats/hub/stats")
    suspend fun getHubStats(): ApiResponse<HubStatsDto>
}
```

### MessageApiService

```kotlin
// features/chat/data/remote/MessageApiService.kt
interface MessageApiService {
    @GET("messages/{chatId}/messages")
    suspend fun getChatMessages(
        @Path("chatId") chatId: String,
        @Query("limit") limit: Int = 50,
        @Query("offset") offset: Int = 0,
    ): ApiResponse<MessageListResponse>
}
```

### GroupApiService

```kotlin
// features/groups/data/remote/GroupApiService.kt
interface GroupApiService {
    @POST("groups/{groupId}/add-members")
    suspend fun addGroupMembers(
        @Path("groupId") groupId: String,
        @Body request: AddMembersRequest,
    ): ApiResponse<Unit>

    @POST("groups/{groupId}/invites")
    suspend fun createInvite(@Path("groupId") groupId: String): ApiResponse<Unit>

    @GET("groups/{groupId}/invites")
    suspend fun getGroupInvites(@Path("groupId") groupId: String): ApiResponse<List<InviteDto>>

    @GET("groups/invites")
    suspend fun getAllUserInvites(): ApiResponse<List<InviteDto>>

    @DELETE("groups/invites/{inviteId}/")
    suspend fun deleteInvite(@Path("inviteId") inviteId: String): ApiResponse<Unit>

    @GET("groups/invites/{inviteId}/joined")
    suspend fun getJoinedUsersByInvite(@Path("inviteId") inviteId: String): ApiResponse<List<JoinedUserDto>>

    @POST("groups/invites/{inviteId}/{userId}/send")
    suspend fun sendInviteToUser(
        @Path("inviteId") inviteId: String,
        @Path("userId") userId: String,
    ): ApiResponse<Unit>

    @POST("groups/join/{inviteCode}")
    suspend fun joinGroupByInvite(@Path("inviteCode") inviteCode: String): ApiResponse<Unit>
}
```

---

## Network Configuration

### BuildConfig Fields

```kotlin
// androidbuild.gradle.kts
buildTypes {
    debug {
        buildConfigField("String", "API_BASE_URL", "\"http://10.0.2.2:8080\"")
        buildConfigField("String", "WS_BASE_URL", "\"ws://10.0.2.2:8080\"")
        buildConfigField("boolean", "ENABLE_LOGGING", "true")
    }
    release {
        buildConfigField("String", "API_BASE_URL", "\"https://echo.example.com\"")
        buildConfigField("String", "WS_BASE_URL", "\"wss://echo.example.com\"")
        buildConfigField("boolean", "ENABLE_LOGGING", "false")
    }
}
```

### OkHttpClient Configuration

```kotlin
// core/data/remote/NetworkModule.kt
@Module
@InstallIn(SingletonComponent::class)
object NetworkModule {

    @Provides @Singleton
    fun provideJson(): Json = Json {
        ignoreUnknownKeys = true
        coerceInputValues = true
        isLenient = false
        encodeDefaults = true
    }

    @Provides @Singleton @RestClient
    fun provideRestOkHttpClient(
        tokenStore: SecureTokenStore,
        loggingEnabled: Boolean,
    ): OkHttpClient {
        return OkHttpClient.Builder()
            .connectTimeout(10, TimeUnit.SECONDS)
            .readTimeout(30, TimeUnit.SECONDS)
            .writeTimeout(30, TimeUnit.SECONDS)
            .addInterceptor(AuthInterceptor(tokenStore))
            .apply {
                if (loggingEnabled) {
                    addInterceptor(HttpLoggingInterceptor().apply {
                        level = Level.BODY
                    })
                }
            }
            .build()
    }

    @Provides @Singleton @WsClient
    fun provideWsOkHttpClient(): OkHttpClient {
        return OkHttpClient.Builder()
            .pingInterval(54, TimeUnit.SECONDS)  // Aligns with server keepalive
            .readTimeout(0, TimeUnit.MILLISECONDS)  // No read timeout for WebSocket
            .connectTimeout(10, TimeUnit.SECONDS)
            .build()
    }

    @Provides @Singleton
    fun provideRetrofit(
        @RestClient okHttpClient: OkHttpClient,
        json: Json,
    ): Retrofit {
        return Retrofit.Builder()
            .baseUrl(BuildConfig.API_BASE_URL + "/echo/v1/")
            .client(okHttpClient)
            .addConverterFactory(json.asConverterFactory("application/json".toMediaType()))
            .build()
    }
}
```

### Qualifier Annotations

```kotlin
@Qualifier @Retention(AnnotationRetention.BINARY) annotation class RestClient
@Qualifier @Retention(AnnotationRetention.BINARY) annotation class WsClient
```

---

## Error Handling

### ApiException

```kotlin
// core/data/remote/ApiException.kt
class ApiException(
    message: String,
    val code: Int = 0,
) : Exception(message)
```

### Backend Error Codes → User Messages

```kotlin
// core/util/ErrorMapper.kt
object ErrorMapper {
    fun mapBackendError(code: String, defaultMessage: String): String = when (code) {
        "INVALID_REQUEST" -> "Invalid request. Please try again."
        "PERMISSION_DENIED" -> "You don't have permission to do that."
        "INVALID_CHAT_ID" -> "Chat not found."
        "INVALID_MESSAGE_ID" -> "Message not found."
        "INVALID_CONTENT" -> "Message contains invalid content."
        "MESSAGE_FAILED" -> "Failed to send message. Tap to retry."
        "REACTION_FAILED" -> "Failed to add reaction."
        "READ_FAILED" -> "Failed to mark as read."
        "PARSE_ERROR" -> "Server couldn't process the request."
        "RATE_LIMIT_EXCEEDED" -> "Too many requests. Slow down."
        "CHANNEL_FULL" -> "Server is busy. Please wait."
        "TOKEN_VERIFICATION_FAILED" -> "Session expired. Please log in again."
        "USER_NOT_AUTHENTICATED" -> "Please log in to continue."
        "NOT_IMPLEMENTED" -> "This feature is coming soon."
        "UNKNOWN_REQUEST" -> "Unknown request type."
        else -> defaultMessage
    }

    fun Exception.toUserMessage(): String = when {
        this is java.net.ConnectException -> "No internet connection. Check your network."
        this is java.net.SocketTimeoutException -> "Server is taking too long. Try again."
        this is java.net.UnknownHostException -> "Cannot reach server. Check your connection."
        this is ApiException -> this.message ?: "Something went wrong."
        else -> this.message ?: "Something went wrong. Please try again."
    }
}
```

### Repository Error Pattern

```kotlin
// Every repository returns Result<T> and maps errors
class AuthRepositoryImpl @Inject constructor(
    private val api: AuthApiService,
    private val tokenStore: SecureTokenStore,
) : AuthRepository {
    override suspend fun login(email: String, password: String): Result<User> {
        return runCatching {
            api.login(LoginRequest(email, password))
                .toResult()
                .getOrThrow()
        }.mapCatching { response ->
            tokenStore.saveAccessToken(response.token)
            response.user.toDomain()
        }
    }
}
```

### ViewModel Error Handling

```kotlin
// ViewModel catches Result and maps to UiState
fun login(email: String, password: String) {
    viewModelScope.launch {
        _state.update { it.copy(isLoading = true, error = null) }
        loginUseCase(email, password)
            .onSuccess { user ->
                _state.update { it.copy(isLoading = false, isAuthenticated = true) }
                _events.emit(AuthEvent.NavigateToHome)
            }
            .onFailure { err ->
                Timber.tag("LoginVM").e(err, "login failed")
                _state.update {
                    it.copy(
                        isLoading = false,
                        error = err.toUserMessage()
                    )
                }
            }
    }
}
```

---

## BuildConfig Access

```kotlin
// In any feature module, use:
object ApiConfig {
    val baseUrl get() = BuildConfig.API_BASE_URL
    val wsBaseUrl get() = BuildConfig.WS_BASE_URL
    val isLoggingEnabled get() = BuildConfig.ENABLE_LOGGING
}