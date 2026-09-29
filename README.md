# ECHO — Real-Time Chat Server

A Go backend for a real-time chat app (Android client): REST API for accounts, contacts, chats and groups, plus a WebSocket hub for live messaging, typing indicators, read receipts and reactions. MongoDB stores data; Redis handles token revocation and pub/sub, so several server instances can share chat rooms.

**Stack:** Go · Gin · gorilla/websocket · MongoDB · Redis · JWT

> Protocol and architecture docs live in [`_docs/`](./_docs/README.md).

## Quick start

```bash
# 1. Start MongoDB + Redis
docker compose -f docker/docker-compose.yml up -d

# 2. Configure (defaults already match docker-compose)
cp .env.example .env

# 3. Run
go run ./cmd/server
```

Health check: `GET http://localhost:8080/health`

## Project layout

```
cmd/
  server/        entrypoint: config → logger → Mongo/Redis → hub → HTTP server
  cleandb/       dev tool: wipe all collections and flush Redis
internal/
  config/        settings from env vars (+ optional .env file)
  constants/     collection names, API base path, enums, context keys
  database/      MongoDB and Redis connections
  logger/        JSON logger with request/transaction IDs
  router/        route registration (URLs → controllers)
  middleware/    JWT auth, CORS, request logging
  controller/    HTTP + WebSocket handlers: parse request, call service, write response
  services/      business logic and MongoDB queries
  realtime/      WebSocket hub, Redis pub/sub fan-out, presence
  models/        MongoDB documents and request/response types
  utils/         JWT signing, response envelope, model defaults
scripts/
  contract-test/ end-to-end REST + WebSocket contract test
docker/          docker-compose for local MongoDB + Redis
```

A request flows **router → middleware → controller → services → database**. WebSocket events go **controller → services → realtime hub → Redis pub/sub → every connected client in the chat**.

## Configuration

All settings are environment variables; `.env` is only a local convenience.

| Variable     | Default                              | Notes                                         |
| ------------ | ------------------------------------ | --------------------------------------------- |
| `PORT`       | `8080`                               |                                               |
| `DB_URI`     | `mongodb://echo:echo@localhost:27017` | Any MongoDB URI (e.g. Atlas)                  |
| `REDIS_URI`  | `localhost:6379`                     | `host:port` or `redis://` / `rediss://` URL   |
| `REDIS_PASS` | —                                    | Only for `host:port` form                     |
| `JWT_SECRET` | — (required)                         |                                               |
| `LOG_FILE`   | — (stdout)                           | Set to a path to log to a file                |
| `LOG_LEVEL`  | `debug`                              | `debug`, `info`, `warn`, `error`              |

## API overview

Base path: `/echo/v1/`. Everything except `signup` and `login` requires `Authorization: Bearer <token>`.

| Area     | Endpoints |
| -------- | --------- |
| Auth     | `POST signup`, `POST login`, `POST auth/logout` |
| Profile  | `GET/PUT profile/`, `DELETE profile/delete` |
| Users    | `GET users/:id`, `users/suggestions`, `users/nearby`, `users/popular` |
| Contacts | `users/contacts/…` — list, requests, sent-requests, blocked, favorites, send/accept/remove, block/unblock, favorite |
| Chats    | `POST chats/direct/:userId`, `POST chats/group`, `GET chats/hub/stats` |
| Messages | `GET messages/:chatID/messages?limit=&offset=` |
| Groups   | `groups/:groupID/add-members`, `groups/:groupID/invites`, `groups/invites…`, `groups/join/:inviteCode` |
| Realtime | `GET ws/chat` (WebSocket) — see [`_docs/WEBSOCKET_PROTOCOL.md`](./_docs/WEBSOCKET_PROTOCOL.md) |

## Testing

The contract test drives every REST endpoint and WebSocket message type against a running server. It records status codes and response shapes, so you can check that a change didn't break the Android client:

```bash
cd scripts/contract-test && npm install
node run.mjs http://localhost:8080 before.json   # before a change
node run.mjs http://localhost:8080 after.json    # after
node diff.mjs before.json after.json
```

## Docker

```bash
docker build -t echo-server .
docker run -p 8080:8080 -e DB_URI=... -e REDIS_URI=... -e JWT_SECRET=... echo-server
```
