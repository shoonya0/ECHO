# Dev deployment — ECHO

## Rule

1. **Run locally with `go run .`** — no Docker required for development. Dependencies: MongoDB (required), Redis (optional — presence features disabled without it).
2. **Config via env file** at `config/envConfig/dev.env`. Never commit `dev.env` to git. A `dev.env.example` template should be committed instead.
3. **Build** with `go build -o build/echo-chat .` for a binary. The binary reads the same `config/envConfig/dev.env` at runtime.
4. **MongoDB collections are created implicitly** by the mongo driver on first insert. No migration step needed.
5. **Logs** go to `logs/server.log` (truncated on each restart). In development, set level to `Debug` for verbose output.

## Why

- Zero Docker required for local dev keeps the feedback loop fast (`go run .`).
- Env config via Viper + `.env` files means no code changes between dev and production — just swap the env file.
- MongoDB's schema-flexible nature means no migration tooling is needed.

## How to apply

### 1. First-time setup

```bash
# 1. Clone and navigate
cd ECHO

# 2. Install dependencies
go mod download

# 3. Create the env config file
mkdir -p config/envConfig
cp config/envConfig/dev.env.example config/envConfig/dev.env
# Edit dev.env with your values

# 4. Ensure MongoDB is running
mongosh --eval "db.runCommand({ping: 1})"

# 5. Run the server
go run .
```

### 2. Required env vars (`config/envConfig/dev.env`)

```
PORT=8080
HOST=localhost
DB_URI=mongodb://localhost:27017
DB_USER_NAME=
REDIS_URI=redis://localhost:6379
REDIS_PASS=
JWT_SECRET=<generate-a-random-64-character-string>
AWS_REGION=
AWS_ACCESS_KEY=
AWS_SECRET_ACCESS_KEY=
AWS_S3_BUCKET=
AUTH_SERVICE_URL=
```

`JWT_SECRET` is the only truly required secret. Generate one with:
```bash
openssl rand -hex 32
```

AWS fields are future placeholders — not currently used.

### 3. Running the server

```bash
# Default port :8080
go run .

# Custom port
go run . -port :9090

# Custom config path
go run . -config /path/to/config/dir
```

Flags:
- `-port` — override the listen port (default `:8080`)
- `-config` — path to directory containing `dev.env` (default `config/envConfig/`)
- `-version` — print version (default `true`)

### 4. Development workflow

```bash
# Watch mode (requires air or similar)
air

# Or just re-run on changes
go run .

# Run tests
go test ./...

# Run with race detector
go test -race ./...

# Run integration tests (requires MongoDB)
go test -tags=integration ./...

# Build binary
go build -o build/echo-chat .

# Run binary
./build/echo-chat
```

### 5. MongoDB setup

The application connects to MongoDB using the URI from `DB_URI`. The database name is `Echo` (hardcoded as `objects.DBName`).

Collections are created automatically on first insert:
- `users` — user accounts, profiles, presence, contacts
- `chats` — direct/group/channel chat metadata
- `messages` — chat messages

No manual collection or index creation is needed for development. For production, create indexes on frequently queried fields (see `mongodb-access.md` §10).

### 6. Redis setup (optional)

Redis is used for:
- WebSocket pub/sub (cross-instance message delivery)
- Presence persistence (survives server restarts)

If Redis is unavailable, the server logs a warning and starts without WebSocket multi-instance support:

```go
if err := db.ConnectRedis(ctx); err != nil {
    log.WithError(err).Warn("Failed to connect to Redis - presence features will be disabled")
}
```

The WebSocket hub still works for single-instance use without Redis — messages are delivered in-process. Presence sync to Redis is skipped.

### 7. Logs

```
logs/server.log    — JSON-formatted application logs
```

Log level is `Debug` by default (set in `main.go` `init()`):
```go
Level = logrus.DebugLevel
```

The log file is truncated on each server start. All old log files in the directory are cleaned before creating a new one.

### 8. Building for production

```bash
# Build optimized binary
go build -ldflags="-s -w" -o build/echo-chat .

# The binary reads config/envConfig/dev.env at runtime
# Set the env file path for production:
./build/echo-chat -config /etc/echo/config
```

### 9. WebSocket client testing

Two HTML test files are included:
- `test-websocket-chat.html` — general WebSocket chat testing
- `test-group-chat.html` — group chat features testing

Open in a browser, connect with a JWT token, and test real-time messaging.

### 10. What NOT to do

- **Don't commit `dev.env`** — it's in `.gitignore`. Only commit `dev.env.example`.
- **Don't hardcode ports** — use the `-port` flag or `PORT` env var.
- **Don't run migrations manually** — MongoDB creates collections implicitly.
- **Don't depend on Redis for local dev** — the server starts fine without it.
- **Don't use Docker unless you need production-like isolation** — `go run .` is sufficient.