# Group invites: state

## Now (updated 2026-11-19 at `45d90af`)

- **Status:** 🟡 increment 1 merged; increment 2 waits for the Android invite screen
- **Current task:** none here. #110 is merged (PR #156): the
  pending list computes `participantCount` from `participants`, and decline no longer
  lowers a stored count
  ([decided](../../decisions/chats-and-groups.md#how-a-groups-participant-count-is-kept-110))
- **Stopped at:** `node scripts/verify.mjs` and `TEST_DB_REQUIRED=1 go test ./...` passed
- **Open questions / blockers:** none
- **Don't forget:** a pending invite is stored as the invitee's `users.chats.<groupId>`
  entry. The backfill tells legacy pending members from new ones by the cutoff in the
  `migrations` doc `accept-legacy-pending-5`, never by the entry (code review).
  `websocket-requests.md` is at 200 lines: split it before adding to it

## Recent sessions (newest first, keep 5)

### 2026-11-19: #110 (participant count computed on read)

- Changed: `group_invites.go`, `chat.service.go`, `account_delete.go`, `migrations.go`, `main.go`
- Verified: `verify` passed; `TEST_DB_REQUIRED=1 go test ./...` all ok
- Docs touched: this file, decisions/chats-and-groups, current-state

### 2026-11-19: #37 step 2b-2 (accept, decline, join in transactions)

- Changed: `group_invites.go`, `chat.service.go` (`joinOutcome`, `finishGroupJoin`)
- Verified: `verify --full` passed; rollback tests with `testutil.FailCommand`
- Docs touched: this file, decisions/data-writes, current-state

### 2026-11-19: #37 step 2b-1 (group writes in transactions)

- Changed: `group_invites.go` (`giveChatEntry`), `chat.service.go`
- Verified: `verify --full` passed; rollback tests with `testutil.FailCommand`
- Docs touched: this file, decisions/data-writes, current-state

### 2026-11-18: routes, backfill, docs

- Changed: `group_invites.controller.go`, `chat.routes.go`, `migrations.go`, `main.go`, docs
- Verified: `verify --full` passed; live local accept/decline run passed
- Docs touched: REST groups, decisions, Android log, protocol, pub/sub, overview

### 2026-11-18: services for accept, decline, pending list

- Changed: `group_invites.go`, `chat.service.go` (invitee chats entry, `GetUserChats`, `finishGroupJoin`)
- Verified: `TEST_DB_REQUIRED=1 go test ./internal/...` → all ok
- Docs touched: tasks, state
