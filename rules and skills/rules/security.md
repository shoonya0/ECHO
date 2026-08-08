# Security — ECHO

## Rule

1. **JWT** is the client auth mechanism. HS256, 7-day expiry, `Bearer` token in `Authorization` header. Tokens carry user profile + presence claims.
2. **Passwords** are hashed with **bcrypt** (`bcrypt.GenerateFromPassword` + `bcrypt.CompareHashAndPassword`). Passwords are never logged.
3. **Rate limiting** at the edge: in-memory token bucket (sliding window), keyed by client IP, returning 429 `RATE_LIMIT_EXCEEDED`.
4. **Secrets** are never in code. Read via env file (`config/envConfig/dev.env`) using Viper. `JWT_SECRET`, `DB_URI`, `REDIS_URI`, `REDIS_PASS` are env-only.
5. **MongoDB injection** is prevented by using `bson.M` filters and `mongo-driver`'s safe API — no string-concatenated queries ever.
6. **WebSocket auth**: JWTs validated during the HTTP upgrade request via `AuthMiddleware`; user set in context before `HandleWebSocketChat` runs.

## Why

- 7-day JWT tokens are a deliberate tradeoff for a chat app (session persistence across app restarts). The token carries the user profile to avoid a DB roundtrip on every WebSocket message.
- bcrypt is the standard for password hashing; its cost factor makes offline brute-force expensive.
- In-memory rate limiting is simple and effective for a single-instance deployment; no Redis dependency.
- Env-based config ensures no secrets leak via git.

## How to apply

### 1. JWT

**Signing:**
```go
// internal/utils/jwt.go
func GetJWTToken(user models.LoginUserResponse, exp int64) (string, error) {
    jwtSecret := GetJWTSecret()  // reads objects.MainConfiguration.JwtSecret

    claims := JwtClaims{
        Email:         user.Email,
        Username:      user.Username,
        DisplayName:   user.Profile.DisplayName,
        AccountStatus: user.AccountStatus,
        Presence:      PresenceClaims{...},
        RegisteredClaims: jwt.RegisteredClaims{
            Issuer:    "Echo",
            Subject:   user.ID.Hex(),
            Audience:  jwt.ClaimStrings{"Web-App", "Mobile-App"},
            ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
            NotBefore: jwt.NewNumericDate(time.Now().Add(-1 * time.Minute)),
            IssuedAt:  jwt.NewNumericDate(time.Now()),
            ID:        uuid.New().String(),
        },
    }

    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString(jwtSecret)
}
```

**Validation (in AuthMiddleware):**
```go
// internal/middleware/auth.go
func verifyToken(tokenString string) (utils.JwtClaims, error) {
    token, err := jwt.ParseWithClaims(tokenString, &claims, func(token *jwt.Token) (interface{}, error) {
        if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
            return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
        }
        return []byte(objects.MainConfiguration.JwtSecret), nil
    })
    // ... validate expiry, notBefore, subject, account status
}
```

JWT secret comes from `objects.MainConfiguration.JwtSecret`, which is loaded from the env file. Trim whitespace and quotes before use:

```go
func GetJWTSecret() []byte {
    secret := objects.MainConfiguration.JwtSecret
    secret = strings.TrimSpace(secret)
    secret = strings.Trim(secret, "\"'")
    if secret == "" {
        panic("JWT_SECRET is not set in configuration")
    }
    return []byte(secret)
}
```

**Claims carried in JWT:**
- `sub` — user ID (ObjectID hex)
- `email`, `username`, `displayName`
- `accountStatus` (isActive, isVerified, isBanned)
- `presence` (status, lastSeen, lastActivity)
- Standard: `iss`, `aud`, `exp`, `nbf`, `iat`, `jti`

### 2. Password hashing

**Registration:**
```go
hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
if err != nil {
    utils.ErrorResponse(c, http.StatusInternalServerError, "could not hash password", err.Error())
    return
}
user := utils.NewUserWithDefaults(req.ID, req.Email, req.Username, string(hash))
```

**Login:**
```go
if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
    utils.ErrorResponse(c, http.StatusUnauthorized, "invalid credentials", err.Error())
    return
}
```

**Critical:** The `PasswordHash` field has `json:"-"` tag — it is **never** serialized in API responses. Projections that include `passwordHash` are only used during login.

