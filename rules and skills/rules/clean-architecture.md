# Clean architecture — ECHO

## Rule

The ECHO project follows a three-layer flow:

```
Router (internal/api/) → Controller (internal/controller/) → Service (internal/services/)
```

All three layers access shared infrastructure through the `objects` package, which holds global singletons (MongoDB client, Redis client, config, logger). **Dependency direction:** controllers may call services; services may call `objects.DB`/`objects.RedisClient` directly. Models and utils are imported by both.

## Why

- This layout is established and internally consistent across the codebase. Controllers are thin (parse → validate → call service → respond with `utils.SuccessResponse`/`utils.ErrorResponse`).
- Services own business logic and DB access through generic helpers (`FindByID[T]`, `FindAll[T]`, `InsertOne[T]`, etc.) and module-specific functions.
- Breaking this convention creates two ways to do the same thing, which is what causes bugs in production.

## How to apply

### Layer responsibilities

| Layer | Owns | Must not |
| --- | --- | --- |
| `internal/api/` | Route registration (`RegisterXxxRoutes(r *gin.Engine)`), mounting on `objects.ApiBasePath` (`/echo/v1/`), applying middleware groups. | Contain business logic. Touch the DB. |
| `internal/controller/` | Parse + validate request (`c.ShouldBindJSON`, `c.Param`), extract user from context via `ReduceGinContextToContext`, call service, format response via `utils.SuccessResponse`/`utils.ErrorResponse`. | Contain business rules. Write raw SQL/Mongo queries. Compose multiple services into a flow (that's the service's job). |
| `internal/services/` | Business logic, generic MongoDB CRUD (`FindByID[T]`, `FindAll[T]`, `InsertOne[T]`, `InsertMany[T]`, `UpdateOne[T]`, `UpdateMany[T]`, `FindByFilter[T]`), module-specific functions (`CreateDirectChat`, `SendMessage`, `MarkMessagesAsRead`), Hub and PubSub management. | Touch `*gin.Context` (use `context.Context`). Format HTTP responses. |
| `internal/models/` | Domain entities with BSON+JSON tags, embedded sub-documents, request/response structs, WebSocket protocol types. | Import any other layer (models are pure). |
| `internal/middleware/` | Auth (JWT validation, user extraction to `c.Set("user", ...)`), Logger (request metadata), CORS, RateLimiter (in-memory sliding window). | Contain business rules. Call services directly. |
| `internal/utils/` | `SuccessResponse`/`ErrorResponse` envelope, JWT helpers, validation utilities, model constructors (`NewUserWithDefaults`), converters, pagination int parser. | Import controllers or services. |
| `logger/` | Logrus initialization, context propagation (`WithContext`, `WithTransactionID`, `WithUserID`, `WithRequestID`). | Know about any business module. |
| `objects/` | Global singletons (`DB`, `DBClient`, `RedisClient`, `FileLog`, `MainConfiguration`), typed constants (Collection names, API paths, user statuses, message types, chat types, context keys). | Contain any logic beyond constants and type definitions. |
| `config/` | Viper config loading from `config/envConfig/dev.env`. | Be imported by anything other than `main.go`. |
| `internal/db/` | DB connection (`ConnectDB`, `ConnectRedis`) called once from `main.go`. | Be imported by controllers or services. |

### Concrete example — chat flow

```
internal/api/routes.go        // r.POST("/echo/v1/chats/direct/:userId", controller.CreateDirectChatHTTP)
internal/controller/chat.controller.go  // CreateDirectChatHTTP — parse param, call services.CreateDirectChat
internal/services/chat.service.go       // CreateDirectChat — check existing, start session, insert chat + update users, return *Chat
internal/models/chat.model.go           // Chat, ParticipantEmbed, ChatInvitationEmbed structs
internal/utils/response.go             // SuccessResponse(ctx, "Direct chat created successfully", chat)
```

### The composition root is `main.go`

`main.go` is the only place that wires everything: loads config, initializes logger, connects DB and Redis, sets up Gin with middleware, calls `routes.RegisterAPIRoutes(r)`, starts the WebSocket hub goroutine, and runs the server. **Modules do not import each other's globals**; they use `objects` package for shared state.

### What goes in `objects/` vs `internal/`

- `objects/` — global singletons and constants with **zero** circular imports. Every package can safely import `objects`.
- `internal/` — everything that depends on ECHO domain models or config.

### Cross-module calls

When service A needs service B's data, it imports `objects` to access `objects.DB.Collection(string(objects.UserColl))` and calls the generic `services.FindByID[T]` or a module-specific helper like `services.GetUserByID`. The `objects` package is the shared dependency injection mechanism — all singletons are set up once in `main.go` and accessed globally thereafter.

### No repository layer

ECHO does **not** have a `repository/` layer. Services call MongoDB directly through:
1. Generic helpers in `internal/services/service.go` (`FindByID[T]`, `FindAll[T]`, etc.)
2. Module-specific collection calls (e.g., `objects.DB.Collection(string(objects.ChatColl)).InsertOne(ctx, chat)`)
3. MongoDB transaction sessions (`objects.DB.Client().StartSession()` + `sess.WithTransaction(ctx, fn)`)

Do not introduce a `repository/` directory without explicit discussion — it adds indirection that doesn't match the established pattern.