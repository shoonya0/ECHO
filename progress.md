# Progress: Chat Routes Cleanup

## Session: 2026-08-08 — Remove Dead Code & Clean Up

### Phase 1 — Discovery (complete)
- Loaded planning-with-files, golang-gin, golang-code-style skills
- Read original `internal/api/chat.routes.go` (206 lines)
- Read `routes.go` (wiring) and `user.routes.go` (style reference from prior cleanup)
- Traced all 12 live handlers against `chat.controller.go` — all verified
- Inventoried 6 issues (findings.md)

### Phase 2 — Planning (complete)
- Decided to delete all commented future-route sections (~200 lines)
- Static invite routes before `/:groupID` routes (project convention)
- No path/method/handler changes — behavior-preserving

### Phase 3 — Implementation (complete)
- Rewrote `internal/api/chat.routes.go` (206 → 50 lines, 12 live routes only)
- Removed trailing slash from DELETE /invites/:inviteID
- Reordered: static invite routes before `/:groupID` routes
- Clean comments: concise, consistent with `user.routes.go` style; fixed "legacy" label; removed misplaced TODO note
- Deleted dead sections: message ops, group CRUD, channels, media, presence, notifications, search, moderation, admin, commented RegisterWebSocketRoutes

### Phase 4 — Verification (complete)
- `go build ./...` → passed
- `go vet ./...` → passed

### Files Changed
- `internal/api/chat.routes.go` — rewritten (206 → 50 lines)

### Files Updated
- `task_plan.md` — new task plan for chat routes
- `findings.md` — discovery findings + API table + previous task summary
- `progress.md` — this file

## Summary of Changes

| Before | After |
|--------|-------|
| 206 lines, ~200 dead | 50 lines, 12 live routes only |
| Dead sections: message ops, CRUD, channels, media, presence, notifications, search, moderation, admin | All removed |
| Commented-out `RegisterWebSocketRoutes` (duplicate of live in routes.go) | Removed |
| `DELETE /invites/:inviteID/` (trailing slash) | `/invites/:inviteID` |
| `/:groupID` routes before static invite routes | Static invite routes (`/invites`, `/join`) before `/:groupID` |
| "Group Messages (legacy endpoint)" label | "Chat messages with pagination" |
| Misplaced grammar-typo TODO note | Removed (recorded in findings.md) |
| Redundant section banners | Simplified, clean architecture |

---

## Session: 2026-08-10 — Postman Route Test Remediation

### Phase 1 — Discovery (complete)
- Read `logs/server.log` — confirmed `DELETE /profile/delete` (step 5) deletes the test user mid-run
- Root cause: JWT is claims-based (no per-request DB check in `AuthMiddleware`), so token stays valid after delete, but all DB lookups by `_id` fail with `mongo: no documents in result`
- `GetContacts` missing `mongo.ErrNoDocuments` branch → 500 instead of 200 empty list

### Phase 2 — Planning (complete)
- Add `mongo.ErrNoDocuments` guard in `GetContacts` for 200 empty response
- Reorder Postman collection: Profile folder to end (delete last)

### Phase 3 — Implementation (complete)
- `internal/services/contact.service.go:84-86` — Added `if err == mongo.ErrNoDocuments { return []models.ContactInfo{}, nil }` branch
- Postman collection — Reordered: Auth → Users → Contacts → Chats → Messages → Groups → Profile → WebSocket. Profile description updated: "RUNS LAST (delete destroys user)"

### Phase 4 — Verification (complete)
- `go build ./...` → passed
- `go vet ./...` → passed
- Postman collection re-run → 32/33 passed (WebSocket 400 expected)

### Files Changed
- `internal/services/contact.service.go` — added `ErrNoDocuments` branch in `GetContacts`
- `task_plan.md` — rewritten for this task
- `findings.md` — appended Postman Route Test Remediation section
- `progress.md` — this session log

### Postman Artifacts Updated
- "ECHO API" collection — folder order reorganized (Profile last)

---

## Session: 2026-08-10 — WebSocket Panic Fix

### Phase 1 — Discovery (complete)
- Android app (okhttp) connections dying abruptly triggered `panic: repeated read on failed websocket connection` in `handleClientRead`
- Gorilla WebSocket hard-panics when `ReadMessage()` is called on a socket with an existing `readErr`
- The `else` branch (non-unexpected close errors) sent a `PARSE_ERROR` and `continue`d — re-reading the dead socket
- Per error-handling.md: "Anything that panics is a bug"

### Phase 2 — Planning (complete)
- Any read error = connection dead → always `break`, never `continue`
- JSON `Unmarshal` failures (post-successful-read) still `continue` safely
- `defer` at top of `handleClientRead` already handles unregister + close cleanup on `break`

### Phase 3 — Implementation (complete)
- `websocket.controller.go:176-191` — simplified: all read errors → `break`. Removed the else/continue/send-PARSE_ERROR path. JSON decode errors still `continue`.

### Phase 4 — Verification (complete)
- `go build ./...` → passed
- `go vet ./...` → passed
- Server restart required for fix to take effect

### Files Changed
- `internal/controller/websocket.controller.go` — lines 176-191 rewritten
- `findings.md` — appended WebSocket Panic Fix section
- `progress.md` — this session log
