# Group invites: state

## Now (updated 2026-11-18 at `f022f2c`)

- **Status:** 🟡 in progress
- **Current task:** 3 in [tasks.md](./tasks.md) (REST routes)
- **Stopped at:** services done and tested (`internal/services/group_invites.go`); no routes yet
- **Open questions / blockers:** none. Decided with the user: REST only for now; legacy
  pending members are auto-accepted
- **Don't forget:** a pending invite is stored as the invitee's `users.chats.<groupId>`
  entry, so the backfill tells legacy pending members (no entry) from new ones (entry)

## Recent sessions (newest first, keep 5)

### 2026-11-18: services for accept, decline, pending list

- Changed: `group_invites.go`, `chat.service.go` (invitee chats entry, `GetUserChats`, `finishGroupJoin`)
- Verified: `TEST_DB_REQUIRED=1 go test ./internal/...` → all ok
- Docs touched: tasks, state

### 2026-11-18: plan and docs

- Changed: feature docs only
- Verified: not verified (no code)
- Docs touched: this folder, registry row, Active work
