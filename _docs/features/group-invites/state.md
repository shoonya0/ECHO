# Group invites: state

## Now (updated 2026-11-18 at `89f9264`)

- **Status:** 🟡 in progress (increment 1 code and docs done, not pushed)
- **Current task:** 6 in [tasks.md](./tasks.md) (`/code-review` + `/security-review`, PR)
- **Stopped at:** `TEST_DB_REQUIRED=1 node scripts/verify.mjs --full` passed; live local
  run passed (pending member gets nothing live, accept starts live messages, 409 again,
  decline 200 then 404)
- **Open questions / blockers:** none
- **Don't forget:** a pending invite is stored as the invitee's `users.chats.<groupId>`
  entry. The backfill tells legacy pending members from new ones by the cutoff in the
  `migrations` doc `accept-legacy-pending-5`, never by the entry (code review).
  `websocket-requests.md` is at 200 lines: split it before adding to it

## Recent sessions (newest first, keep 5)

### 2026-11-18: routes, backfill, docs

- Changed: `group_invites.controller.go`, `chat.routes.go`, `migrations.go`, `main.go`, docs
- Verified: `verify --full` passed; live local accept/decline run passed
- Docs touched: REST groups, decisions, Android log, protocol, pub/sub, overview

### 2026-11-18: services for accept, decline, pending list

- Changed: `group_invites.go`, `chat.service.go` (invitee chats entry, `GetUserChats`, `finishGroupJoin`)
- Verified: `TEST_DB_REQUIRED=1 go test ./internal/...` → all ok
- Docs touched: tasks, state

### 2026-11-18: plan and docs

- Changed: feature docs only
- Verified: not verified (no code)
- Docs touched: this folder, registry row, Active work
