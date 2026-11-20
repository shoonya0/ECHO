# ECHO

ECHO is the server for a real-time chat app: a REST API for accounts, contacts, chats and
groups, and a WebSocket for live messages, typing, read receipts, reactions and presence.
The client is an Android app, which lives in its own repository,
[ECHO-app](https://github.com/shoonya0/ECHO-app). This repository holds the server only;
`Android/` holds the app's plans and the log of server changes it needs to follow.

## What works today

- Accounts: sign up, log in (a JWT that lasts 7 days), log out
- Profiles: view, edit and delete your own; view other people's
- Contacts: requests, accept and decline, remove, favorites, block and unblock
- Direct chats and group chats, with owner, admin and member roles
- Group invite codes: create, list, delete, send, join with a code; accept or decline
- Live messages, message history (page by page, or catch up after a `seq`)
- Typing indicators, read receipts, emoji reactions, online status
- Attachments as links (no file upload)

Known gaps and what isn't built yet (editing or deleting messages, a chat list route, file
upload, push notifications and more) are in
[product-overview.md](_docs/product-overview.md#what-works-today).

## Tech stack

- Go 1.24.3+ with [Gin](https://github.com/gin-gonic/gin) and
  [gorilla/websocket](https://github.com/gorilla/websocket)
- MongoDB as a one-node replica set (the server needs transactions and won't start on a
  standalone server)
- Redis for the logout list, the account-access cache and pub/sub between server
  instances. The server won't start without it.
- Docker image, deployed to Render (`render.yaml`). Every merge to `main` deploys.

## Quick start

You need Go 1.24.3 or newer, Node 20 or newer (for the check scripts) and Docker.

```bash
cp .env.example .env                              # defaults match the Docker setup
docker compose -f docker/docker-compose.yml up -d # MongoDB on 27017, Redis on 6379
go run ./cmd/server                               # listens on PORT (8080)
curl http://localhost:8080/health                 # {"status":"ok"}
```

Logs go to `LOG_FILE` from `.env` (`cmd/server/logs/server.log` in `.env.example`); leave it
empty to log to stdout. Every setting is an environment variable, and real environment
variables override `.env`. `JWT_SECRET` is required; use a long random value anywhere but
local development.

More, including a signup/login/WebSocket walkthrough with `curl` and `wscat`, resetting
local data and the replica-set notes: [getting-started.md](_docs/getting-started.md).

## Checks

```bash
git config core.hooksPath .githooks   # once per clone: pre-commit docs-check, gofmt, vet
node scripts/verify.mjs               # fast: docs-check, build, vet, gofmt, lint on new code, unit tests
node scripts/verify.mjs --full        # the same plus go test -race ./... (what CI runs)
go test ./internal/services -run TestName -v
```

Database tests use MongoDB at `TEST_DB_URI` (default: the Docker MongoDB) and skip when it
isn't reachable. A skipped test proves nothing. With `TEST_DB_REQUIRED=1`, as in CI, they
fail instead. Redis tests use `TEST_REDIS_URI` (default `localhost:6379`) with the same rule.
The API contract test and the load test need a running server:
[getting-started.md](_docs/getting-started.md#run-the-checks).

## API at a glance

- **REST:** every route is under `/echo/v1/`. All of them except `signup`, `login` and
  `/health` need an `Authorization: Bearer <token>` header. Reference:
  [rest-api/](_docs/rest-api/README.md).
- **WebSocket:** `GET /echo/v1/ws/chat`. The token goes in the same `Authorization: Bearer`
  header and nowhere else (not in the URL, not in a cookie). Contract, requests, events and
  error codes: [WEBSOCKET_PROTOCOL.md](_docs/WEBSOCKET_PROTOCOL.md).

## Project layout

```
cmd/server/          entry point: config, logger, MongoDB, Redis, hub, HTTP server
cmd/cleandb/         dev tool: wipe MongoDB collections and flush Redis
internal/config/     settings from the environment and an optional .env
internal/constants/  collection names, API base path, statuses, roles
internal/router/     route registration
internal/middleware/ token and account check, CORS, rate limits, request logging
internal/controller/ HTTP and WebSocket handlers
internal/services/   business rules, MongoDB queries, startup data repairs
internal/realtime/   WebSocket hub, Redis pub/sub, presence
internal/models/     MongoDB documents, API and WebSocket payloads
internal/database/   MongoDB and Redis connections, indexes, transactions
internal/logger/     JSON logging with request IDs
internal/utils/      tokens, reply helpers, defaults
internal/testutil/   shared test helpers (throwaway test DB, test Redis)
scripts/             verify, docs-check, contract test, load test, audits
docker/              docker-compose for local MongoDB and Redis
_docs/               project docs (start at _docs/README.md)
Android/             the Android app's plans and the server-changes log
rules and skills/    project coding rules
```

## Documentation

Each fact has one home; the README links to it rather than repeating it.

| Need | Read |
|---|---|
| Docs index and feature registry | [_docs/README.md](_docs/README.md) |
| What works, gaps, security, performance | [product-overview.md](_docs/product-overview.md) |
| Where the project stands, active work, test baseline | [current-state.md](_docs/current-state.md) |
| Packages, flows, data model, riskiest areas | [architecture-overview.md](_docs/architecture-overview.md) |
| Pub/sub and hub internals | [PUBSUB_ARCHITECTURE.md](_docs/PUBSUB_ARCHITECTURE.md) |
| Decisions and what they unblock | [decisions/](_docs/decisions/README.md) |
| Terms | [glossary.md](_docs/glossary.md) |
| Server changes the Android app must follow | [Android/_docs/08_SERVER_CHANGES/](Android/_docs/08_SERVER_CHANGES/README.md) |

Bugs are tracked as [GitHub issues](https://github.com/shoonya0/ECHO/issues).

## Contributing

- Branch from an up-to-date `main`: `feature/<name>`, `fix/<issue#>-<name>`, `refactor/`,
  `docs/` or `chore/`. One branch, one change.
- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/):
  `type(scope): summary`, a body that says why, and `Fixes #N` when there is an issue.
- Run `node scripts/verify.mjs` before opening a pull request to `main`. CI runs
  `--full` with MongoDB and Redis.
- Merging to `main` deploys to production on Render, so pull requests are merged by the
  maintainer.

This repository has no license file yet.
