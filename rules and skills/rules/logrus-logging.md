# Logrus logging — ECHO

## Rule

1. Logger is **logrus** (github.com/sirupsen/logrus), not `log/slog`, not `zap` (except in the WebSocket hub which uses its own zap logger).
2. Output is **JSON** (`logrus.JSONFormatter`) to a file (`logs/server.log`) or stdout.
3. **Every log line carries `transaction_id`** (and `user_id`, `request_id` when known). They come from `context.Context`, extracted by `logger.WithContext(ctx)`.
4. Use `logger.WithContext(ctx)` to create a logrus entry with context fields. Don't call `logrus.Info(...)` directly.
5. Never log secrets, passwords, JWT tokens, full message content, or any PII beyond `user_id` and `username`.

## Why

- `transaction_id` is what lets ops trace a request across handler → service → DB. Logs without it are a graveyard.
- JSON in production is non-negotiable — structured fields enable grepping (`jq '.level=="ERROR"'`, `jq 'select(.user_id=="...")'`).
- A leaked password hash in logs is a P0; a single careless `log.WithField("passwordHash", hash).Info(...)` is enough to fail an audit.

## How to apply

### 1. Setup in `main.go`

```go
if err := logger.InitLogger("logs/server.log", Level); err != nil {
    panic(err)
}
```

`logger.InitLogger` creates a logrus instance with:
- `JSONFormatter` with timestamp format `2006-01-02T15:04:05.000Z07:00`.
- Caller reporting enabled (fields `0_level`, `1_file`, `2_func`, `3_msg`, `4_time`).
- Output to the file path or stdout if file creation fails.
- Cleans old log files on startup (removes all files from the log directory).
- Stores the logger in `objects.FileLog` (the global singleton).

### 2. Context propagation

The **AuthMiddleware** starts the context chain:

```go
// middleware/auth.go
reqCtx := logger.WithTransactionID(ctx.Request.Context())
// ... after verifying token ...
reqCtx = logger.WithUserID(reqCtx, claims.RegisteredClaims.Subject)
ctx.Request = ctx.Request.WithContext(reqCtx)
c.Set("user", user)
```

The **LoggerMiddleware** adds `request_id`:

```go
// middleware/logger.go
ctx := logger.WithRequestID(c.Request.Context())
c.Request = c.Request.WithContext(ctx)
// If user_id was set by AuthMiddleware, it gets picked up by logger.WithContext
```

### 3. Retrieving the logger in controllers and services

**From a Gin context (controller):**
```go
func SomeHandler(ctx *gin.Context) {
    reqCtx, log, ok := ReduceGinContextToContext(ctx)
    if !ok {
        return  // 401 sent
    }
    log.Info("doing something")
    // Pass reqCtx to services
    result, err := service.DoSomething(reqCtx, input)
}
```

**From a plain context (service):**
```go
func DoSomething(ctx context.Context, input string) error {
    log := logger.WithContext(ctx)
    log.WithField("input", input).Debug("processing")

    user, err := FindByID[models.User](ctx, coll, filter, projection)
    if err != nil {
        log.WithError(err).Error("Failed to find user")
        return fmt.Errorf("failed to find user: %w", err)
    }

    log.WithField("user_id", user.ID.Hex()).Info("user processed successfully")
    return nil
}
```

`logger.WithContext(ctx)` reads these keys from context and adds them as logrus fields:
- `transaction_id` (key: `objects.TransactionIDKey`)
- `user_id` (key: `objects.UserIDKey`)
- `request_id` (key: `objects.RequestIDKey`)

### 4. Levels — keep them honest

| Level | Use for |
| --- | --- |
| `Debug` | Expected business failures (wrong password, duplicate email, validation errors). Internal state for development. |
| `Info` | Successful business events (login succeeded, user created, message sent, WebSocket connected). Request log (one per HTTP request). |
| `Warn` | Recoverable anomalies (Redis unavailable — falling back, rate limit triggered, failed to clean old logs). |
| `Error` | Infrastructure failures (MongoDB down, Redis connection lost). Always include the error attribute via `WithError`. |
| `Fatal` | Unrecoverable startup failures (can't connect to DB on boot). |

**Do not** use `Error` for expected business failures (e.g., user entered wrong password) — that is `Debug`.

### 5. Structured fields, not string interpolation

```go
// ✓ Good — structured, greppable
log.WithFields(map[string]interface{}{
    "method":     c.Request.Method,
    "path":       c.Request.URL.Path,
    "status":     c.Writer.Status(),
    "duration":   duration.String(),
    "client_ip":  c.ClientIP(),
}).Info("HTTP Request")

// ✓ Good — single field
log.WithField("user_id", user.ID.Hex()).Info("login successful")
log.WithError(err).Error("Failed to connect to MongoDB")

// ✗ Bad — error vanishes into a string
log.Error(fmt.Sprintf("zuelpay failed for %s: %v", txn.ID, err))
```

### 6. The mandatory request log fields

Every HTTP request emits one access log via `LoggerMiddleware` with:

```
method, path, status, duration, client_ip, user_agent
```

Plus the contextual fields: `transaction_id`, `user_id`, `request_id`.

### 7. What never goes in logs

- Password hashes — even bcrypt outputs
- JWT access tokens (even partial)
- Full message content from chat
- Raw request/response bodies containing PII
- `Authorization` headers (even partial)
- Email addresses in full — `user_id` is sufficient

The JSON formatter does not have a redaction filter. The primary defense is **not logging sensitive data in the first place**.

### 8. Service-level logging pattern

```go
func FindByID[T any](ctx context.Context, collection *mongo.Collection, filter bson.M, projection bson.M) (*T, error) {
    log := logger.WithContext(ctx)
    log.WithField("collection", collection.Name()).Debug("Finding document by ID")

    var result T
    err := collection.FindOne(ctx, filter, options.FindOne().SetProjection(projection)).Decode(&result)
    if err != nil {
        if err == mongo.ErrNoDocuments {
            log.Debug("Document not found")
        } else {
            log.WithError(err).Error("Failed to find document")
        }
        return nil, err
    }

    log.Debug("Document found successfully")
    return &result, nil
}
```

### 9. WebSocket logging

The WebSocket hub (`internal/services/websocket.hub.go`) uses **zap** logger (`go.uber.org/zap`), not logrus. This is a separate concern — the hub is a long-running goroutine, not per-request. Hub logs use `zap.String`, `zap.Error`, `zap.Int` for structured fields.

The WebSocket controller (`internal/controller/websocket.controller.go`) mixes logrus (for request-context logging) and `log.Printf` (for connection lifecycle events). This is intentional and established — do not change it without discussion.

### 10. File management

- Logs go to `logs/server.log` (configured in `main.go`).
- File is truncated on each server start (old logs are cleaned first).
- If file creation fails, the logger falls back to stdout.
- There is no log rotation — the file is cleaned and recreated on each restart.
- In production, use a log shipper (e.g., Vector, Filebeat) to ship `logs/server.log` to a centralized system.