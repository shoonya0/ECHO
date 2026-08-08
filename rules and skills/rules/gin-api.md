# Gin API conventions — ECHO

## Rule

Gin is the **only** HTTP framework. Use it consistently:

1. One route registration function per module: `RegisterUserRoutes`, `RegisterChatRoutes`, `RegisterWebSocketRoutes`.
2. All routes mount under `objects.ApiBasePath` (`/echo/v1/`).
3. Handlers are thin — parse, validate, extract user context via `ReduceGinContextToContext`, call service, return with `utils.SuccessResponse`/`utils.ErrorResponse`.
4. Middleware order: `CORS → AuthMiddleware → LoggerMiddleware → handler`.
5. Responses always go through `utils.SuccessResponse` or `utils.ErrorResponse`.

## Why

- Consistent middleware order ensures every request gets authenticated, logged, and race-checked before hitting business logic.
- A standard response envelope lets clients share a single decode path.
- Centralized response helpers (`utils.SuccessResponse`, `utils.ErrorResponse`) prevent drift in JSON shape.

## How to apply

### Route registration pattern

Each module exposes a `RegisterXxxRoutes(r *gin.Engine)` function called from `routes.RegisterAPIRoutes`:

```go
// internal/api/chat.routes.go
func RegisterChatRoutes(r *gin.Engine) {
    chatApi := r.Group(objects.ApiBasePath)  // /echo/v1/

    chatRoutes := chatApi.Group("chats")
    {
        chatRoutes.POST("/direct/:userId", controller.CreateDirectChatHTTP)
        chatRoutes.POST("/group", controller.CreateGroupChatHTTP)
        chatRoutes.GET("/hub/stats", controller.GetHubStatsHTTP)
    }

    messageRoutes := chatApi.Group("messages")
    {
        messageRoutes.GET("/:chatID/messages", controller.GetChatMessagesHTTP)
    }
}

// internal/api/routes.go
func RegisterAPIRoutes(r *gin.Engine) {
    r.POST(objects.ApiBasePath+"login", Login())
    r.POST(objects.ApiBasePath+"signup", Signup())

    RegisterWebSocketRoutes(r)

    r.Use(middleware.AuthMiddleware)
    r.Use(middleware.LoggerMiddleware())

    RegisterUserRoutes(r)
    RegisterChatRoutes(r)
}
```

### Controller template

```go
func CreateDirectChatHTTP(ctx *gin.Context) {
    reqCtx, log, ok := ReduceGinContextToContext(ctx)
    if !ok {
        return  // already responded with 401
    }

    targetUserID := ctx.Param("userId")
    if targetUserID == "" {
        log.Debug("user id is required")
        utils.ErrorResponse(ctx, http.StatusBadRequest, "User ID is required", nil)
        return
    }

    targetObjectID, err := bson.ObjectIDFromHex(targetUserID)
    if err != nil {
        log.Debug("invalid user id format")
        utils.ErrorResponse(ctx, http.StatusBadRequest, "Invalid user ID format", nil)
        return
    }

    chat, err := services.CreateDirectChat(reqCtx, targetObjectID)
    if err != nil {
        log.Debug("failed to create direct chat")
        utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to create chat", nil)
        return
    }

    utils.SuccessResponse(ctx, "Direct chat created successfully", chat)
}
```

Things the controller **does not do**:
- Build Mongo queries or touch `objects.DB`.
- Call multiple services to compose a workflow.
- Log business events at `Info` level (service layer does that).
- Access Redis or WebSocket hub directly (unless it's the WebSocket controller).

### ReduceGinContextToContext

Every authenticated controller starts with this helper:

```go
// internal/controller/controller.go
func ReduceGinContextToContext(ctx *gin.Context) (context.Context, logrus.Entry, bool) {
    userInterface, exists := ctx.Get("user")
    if !exists {
        utils.ErrorResponse(ctx, http.StatusUnauthorized, "User not authenticated", nil)
        return nil, logrus.Entry{}, false
    }

    reqCtx := context.WithValue(ctx.Request.Context(), objects.UserDataKey, userInterface)
    log := logger.WithContext(reqCtx)

    return reqCtx, *log, true
}
```

This:
1. Extracts the user from `c.Get("user")` (set by `AuthMiddleware`).
2. Creates a `context.Context` with `objects.UserDataKey` carrying the user.
3. Creates a logrus entry with `transaction_id`, `request_id`, and `user_id` fields.
4. Returns `false` if user is not authenticated (already sent the 401 response).

### Middleware order — the canonical chain

In `main.go`:
```go
r := gin.New()
r.Use(gin.Recovery())          // panic recovery, returns 500
// CORS applied before auth so preflight requests work
r.Use(middleware.CORSMiddleware())
```

In `routes.RegisterAPIRoutes`:
```go
// Unauthenticated routes first
r.POST(objects.ApiBasePath+"login", Login())
r.POST(objects.ApiBasePath+"signup", Signup())

// WebSocket routes (they handle auth internally)
RegisterWebSocketRoutes(r)

// Then auth middleware for all subsequent routes
r.Use(middleware.AuthMiddleware)     // JWT validation, sets c.Set("user", user) + c.Set("userId", id)
r.Use(middleware.LoggerMiddleware()) // request logging with request_id, user_id, latency

// Authenticated routes
RegisterUserRoutes(r)
RegisterChatRoutes(r)
```

### Request validation

Use `binding:"required"` and `binding:"required,email"` tags on request structs, then `c.ShouldBindJSON(&req)`:

```go
var req SignupRequest
if err := c.ShouldBindJSON(&req); err != nil {
    utils.ErrorResponse(c, http.StatusBadRequest, "invalid request", err.Error())
    return
}
```

For complex validation beyond binding tags, validate inline in the controller or call a `utils.ValidateXxx()` helper.

### Response envelope

**Success:**
```json
{ "success": true, "message": "User created successfully", "data": { ... } }
```

**Error:**
```json
{ "success": false, "message": "invalid request", "error": "error details" }
```

Helpers:
```go
utils.SuccessResponse(c, "User created successfully", user)
utils.ErrorResponse(c, http.StatusBadRequest, "invalid request", err.Error())
utils.CreatedResponse(c, "Resource created", data)         // 201
utils.PaginatedResponse(c, "Items", items, limit, total)   // 200 with pagination wrapper
```

**Note:** The response envelope does **not** include `request_id`. It has `success`, `message`, `data` (optional), and `error` (optional). HTTP status codes per convention:

| Category | Status |
| --- | --- |
| Validation / bad request | 400 |
| Authentication | 401 |
| Conflict (e.g., email exists) | 409 |
| Not found | 404 |
| Internal error | 500 |
| Rate limited | 429 |

### API versioning

Current base: `/echo/v1/` (defined as `objects.ApiBasePath`). New endpoints land in the same prefix. **Never** modify an existing endpoint's contract — add an optional field or ship `/echo/v2/` for breaking changes.

### WebSocket routes

WebSocket endpoints use their own middleware group but still apply `AuthMiddleware`:

```go
func RegisterWebSocketRoutes(r *gin.Engine) {
    wsGroup := r.Group(objects.ApiBasePath + "ws")
    wsGroup.Use(middleware.AuthMiddleware)
    wsGroup.Use(middleware.LoggerMiddleware())
    wsGroup.GET("/chat", controller.HandleWebSocketChat)
}
```

WebSocket handlers upgrade the connection with `gorilla/websocket`, then register the client with the hub. They do **not** use `utils.SuccessResponse`/`utils.ErrorResponse` (the response is a WebSocket frame, not JSON).