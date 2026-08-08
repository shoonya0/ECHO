# MongoDB access — ECHO

## Rule

1. **Only `internal/services/` talks to MongoDB.** Controllers and middleware must not import `mongo-driver` or touch `objects.DB`.
2. Database access goes through either the **generic CRUD helpers** in `internal/services/service.go` or module-specific functions that call `objects.DB.Collection(...)` directly with `bson.M` filters and projections.
3. Multi-document atomic writes use **MongoDB transaction sessions** (`Client().StartSession()` + `WithTransaction`).
4. Every collection document carries `_id bson.ObjectID`, `createdAt`, and `updatedAt` timestamps.
5. Deletes are **soft** — set a `deleted` flag or remove from arrays; never `DeleteOne`/`DeleteMany` for user-visible data.

## Why

- A controller that builds Mongo queries mixes the HTTP layer with data access, making it impossible to test business logic without HTTP.
- The generic helpers (`FindByID[T]`, `FindAll[T]`, `InsertOne[T]`) handle logging, error wrapping, and projection/limit/sort options — duplicating this logic in every module causes drift.
- MongoDB sessions are how ECHO ensures atomic multi-doc operations (e.g., creating a chat + updating user chat lists atomically).

## How to apply

### 1. Generic CRUD helpers (preferred for simple ops)

```go
// internal/services/service.go — available to all service modules

// Find a single document by filter
user, err := FindByID[models.User](ctx, objects.DB.Collection(string(objects.UserColl)), filter, projection)
if err != nil {
    if err == mongo.ErrNoDocuments {
        // handle not found
    }
    return err
}

// Find all matching documents
users, err := FindAll[models.User](ctx, coll, filter, projection, sort, limit, skip)

// Insert one
id, err := InsertOne(ctx, coll, document)

// Insert many
ids, err := InsertMany(ctx, coll, documents)

// Update one
result, err := UpdateOne(ctx, coll, filter, bson.M{"$set": bson.M{"field": value}})

// Update many
result, err := UpdateMany(ctx, coll, filter, bson.M{"$set": bson.M{"field": value}})

// Find by arbitrary filter (returns value, not pointer)
result, err := FindByFilter[models.Chat](ctx, coll, filter, projection)
```

These helpers:
- Accept `context.Context` (not `*gin.Context`).
- Log debug/error with collection name.
- Wrap errors with `fmt.Errorf("description: %w", err)`.
- Return `*T` for single-doc lookups (nil on `ErrNoDocuments`).

### 2. Direct collection access (for complex queries)

When a query needs multiple filters, complex `$and`/`$or`, or result decoding into a non-standard shape, call the collection directly:

```go
// internal/services/chat.service.go
err := objects.DB.Collection(string(objects.ChatColl)).FindOne(ctx, filter, options.FindOne().SetProjection(projection)).Decode(&chat)
```

Always use `options.FindOne().SetProjection(projection)` to limit returned fields — never return full documents with password hashes or large arrays you don't need.

### 3. Projections — always limit returned fields

```go
userProjection := bson.M{
    "_id":                 1,
    "email":               1,
    "username":            1,
    "passwordHash":        1,  // only when authenticating
    "profile.displayName": 1,
    "profile.avatar":      1,
    "accountStatus":       1,
    "presence.status":     1,
    "presence.lastSeen":   1,
}
```

Every query outside the `UserColl` itself must project only needed fields. Never fetch a full user document unless you need every field.

### 4. MongoDB transactions — use for multi-document atomicity

When an operation updates two or more collections and must be atomic:

```go
sess, err := objects.DB.Client().StartSession()
if err != nil {
    return fmt.Errorf("failed to start session: %w", err)
}
defer sess.EndSession(ctx)

_, err = sess.WithTransaction(ctx, func(sessCtx context.Context) (interface{}, error) {
    // All DB ops inside this callback use sessCtx, not ctx
    _, err := objects.DB.Collection(string(objects.ChatColl)).InsertOne(sessCtx, chat)
    if err != nil {
        return nil, fmt.Errorf("failed to insert chat: %w", err)
    }

    _, err = objects.DB.Collection(string(objects.UserColl)).UpdateMany(sessCtx, filter, update)
    if err != nil {
        return nil, fmt.Errorf("failed to update users: %w", err)
    }

    return nil, nil
})
```

**Critical:** All operations inside `WithTransaction` must use `sessCtx`, not the outer `ctx`. The session context carries the transaction.

### 5. bson.ObjectID — the primary key type

Every collection document uses `bson.ObjectID` for `_id`. Convert from hex strings:

```go
id, err := bson.ObjectIDFromHex(idString)
if err != nil {
    return fmt.Errorf("invalid ID format: %w", err)
}
```

Export IDs as strings with `.Hex()`:

```go
chatIDStr := chat.ChatID.Hex()
```

### 6. Collections and database constants

All collection names are typed constants in `objects/`:

```go
const (
    DBName      Database   = "Echo"
    UserColl    Collection = "users"
    ChatColl    Collection = "chats"
    MessageColl Collection = "messages"
)
```

Always access them as `objects.DB.Collection(string(objects.UserColl))`. Never hardcode collection name strings.

### 7. Embedded documents pattern

ECHO uses deeply embedded sub-documents in MongoDB to minimize joins:

```go
type User struct {
    ID       bson.ObjectID     `json:"_id,omitempty" bson:"_id,omitempty"`
    Profile  UserProfileEmbed  `json:"profile" bson:"profile"`
    Presence PresenceEmbed     `json:"presence" bson:"presence"`
    Chats    map[bson.ObjectID]int `json:"chats" bson:"chats"` // chatID → unread count
}
```

Embed when:
- Sub-data is always accessed together with the parent.
- Sub-data has a bounded size (not an unbounded array).
- You want atomic updates to parent + sub-data.

Do **not** embed unbounded arrays (e.g., all messages in a chat) — those go in their own collection.

### 8. Soft deletes

- Chats: never physically deleted. Invite codes get `deleted: true` flag.
- Messages: `status.isDeleted: true` (mark, don't remove).
- Users: no deletion support — `accountStatus.isBanned` or `accountStatus.isActive=false`.

Query filters default to `bson.M{"deleted": bson.M{"$ne": true}}` or equivalent status checks.

### 9. No migrations

MongoDB is schema-flexible. There is no `migrations/` directory. Schema evolution happens by:
1. Adding new optional fields (old documents simply don't have them).
2. Writing backward-compatible code that handles missing fields.
3. Running ad-hoc migration scripts (not part of server boot) for large-scale backfills.

### 10. Indexes

Current collections should be indexed on frequently queried fields:
- `users`: `email` (unique), `username`
- `chats`: `_id`, `participants.<userID>`
- `messages`: `chatId`, `senderId`, `createdAt`

When adding a query that filters on a new field, add an index via the MongoDB shell or a migration script — document the index in a code comment near the query.

### 11. Connection management

- `ConnectDB` in `internal/db/db.go` is called once from `main.go`.
- `objects.DBClient` is the global `*mongo.Client`.
- `objects.DB` is `objects.DBClient.Database("Echo")`.
- Services get collections from `objects.DB.Collection(...)`.
- There is no connection pooling to manage — the mongo driver handles it.