# Task Plan: Postman Route Test Remediation

## Goal
Fix the two classes of failing routes discovered in the Postman test run:
1. **Contact GET routes return 500** for users with no `contactInfo` document (fresh or deleted users)
2. **`GET /users/:id` returns 404** because the test run self-deletes the user mid-run via `DELETE /profile/delete`

## Current Phase
Phase 5 (Delivery)

## Phases

### Phase 1: Discovery
- [x] Read `logs/server.log` — confirmed `DELETE /profile/delete` deletes the user in step 5; all subsequent DB lookups fail with `mongo: no documents in result`
- [x] Read `internal/services/contact.service.go` — confirmed `GetContacts` missing `mongo.ErrNoDocuments` branch (rule 6 in error-handling.md)
- [x] Read `internal/middleware/auth.go` — confirmed JWT is claims-based (no per-request DB existence check)
- [x] Read `internal/controller/controller.go` — confirmed `MapServiceErrorToHTTP` default is 500
- [x] Read `error-handling.md` rule — confirmed expected behavior is 200 with empty arrays for no data
- **Status:** complete

### Phase 2: Planning
- [x] `GetContacts`: add `if err == mongo.ErrNoDocuments { return []models.ContactInfo{}, nil }` branch
- [x] Postman: reorder Profile folder to run last (DELETE destroys user, so must be after all other tests)
- [x] Update planning files (task_plan.md, findings.md, progress.md)
- **Status:** complete

### Phase 3: Implementation
- [x] Add `mongo.ErrNoDocuments` guard in `GetContacts` → returns `200 []` instead of `500`
- [x] Reorder Postman collection: Profile folder moved from position 2 to position 7 (before WebSocket)
- **Status:** complete

### Phase 4: Verification
- [x] `go build ./...` — passed
- [x] `go vet ./...` — passed
- [x] Run Postman collection — 32/33 passed (WebSocket 400 expected)
- **Status:** complete

### Phase 5: Delivery
- [x] Update planning files (task_plan.md, findings.md, progress.md)
- [x] Present summary
- **Status:** in_progress

## Decisions Made
| Decision | Rationale |
|----------|-----------|
| `GetContacts` returns empty slice on `ErrNoDocuments`, not an error | Rule 6 of error-handling.md: "For FindByFilter[T] which returns (T, error): mongo.ErrNoDocuments → return zero value + error." But GetContacts returns `([]T, error)` — per API contract, 200 with empty items is correct |
| Postman Profile folder moved to penultimate position (before WebSocket) | DELETE /profile/delete hard-deletes the authenticated user from MongoDB — all subsequent authenticated lookups fail. Running it last protects all other tests |
| `GET /users/:id` 404 is correct behavior post-delete | User genuinely doesn't exist in DB; JWT is claims-based so auth succeeds but find fails. This is not a code bug |

## Notes
- `AuthMiddleware` performs no DB existence check — claims-based JWT design. Adding a per-request DB check would be an architectural change, out of scope
- All 400/404 responses for empty path params (`:userId`, `:chatId`, etc.) are correct input validation and stay as-is
- WebSocket GET returning 400 is expected — Postman isn't a WebSocket client