# ECHO: getting started

How to run ECHO on your computer and check that it works.

## What you need

- Go 1.24.3 or newer
- Node 20 or newer, for the check scripts
- Docker, the easiest way to run MongoDB and Redis. You can also use your own.

Redis is required: the server exits at startup if it can't connect.

## Run it

**1. Get the code**

```bash
git clone https://github.com/shoonya0/ECHO.git
cd ECHO
git config core.hooksPath .githooks   # once per clone: turns on the pre-commit checks
```

**2. Start MongoDB and Redis**

```bash
docker compose -f docker/docker-compose.yml up -d
```

This starts `echo-mongodb` on port 27017 (user and password are both `echo`) and
`echo-redis` on port 6379.

**3. Create your settings file**

```bash
cp .env.example .env
```

The defaults work with the Docker setup above. Real environment variables override `.env`.

| Variable | Value in `.env.example` | Notes |
|---|---|---|
| `PORT` | `8080` | |
| `DB_URI` | `mongodb://echo:echo@localhost:27017` | Matches the Docker setup |
| `REDIS_URI` | `localhost:6379` | `host:port`, or a `redis://` or `rediss://` URL |
| `REDIS_PASS` | empty | Only used with the `host:port` form |
| `JWT_SECRET` | `echo-dev-secret` | **Required.** Use a long random value outside local development |
| `LOG_FILE` | `cmd/server/logs/server.log` | Leave empty to log to the screen |
| `LOG_LEVEL` | `debug` | Use `info` for deployment. `info` is also the default if unset |

**4. Start the server**

```bash
go run ./cmd/server
```

To build a program file instead:

```bash
go build -o build/echo-server ./cmd/server && ./build/echo-server
```

At startup the server also builds its MongoDB indexes and repairs old group owner roles
(#18). Both run in the background, so they don't delay the start.

**5. Check that it's alive**

```bash
curl http://localhost:8080/health
```

You should get `{"status":"ok"}`.

## Try it

```bash
# Create an account
curl -X POST http://localhost:8080/echo/v1/signup \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","username":"test_user","password":"testpass123"}'

# Log in. Your token is in data.token in the reply.
curl -X POST http://localhost:8080/echo/v1/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"testpass123"}'

# Open a live connection (first: npm install -g wscat)
wscat -c "ws://localhost:8080/echo/v1/ws/chat" -H "Authorization: Bearer <token>"
```

The live connection answers with a welcome message that has `"status":"connected"`.

## Reset local data

```bash
go run ./cmd/cleandb -dry-run   # show what would be deleted
go run ./cmd/cleandb            # delete all MongoDB collections and flush Redis
```

For local development only.

## Run the checks

| Command | What it does |
|---|---|
| `node scripts/verify.mjs` | The fast check: docs, build, vet, format, lint on new code, unit tests. Run it before saying a change is done. |
| `node scripts/verify.mjs --full` | The same, plus race tests. This is what CI runs. |
| `go test ./internal/services -run TestName -v` | Runs a single test |

**Database tests need MongoDB.** They use a throwaway database and drop it afterwards. They
connect to `TEST_DB_URI` (default: the Docker MongoDB). If MongoDB isn't reachable they are
skipped, and a skipped test proves nothing. With `TEST_DB_REQUIRED=1`, as in CI, they fail
instead of skipping. A few tests also need Redis, at `TEST_REDIS_URI` (default: the Docker
Redis, `localhost:6379`). They use Redis database 15 and the same skip rule.

### API contract test

This drives the REST API and the WebSocket with two throwaway users against a running
server, and records the shape of each reply. Compare two runs to check that a change
didn't break the Android app:

```bash
cd scripts/contract-test && npm install
node run.mjs http://localhost:8080 before.json   # before your change
node run.mjs http://localhost:8080 after.json    # after your change
node diff.mjs before.json after.json
```

### Load test

[`scripts/load-test`](../scripts/load-test) produces the numbers in
[product-overview.md](./product-overview.md#performance).

## Deployment

- The `Dockerfile` builds the server image.
- `render.yaml` describes the setup on Render: a web service plus Render Key Value for
  Redis. MongoDB runs on Atlas.
- **Merging to `main` deploys automatically**, so ask before merging.
- Render checks `GET /health` to see if the server is healthy.
- Redis on the free plan keeps data in memory only. After a restart the logout list is
  gone, so logged-out tokens work again until they expire.

## See what's running

- `GET /health`: is the server alive? No login needed.
- `GET /echo/v1/chats/hub/stats`: how many connections, chats and users are live. It needs
  the `X-Stats-Token` header set to the server's `STATS_TOKEN`. See
  [rest-api.md](./rest-api.md#hub-statistics).
