# Error handling — ECHO

## Rule

1. Errors are **values**. Return them, wrap with `fmt.Errorf("context: %w", err)`, never panic in business code.
2. Every error maps to an HTTP status code and a message string passed to `utils.ErrorResponse`.
3. Error code strings (from the README) are used in WebSocket error frames; HTTP errors use status codes rather than code strings.
4. **Panics** are recovered only by `gin.Recovery()`. Anything that panics is a bug.

## Why

- A consistent error response shape lets the client handle errors predictably.
- Wrapping adds context so a single log line in production tells the on-caller where it broke.
- `gin.Recovery()` catches panics and returns 500; deliberate panics in handlers would skip proper cleanup.

## How to apply

### 1. Wrap errors with context at each layer boundary

```go
// service layer — wrap with operation context
func CreateDirectChat(ctx context.Context, targetUserID bson.ObjectID) (*models.Chat, error) {
    chat, err := doSomething(ctx, targetUserID)
    if err != nil {
        return nil, fmt.Errorf("failed to create direct chat: %w", err)
    }
    return chat, nil
}

// repository access (via generic helpers)
user, err := FindByID[models.User](ctx, coll, filter, projection)
if err != nil {
    if err == mongo.ErrNoDocuments {
        return fmt.Errorf("user not found: %w", err)
    }
    return fmt.Errorf("failed to find user: %w", err)
}
```

### 2. Error codes for recognized failure modes

The project uses string error codes in WebSocket error frames (from `README.md`):

| Code                          | Meaning                           |
| ----------------------------- | --------------------------------- |
| `INVALID_REQUEST`           | Missing or invalid parameters     |
| `PERMISSION_DENIED`         | User lacks required permissions   |
| `INVALID_CHAT_ID`           | Chat ID format is invalid         |
| `MESSAGE_FAILED`            | Failed to send message            |
| `PERMISSION_CHECK_FAILED`   | Error validating permissions      |
| `INVALID_CONTENT`           | Message content validation failed |
| `PARSE_ERROR`               | Failed to parse WebSocket message |
| `UNKNOWN_REQUEST`           | Unknown WebSocket request type    |
| `NOT_IMPLEMENTED`           | Feature not yet implemented       |
| `CHANNEL_FULL`              | Client send channel full          |
| `TOKEN_VERIFICATION_FAILED` | JWT invalid or expired            |
| `RATE_LIMIT_EXCEEDED`       | Too many requests                 |
| `MISSING_AUTH_HEADER`       | No Authorization header           |
| `INVALID_AUTH_FORMAT`       | Header not in Bearer format       |
| `EMPTY_TOKEN`               | Bearer token is empty             |
| `USER_NOT_AUTHENTICATED`    | User not in context               |

These are used in WebSocket error messages sent to clients. HTTP endpoints use status codes + human-readable messages instead of code strings (though the rate limiter sends `"code": "RATE_LIMIT_EXCEEDED"` in its JSON body).

### 3. Mapping errors to HTTP responses

Controllers map the error to an HTTP status and a **human-readable message** (not the raw error):

```go
if err != nil {
    log.Debug("failed to create group chat ", err)
    utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to create group chat", nil)
    return
}
```

Error messages in HTTP responses are **static strings** like `"Failed to create group chat"`. Do **not** echo `err.Error()` to the client.

### 4. Logging errors

- **Debug** level: expected user-facing failures (wrong password, duplicate email, validation fails). These are not bugs — they're normal control flow.
- **Error** level: infrastructure failures (MongoDB down, Redis unavailable). Always include `log.WithError(err)`.
- **Info** level: successful business events (login succeeded, message sent).

```go
// Expected — Debug
log.Debug("email already registered")

// Unexpected — Error
log.WithError(err).Error("Failed to create user")

// Success — Info
log.WithField("user", user.ID.Hex()).Info("login successful")
```

### 5. WebSocket error handling

WebSocket handlers send error frames to the client via `sendErrorResponse`:

```go
func sendErrorResponse(client *models.Client, requestID, code, message string) {
    errorMsg := models.WebSocketMessage{
        Type: models.WSMessageTypeError,
        Data: models.ErrorMessage{
            Code:    code,
            Message: message,
        },
        RequestID: requestID,
        Timestamp: time.Now(),
    }
    select {
    case client.Send <- errorMsg:
    default:
        log.Printf("Failed to send error response to client %s: channel full", client.ID)
    }
}
```

WebSocket errors use `log.Printf` (not structured logrus) because the WebSocket code imports `log` from stdlib.

### 6. Error vs. nil distinction

For `FindByFilter[T]` which returns `(T, error)` (value, not pointer):

- `mongo.ErrNoDocuments` → return zero value + error.
- Other errors → return zero value + wrapped error.
- Success → return value + nil.

For `FindByID[T]` which returns `(*T, error)`:

- `mongo.ErrNoDocuments` → return nil + error.
- Other errors → return nil + wrapped error.
- Success → return pointer + nil.

Always check `err == mongo.ErrNoDocuments` before treating it as a not-found vs a DB failure.

### 7. Panics — only for unrecoverable startup failures

```go
// ✓ OK — can't start without config
if secret == "" {
    panic("JWT_SECRET is not set in configuration")
}

// ✓ OK — can't proceed without DB
if err := db.ConnectDB(ctx); err != nil {
    log.WithError(err).Fatal("Failed to connect to database")
}

// ✗ BAD — never panic from a handler or service
```

The `gin.Recovery()` middleware catches any panic during request handling and returns 500. Worker goroutines should use defer+recover when appropriate.