### 3. Rate limiting

In-memory sliding window (`internal/middleware/rate_limit.go`):

```go
func RateLimitMiddleware(window time.Duration, limit int) gin.HandlerFunc {
    limiter := NewRateLimiter(window, limit)
    return func(c *gin.Context) {
        clientIP := c.ClientIP()
        if !limiter.isAllowed(clientIP) {
            c.JSON(http.StatusTooManyRequests, gin.H{
                "error": "rate limit exceeded",
                "code":  "RATE_LIMIT_EXCEEDED",
            })
            c.Abort()
            return
        }
        c.Next()
    }
}
```

The rate limiter is **not currently activated** in the route registration (`routes.go`). To enable it, add `r.Use(middleware.RateLimitMiddleware(1*time.Minute, 60))` to the middleware chain in `RegisterAPIRoutes`.

### 4. Secrets management

**Config file:** `config/envConfig/dev.env` (gitignored, template `dev.env.example`)

Required env vars:
```
PORT=8080
DB_URI=mongodb://localhost:27017
JWT_SECRET=<random-64-char>
REDIS_URI=redis://localhost:6379
REDIS_PASS=
```

Loaded via Viper:
```go
// config/main.config.go
viper.SetConfigName("dev")
viper.SetConfigType("env")
viper.AutomaticEnv()
viper.ReadInConfig()
viper.Unmarshal(&objects.MainConfiguration)
```

**Critical rules:**
- Never add `dev.env` to git. The `.gitignore` already excludes it.
- Never hardcode `JWT_SECRET` or `DB_URI` in source files.
- `objects.MainConfiguration` is the only place config values are read.

### 5. MongoDB injection safety

MongoDB queries use `bson.M` (safe parameterized map), never string concatenation:

```go
// ✓ Safe
filter := bson.M{"email": req.Email}
user := objects.DB.Collection(string(objects.UserColl)).FindOne(ctx, filter)

// ✓ Safe — complex filters
filter := bson.M{
    "chatType": "direct",
    "$and": []bson.M{
        {"participants." + userID.Hex(): bson.M{"$exists": true}},
        {"participants." + targetID.Hex(): bson.M{"$exists": true}},
    },
}
```

The `bson.M` type is a `map[string]interface{}` and the mongo driver handles all parameterization. There is no SQL-injection-equivalent risk with `bson.M` filters.

### 6. WebSocket auth

WebSocket connections are authenticated during the HTTP upgrade:

```go
// routes.go
wsGroup := r.Group(objects.ApiBasePath + "ws")
wsGroup.Use(middleware.AuthMiddleware)  // validates JWT, sets c.Set("user", user)
wsGroup.GET("/chat", controller.HandleWebSocketChat)
```

The controller extracts the authenticated user:
```go
func HandleWebSocketChat(ctx *gin.Context) {
    reqCtx, log, ok := ReduceGinContextToContext(ctx)
    // ...
    user, exists := reqCtx.Value(objects.UserDataKey).(models.LoginUserResponse)
    // ... upgrade connection, create client with user.ID
}
```

### 7. CORS

```go
// middleware/cors.go
func CORSMiddleware() gin.HandlerFunc {
    // Uses github.com/gin-contrib/cors
    // Configured to allow all origins (for development)
    // Tighten for production
}
```

CORS is applied **first** in the middleware chain so preflight `OPTIONS` requests are handled before auth.

### 8. Input validation

- Request structs use `binding:"required"` and `binding:"required,email"` tags.
- Message content is limited to 4000 characters.
- Attachments limited to 10 files, 50MB each.
- Mentions limited to 20 users.
- ObjectID validation on all ID parameters via `bson.ObjectIDFromHex`.

### 9. Output hygiene

- `utils.ErrorResponse` sends a static message string + optional raw error. Be intentional about what error details you pass — for production, pass `nil` or a sanitized string.
- The `User.PasswordHash` field has `json:"-"` — it can never leak in JSON responses.
- Stack traces are not returned in HTTP responses (Gin's recovery middleware returns generic 500).

### 10. Dependencies

Run `go vet ./...` regularly. Check for known vulnerabilities with `govulncheck ./...` if installed. Keep dependencies updated — the go.mod already uses recent versions.