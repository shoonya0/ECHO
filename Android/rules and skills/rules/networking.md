# Networking — ECHO Android

## Rule

1. **Retrofit + OkHttp** for REST calls (`/echo/v1/auth/*`, `/echo/v1/chats/*`, `/echo/v1/messages/*`). OkHttp singleton for WebSocket.
2. **kotlinx.serialization** for JSON — not Gson, not Moshi.
3. **Auth interceptor** on OkHttpClient injects the JWT Bearer token from `SecureTokenStore`.
4. **Base URL** from `BuildConfig.API_BASE_URL` — no hardcoded URLs.
5. **Timeout:** connect 10s, read 30s, write 30s. WebSocket: read=0 (infinite).

## Why

- The backend (`_docs/README.md`) exposes REST endpoints for auth, chat management, and message history. These need a typed Retrofit interface matching the server's response envelope.
- `kotlinx.serialization` is the Kotlin-native serialization library — zero reflection, compile-time safety, and integrates with multiplatform.
- An auth interceptor ensures every request carries a valid token without duplicating token-retrieval code in every API call.

## How to apply

### 1. Retrofit setup

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

    @Provides @Singleton
    fun provideOkHttpClient(tokenStore: SecureTokenStore): OkHttpClient {
        return OkHttpClient.Builder()
            .connectTimeout(10, TimeUnit.SECONDS)
            .readTimeout(30, TimeUnit.SECONDS)
            .writeTimeout(30, TimeUnit.SECONDS)
            .addInterceptor(AuthInterceptor(tokenStore))
            .addInterceptor(HttpLoggingInterceptor().apply {
                level = if (BuildConfig.DEBUG) Level.BODY else Level.NONE
            })
            .build()
    }

    @Provides @Singleton
    fun provideRetrofit(okHttpClient: OkHttpClient, json: Json): Retrofit {
        return Retrofit.Builder()
            .baseUrl(BuildConfig.API_BASE_URL + "/echo/v1/")
            .client(okHttpClient)
            .addConverterFactory(json.asConverterFactory("application/json".toMediaType()))
            .build()
    }
}
```

### 2. Auth interceptor

```kotlin
// core/data/remote/AuthInterceptor.kt
class AuthInterceptor @Inject constructor(
    private val tokenStore: SecureTokenStore
) : Interceptor {
    override fun intercept(chain: Interceptor.Chain): Response {
        val token = tokenStore.getAccessToken()
        val request = if (token != null) {
            chain.request().newBuilder()
                .header("Authorization", "Bearer $token")
                .build()
        } else {
            chain.request()
        }
        return chain.proceed(request)
    }
}
```

### 3. API service interfaces

```kotlin
// features/auth/data/remote/AuthApiService.kt
interface AuthApiService {
    @POST("auth/signup")
    suspend fun signup(@Body request: SignupRequest): ApiResponse<LoginResponse>

    @POST("auth/login")
    suspend fun login(@Body request: LoginRequest): ApiResponse<LoginResponse>
}

// features/chat/data/remote/ChatApiService.kt
interface ChatApiService {
    @POST("chats/direct/{userId}")
    suspend fun createDirectChat(@Path("userId") userId: String): ApiResponse<ChatResponse>

    @POST("chats/group")
    suspend fun createGroupChat(@Body request: CreateGroupRequest): ApiResponse<ChatResponse>
}

// features/chat/data/remote/MessageApiService.kt
interface MessageApiService {
    @GET("messages/{chatId}/messages")
    suspend fun getMessages(
        @Path("chatId") chatId: String,
        @Query("page") page: Int = 1,
        @Query("limit") limit: Int = 50,
    ): ApiResponse<MessageListResponse>
}
```

### 4. Response envelope

```kotlin
// core/data/remote/ApiResponse.kt — matches backend utils.SuccessResponse/ErrorResponse shape
@Serializable
data class ApiResponse<T>(
    val success: Boolean,
    val message: String? = null,
    val data: T? = null,
    val error: String? = null,
)

// Extension to unwrap safely
fun <T> ApiResponse<T>.toResult(): Result<T> {
    return if (success && data != null) {
        Result.success(data)
    } else {
        Result.failure(ApiException(message ?: error ?: "Unknown error"))
    }
}
```

### 5. Error handling for HTTP errors

```kotlin
// core/data/remote/ApiException.kt
class ApiException(
    message: String,
    val code: Int = 0
) : Exception(message)

// In repository:
override suspend fun login(email: String, password: String): Result<LoginResponse> {
    return runCatching {
        api.login(LoginRequest(email, password)).toResult().getOrThrow()
    }.mapCatching { response ->
        tokenStore.saveAccessToken(response.token)
        response
    }
}
```

### 6. Dependency injection wiring

```kotlin
// features/chat/data/di/ChatDataModule.kt
@Module
@InstallIn(SingletonComponent::class)
object ChatDataModule {
    @Provides @Singleton
    fun provideChatApi(retrofit: Retrofit): ChatApiService =
        retrofit.create(ChatApiService::class.java)

    @Provides @Singleton
    fun provideMessageApi(retrofit: Retrofit): MessageApiService =
        retrofit.create(MessageApiService::class.java)
}
```

### 7. Image upload

```kotlin
// Upload attachments as multipart
interface MediaApiService {
    @Multipart
    @POST("media/upload")
    suspend fun uploadFile(
        @Part file: MultipartBody.Part,
        @Part("chatId") chatId: RequestBody,
    ): ApiResponse<UploadResponse>
}
```

### 8. No network on Main thread

```kotlin
// ✓ GOOD — Retrofit suspend functions are main-safe
override suspend fun getMessages(chatId: String, page: Int): Result<List<Message>> =
    withContext(Dispatchers.IO) {
        runCatching {
            api.getMessages(chatId, page)
                .toResult()
                .getOrThrow()
                .messages
                .map { it.toDomain() }
        }
    }

// ✗ BAD — blocking the Main thread
override fun getMessages(chatId: String): List<Message> {
    return api.getMessages(chatId).execute().body()!!.messages
}