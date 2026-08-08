# Functional programming in Go — ECHO

## Rule

ECHO uses a **pragmatic functional style** within the service layer:

1. **Pure helper functions** operate on data without side effects — input in, output out. No DB, no logging, no `time.Now()`.
2. **Generic helpers** (`FindByID[T]`, `FindAll[T]`, etc.) abstract common MongoDB operations behind typed signatures.
3. **Immutable-by-convention**: don't mutate input slices, maps, or structs. Return new values.
4. **No package-level mutable state** outside the sanctioned `objects` singletons (`objects.DB`, `objects.RedisClient`, `objects.FileLog`) and `sync.Once`-guarded singletons (hub, presence).

## Why

- Pure functions are trivially testable with table-driven tests — no mocks, no fixtures.
- Generic DB helpers reduce repetition: one `FindByID[T]` replaces a per-collection `findUserByID`, `findChatByID`, `findMessageByID`.
- Immutability conventions make concurrent code (WebSocket hub, goroutines) easier to reason about.

## How to apply

### 1. Split pure logic from I/O

Pure functions live alongside service functions in `internal/services/`:

```go
// Pure core — trivially testable.
// No DB, no time.Now, no logging.
func CountActiveUsers(users []models.User) int {
    count := 0
    for _, user := range users {
        if user.AccountStatus.IsActive {
            count++
        }
    }
    return count
}

func RemoveElement(slice *[]bson.ObjectID, element bson.ObjectID) *[]bson.ObjectID {
    if slice == nil {
        return slice
    }
    result := make([]bson.ObjectID, 0, len(*slice))
    for _, item := range *slice {
        if item != element {
            result = append(result, item)
        }
    }
    return &result
}
```

These are called from the imperative shell (services that do I/O):

```go
// Imperative shell — orchestrates I/O and calls pure helpers.
func SomeServiceFunction(ctx context.Context, input Input) error {
    log := logger.WithContext(ctx)
    users, err := FindAll[models.User](ctx, coll, filter, nil, nil, 0, 0)
    if err != nil {
        return err
    }

    activeCount := CountActiveUsers(users)  // ← pure helper
    log.WithField("active_count", activeCount).Info("counted active users")

    return nil
}
```

### 2. Generic MongoDB helpers

All collection-agnostic DB operations use Go generics:

```go
// internal/services/service.go

func FindByID[T any](ctx context.Context, collection *mongo.Collection, filter bson.M, projection bson.M) (*T, error)
func FindAll[T any](ctx context.Context, collection *mongo.Collection, filter bson.M, projection bson.M, sort bson.M, limit int64, skip int64) ([]T, error)
func FindByFilter[T any](ctx context.Context, collection *mongo.Collection, filter bson.M, projection bson.M) (T, error)
func InsertOne[T any](ctx context.Context, coll *mongo.Collection, document T) (bson.ObjectID, error)
func InsertMany[T any](ctx context.Context, coll *mongo.Collection, documents []T) ([]bson.ObjectID, error)
func UpdateOne[T any](ctx context.Context, coll *mongo.Collection, filter bson.M, update T, opts ...options.Lister[options.UpdateOneOptions]) (*mongo.UpdateResult, error)
func UpdateMany[T any](ctx context.Context, coll *mongo.Collection, filter bson.M, update T, opts ...options.Lister[options.UpdateManyOptions]) (*mongo.UpdateResult, error)
```

When the generic helper doesn't fit (complex `$and`/`$or`, non-standard decoding), fall back to direct `objects.DB.Collection(...).FindOne(ctx, filter, opts).Decode(&result)`.

### 3. Immutability by convention

- Don't mutate input slices or maps. Return new values.
- Use **value receivers** for domain models unless pointer semantics are needed.
- Prefer constructing a new struct over field mutation.

```go
// ✓ Good — returns a new User
func NewUserWithDefaults(id bson.ObjectID, email, username, passwordHash string) models.User {
    return models.User{
        ID:           id,
        Email:        email,
        Username:     username,
        PasswordHash: passwordHash,
        AccountStatus: models.AccountStatusEmbed{
            IsActive:   true,
            IsVerified: false,
            IsBanned:   false,
        },
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
    }
}

// ✗ Bad — mutates caller's value silently
func (u *User) SetStatus(active bool) { u.AccountStatus.IsActive = active }
```

Exception: `objects` globals (`objects.DB`, `objects.MainConfiguration`) are set once at startup in `main.go`/`config.New()` and read-only thereafter.

### 4. Context propagation over globals

Pass `context.Context` to every function that needs logging or cancellation — don't access `gin.Context` outside handlers:

```go
// ✓ Good — context flows down
func DoSomething(ctx context.Context, input Input) error {
    log := logger.WithContext(ctx)
    // ...
}

// ✗ Bad — global context, no cancellation support
var globalCtx = context.Background()
```

### 5. No package-level mutable state

The only package-level variables allowed:

- `objects.MainConfiguration` — set once via Viper in `config.New()`.
- `objects.DBClient`, `objects.DB` — set once in `db.ConnectDB()`.
- `objects.RedisClient` — set once in `db.ConnectRedis()`.
- `objects.FileLog` — set once in `logger.InitLogger()`.
- `services.GetHubInstance()` — `sync.Once` singleton.
- `services.GetPresenceInstance()` — `sync.Once` singleton.

Everything else enters through function parameters and constructors.

### 6. Interface usage is minimal

ECHO does not use interfaces heavily. The generic helpers and concrete types suffice. If you need to mock in tests, pass a function parameter or use a test-specific build tag. Do not introduce interface-heavy DI without discussion — the project values simplicity over abstraction.