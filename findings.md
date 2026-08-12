# Findings: Chat Routes Cleanup

## Discovery Date
2026-08-08

## API Path Trace (12 live routes)

### Chat Management (`/echo/v1/chats`)
| # | Method | Path | Controller |
|---|--------|------|------------|
| 1 | POST | `/chats/direct/:userId` | `CreateDirectChatHTTP` |
| 2 | POST | `/chats/group` | `CreateGroupChatHTTP` |
| 3 | GET | `/chats/hub/stats` | `GetHubStatsHTTP` |

### Messaging (`/echo/v1/messages`)
| # | Method | Path | Controller |
|---|--------|------|------------|
| 4 | GET | `/messages/:chatID/messages` | `GetChatMessagesHTTP` |

### Group Management (`/echo/v1/groups`)
| # | Method | Path | Controller |
|---|--------|------|------------|
| 5 | POST | `/groups/:groupID/add-members` | `AddGroupMembersHTTP` |
| 6 | GET | `/groups/invites` | `GetAllInvitesOfUser` |
| 7 | POST | `/groups/join/:inviteCode` | `JoinGroupByInvite` |
| 8 | DELETE | `/groups/invites/:inviteID` | `DeleteInvite` |
| 9 | GET | `/groups/invites/:inviteID/joined` | `GetJoinedUsersByInvite` |
| 10 | POST | `/groups/invites/:inviteID/:userID/send` | `SendInviteToUser` |
| 11 | POST | `/groups/:groupID/invites` | `CreateInvite` |
| 12 | GET | `/groups/:groupID/invites` | `GetGroupInvites` |

## Issues Found

### 1. ~200 lines of dead commented code
The original file had 206 lines; only 12 routes are live. Everything else was commented-out scaffolding: 7 message operations, group CRUD, channels, media, presence, notifications, search, moderation, admin, plus a fully commented-out `RegisterWebSocketRoutes` (a live one already exists in `routes.go`). **Removed entirely.**

### 2. Trailing slash on DELETE invite route
`DELETE /invites/:inviteID/` was the only route in the codebase with a trailing slash. Fixed to `/invites/:inviteID`.

### 3. Route ordering: `:groupID` before static invite routes
The original registered `:groupID/*` routes before static routes (`/invites`, `/join/:inviteCode`). This violates the project's convention (established in the user.routes.go cleanup: "Static routes MUST be registered before /:id"). **Reordered.**

### 4. Misleading "legacy" label
`GET /:chatID/messages` was labeled "Group Messages (legacy endpoint for backward compatibility)". It handles all chat types (direct + group). Fixed comment.

### 5. Misplaced TODO note
```
// remaining :- when sending an message to an unknown user then we have to autojoin both the users insted of one.
```
This product note about `CreateDirectChat` auto-join behavior was sitting at the top of a route-registration function with grammar errors. **Removed from routes file; recorded here instead.**

### 6. Redundant section banners wrapping mostly dead code
Section banners like `============ GROUP MANAGEMENT ROUTES ============` wrapped sections that were ~90% commented out. Simplified.

## Documented (Not Changed — Behavior-Preserving)
- **Redundant `messages` path segment:** `GET /messages/:chatID/messages` repeats `messages` — changing it would break the API contract.
- **Mixed terminology:** `chats/:chatID` vs `groups/:groupID` for the same underlying entity — out of scope for a routes-file cleanup.
- **`GET /groups/invites` scoped under `groups`:** semantically user-level, not group-level — but changing the mount point would break the API contract.

## Previous Task (User Routes Cleanup — Complete)
- Cleaned `internal/api/user.routes.go` (lines 33–50): static-before-param ordering, renamed `userRoutes` → `discoveryRoutes`, consistent kebab-case, concise comments
- Controller-level deduplication: extracted `authUserID`, `parseLimit`, `paramObjectID`, `contactsList` helpers
- Service-level fixes: `context.Background()` → `ctx`, `Projection` → `projection`, data-table pattern for `GetContacts`

## Postman Route Test Remediation (2026-08-10)

### Root Cause: Self-deleting test user
The Postman collection run flow was:
1. POST /signup → 409 (user existed from prior test, ok)
2. POST /login → 200 (JWT issued)
3. GET /profile/ → 200
4. PUT /profile/ → 200
5. **DELETE /profile/delete → 200** (user hard-deleted from MongoDB)
6. GET /users/:id → 404 (`mongo: no documents in result` — user gone)
7. All contact GET routes → 500 (`mongo: no documents in result` — user gone)

### Findings
1. **`AuthMiddleware` is claims-based**: JWT tokens carry all user claims inline. No DB existence check on each request. So after the user is deleted, the token stays "valid" but every DB lookup by `_id` fails.
2. **`GetContacts` missing `ErrNoDocuments` branch**: When the user document doesn't exist, `FindOne` returns `mongo.ErrNoDocuments`. The function returned the raw error, which `MapServiceErrorToHTTP` mapped to 500 (its default). Per the API contract and `error-handling.md` rule 6, no data should render as 200 with empty items array, not 500.
3. **`DELETE /profile/delete` is destructive**: `DeleteProfile` calls `DeleteOne` which permanently removes the user document from MongoDB. As a test, it must run last or the collection must re-create the user before subsequent tests.

### Fixes Applied
1. **`contact.service.go:84-86`** — Added `mongo.ErrNoDocuments` branch returning `([]models.ContactInfo{}, nil)` for 200 empty response
2. **Postman collection** — Reordered folders: Auth → Users → Contacts → Chats → Messages → Groups → **Profile** → WebSocket. Profile (with destructive delete) now runs second-to-last
3. **Planning files** — Updated task_plan.md, findings.md, progress.md

---

## WebSocket Panic Fix (2026-08-10 11:56)

### Problem: `panic: repeated read on failed websocket connection`
Gorilla WebSocket panics when `ReadMessage()` is called on a socket that has already returned an error. The `handleClientRead` loop in `websocket.controller.go:176-203` treated **all non-unexpected-close errors as parse errors** and called `continue` — re-reading the dead socket and triggering the panic.

### Trigger
Android app (okhttp) connections dying abruptly (network drop / background kill / 60s read deadline) — `ReadMessage()` returns a non-standard close error → code takes `else` branch → `continue` → panic.

### Fix: `websocket.controller.go:176-190`
Simplified the read-error branch: **any read error = connection dead → always `break`**. JSON `Unmarshal` failures (which happen after a *successful* read) still `continue` safely.

```go
// Before: else branch → send PARSE_ERROR → continue → panic on re-read
// After:  always break on any read error; only json.Unmarshal failures continue
if err != nil {
    log.Printf("WebSocket read failed for client %s: %v", client.ID, err)
    break
}
```

Build + vet passed. Server restart required for the fix to take effect.
