# Testing — ECHO

## Rule

1. Every pure helper function has a **table-driven** unit test.
2. Every service function with DB access has an **integration** test against a real MongoDB.
3. WebSocket handlers are testable with `gorilla/websocket`'s `httptest` server.
4. Tests are **deterministic** — no real time, no real randomness, no real network calls to external services.
5. `go test -race ./...` passes.

## Why

- Chat code has a high blast radius. Tests are how we avoid regressions.
- Table-driven tests document behavior inline — readers learn the rules from the test cases.
- Deterministic tests don't flake; flakes destroy trust in the suite.

## How to apply

### 1. Table-driven unit tests for pure functions

```go
func TestCountActiveUsers(t *testing.T) {
    tests := []struct {
        name  string
        users []models.User
        want  int
    }{
        {"no users",       []models.User{}, 0},
        {"one active",     []models.User{{AccountStatus: models.AccountStatusEmbed{IsActive: true}}}, 1},
        {"one inactive",   []models.User{{AccountStatus: models.AccountStatusEmbed{IsActive: false}}}, 0},
        {"mixed",          []models.User{
            {AccountStatus: models.AccountStatusEmbed{IsActive: true}},
            {AccountStatus: models.AccountStatusEmbed{IsActive: false}},
            {AccountStatus: models.AccountStatusEmbed{IsActive: true}},
        }, 2},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := CountActiveUsers(tt.users)
            require.Equal(t, tt.want, got)
        })
    }
}
```

Pure functions to test:
- `internal/services/service.go`: `CountActiveUsers`, `RemoveElement`, `contains`, `findIndex`
- `internal/utils/`: validation helpers, `SplitAndTrim`, `ParseInt`, model constructors
- `internal/controller/controller.go`: `BuildPartialDocument`, `buildNestedUpdate`

### 2. Integration tests for MongoDB operations — `//go:build integration`

```go
//go:build integration

func TestFindByID_User(t *testing.T) {
    // Connect to real MongoDB (test database)
    pool := testdb.Connect(t)           // helper: connect to test MongoDB
    defer pool.Disconnect(t)

    coll := pool.Collection("users")

    // Insert test user
    user := utils.NewUserWithDefaults(bson.NewObjectID(), "test@email.com", "testuser", "hashed")
    id, err := InsertOne(context.Background(), coll, user)
    require.NoError(t, err)

    // Find and verify
    found, err := FindByID[models.User](context.Background(), coll, bson.M{"_id": id}, nil)
    require.NoError(t, err)
    require.Equal(t, user.Email, found.Email)
    require.Equal(t, user.Username, found.Username)
}
```

Tag integration tests with `//go:build integration` and run with:
```bash
go test -tags=integration ./...
```

Default `go test ./...` runs only unit tests.

### 3. HTTP handler tests with `httptest`

```go
func TestCreateDirectChatHTTP(t *testing.T) {
    r := setupTestRouter(t)  // sets up Gin with middleware, test DB, auth

    body := `{"name": "Test Group", "participants": ["` + testUserID + `"]}`
    req := httptest.NewRequest(http.MethodPost, "/echo/v1/chats/group", strings.NewReader(body))
    req.Header.Set("Authorization", "Bearer "+testJWT)
    req.Header.Set("Content-Type", "application/json")

    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    require.Equal(t, http.StatusOK, w.Code)

    var resp utils.APIResponse
    require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
    require.True(t, resp.Success)
}
```

### 4. WebSocket handler tests

```go
func TestWebSocketChat(t *testing.T) {
    s := httptest.NewServer(setupTestRouter(t))
    defer s.Close()

    wsURL := "ws" + strings.TrimPrefix(s.URL, "http") + "/echo/v1/ws/chat"
    conn, _, err := websocket.DefaultDialer.Dial(wsURL+"?token="+testJWT, nil)
    require.NoError(t, err)
    defer conn.Close()

    // Send a message
    msg := models.MessageRequest{
        Type:    models.WSRequestTypeSendMessage,
        ChatID:  testChatID,
        Content: "hello",
    }
    err = conn.WriteJSON(msg)
    require.NoError(t, err)

    // Read response
    var resp models.WebSocketMessage
    err = conn.ReadJSON(&resp)
    require.NoError(t, err)
    require.Equal(t, models.WSMessageTypeResponse, resp.Type)
}
```

### 5. Fake the dependencies, not the core

For unit testing services:
- **DB** → use a test MongoDB instance (tagged `integration`) or skip DB-dependent tests in unit mode.
- **Time** → use fixed timestamps in test fixtures.
- **UUIDs** → use deterministic IDs in test fixtures.
- **Redis** → mock or skip (Redis is optional for presence features).

### 6. Race detector

```bash
go test -race ./...
```

Required to pass. The WebSocket hub has significant concurrent access (goroutines for read/write, mutexes on client maps) — race detector catches protocol violations.

### 7. Coverage — guidance, not target

Don't enforce a percentage. Instead:

- Every pure function in `service.go` and `utils/` has a table test.
- Every handler has at least one happy-path HTTP test.
- Error branches get at least one test case walking that branch.
- New code lands with tests; legacy code is covered when it changes.

### 8. Errors in tests

Use `require` for hard failures (can't continue the test) and `assert` for soft checks (continue and report multiple failures):

```go
require.NoError(t, err)          // fatal if err != nil
require.Equal(t, expected, got)  // fatal if not equal
assert.Equal(t, expected, got)   // non-fatal, continue test
```

### 9. Determinism — the master rule

- No `time.Sleep` in tests. Use fixed timestamps or synchronous channels.
- No `time.Now()` in test fixtures — pass a fixed time.
- No random IDs — use `bson.NewObjectID()` in test setup (deterministic within the test).
- No real network calls to external services (ZuelPay, FCM, etc. don't apply to ECHO).

### 10. Test helpers

Create test helpers in a `testutil/` or `tests/helpers/` package:
- `setupTestRouter(t)` — creates a Gin engine with middleware, auth bypass for test tokens.
- `testdb.Connect(t)` — connects to a test MongoDB (separate database like `Echo_test`).
- `createTestUser(t, db, email)` — inserts a user and returns the JWT token.
- `createTestChat(t, db, participants)` — creates a chat for test assertions.