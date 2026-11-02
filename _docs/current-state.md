# ECHO: current state (snapshot 2026-11-01)

Snapshot taken at `e1291b2` on `chore/ai-harness` (branched from `main`). One page.
Architecture: [architecture-overview.md](./architecture-overview.md).

## Active work

<!-- Read by the SessionStart hook (session-context.mjs): keep this section first, short,
and with a link to exactly one feature state.md. Updated by /close-increment and
/feature-docs update. -->

- **Feature:** none yet (set by `/feature-docs start`) · AI harness setup
- **Next step:** fix #18 (group creators get role "member"), then #19 (hub map races)
- **Updated:** 2026-11-01 at `e1291b2`

## What a user can do today (VERIFIED by reading the code)

- Sign up, log in (7-day JWT), log out (token blacklisted in Redis).
- Read and update their profile; look up other users; get suggestions.
- Contacts: send/accept/decline requests, remove, block/unblock, favorites.
- Direct chats and group chats; send messages over the WebSocket; cursor-paged history.
- Typing, read receipts and reactions over the WebSocket.
- Group invite codes: create, list, delete, send to a user, join.
- **Broken:** adding/removing group members fails for everyone (#18).
- Not implemented: edit/delete message, update chat (`NOT_IMPLEMENTED`).

## How to build, run, test

- Local services: `docker compose -f docker/docker-compose.yml up -d` (MongoDB 7, Redis 7)
- Config: `cp .env.example .env` (ASSUMED: the defaults match docker-compose)
- Run: `go run ./cmd/server` (port 8080, `GET /health`)
- Fast check: `node scripts/verify.mjs` · Full (adds `-race`): `node scripts/verify.mjs --full`
- DB tests run only when MongoDB is reachable; otherwise they skip (VERIFIED: 2 skips).

## Test baseline (at `e1291b2`, 2026-11-01, Windows, no MongoDB/Redis)

| Command | Duration | Result | Known failures (by name) |
|---|---|---|---|
| `go build ./...` | 16s | pass | — |
| `go vet ./...` | 3s | pass | — |
| `gofmt -l cmd internal` | <1s | pass (no files) | — |
| `go test ./...` | 2s | pass; skipped: `TestAddGroupMemberAuthorization`, `TestGetChatMessagesRequiresMembership` (no DB) | — |
| `go test -race ./...` | 14s | pass (same skips) | — |
| `golangci-lint run ./...` (v2.14) | 20s | **28 issues**: errcheck 21, staticcheck 7 | not blocking: verify lints only new code (`--new-from-merge-base=origin/main`) |

Compare new runs by test name. The race detector passes only because no test exercises
the hub concurrently; #19 has a reproducer.

## Open decisions

| # | Decision needed | Options | Blocking? |
|---|---|---|---|
| 1 | Single source of truth for chat membership | `participants` only / keep `users.chats` in sync | #14, #17 |
| 2 | Block model | one-sided `blocked` + `blockedBy` | #26 |
| 3 | Account deletion | soft delete + anonymize / hard delete + cleanup | #35 |
| 4 | Mongo transactions | require replica set (Atlas yes, local compose no) | #37 |

## Known issues and risks

Open issues: #5, #9, #14–#17 (filed earlier) and #18–#43 (code review 2026-11-01). The
highest priority: #18, #19, #23, #26, #34. Map: [architecture-overview.md](./architecture-overview.md#riskiest-areas).

## Next task

- Fix #18 on `fix/18-group-owner-role` with a DB test that creates a group through
  `CreateGroupChat` and adds a member as the creator.
