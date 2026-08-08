# Task Plan: Clean Up User Routes — Contacts Section (lines 33–50)

## Goal
Trace and refactor the contacts API chain (routes → controller → service) for clarity, correctness, and reduced boilerplate — behavior-preserving with targeted bug fixes.

## Current Phase
Phase 5

## Phases

### Phase 1: Requirements & Discovery
- [x] Read route definitions (lines 33–50 of user.routes.go)
- [x] Read contact.controller.go (437 lines)
- [x] Read contact.service.go (520 lines)
- [x] Read contact.model.go for type definitions
- [x] Trace all 11 endpoints through full chain
- **Status:** complete

### Phase 2: Planning & Structure
- [x] Identify boilerplate duplication (5 list handlers, 6 action handlers)
- [x] Identify context.Background() bug in service layer
- [x] Identify Projection naming violation
- [x] Identify BlockUnblock hardcoded "block" dead-end
- [x] Plan extractable helpers (authUserID, parseLimit, paramObjectID, contactsList)
- [x] Plan service-layer data table (contactFieldsByStatus)
- **Status:** complete

### Phase 3: Implementation — Controller
- [x] Extract shared helpers: authUserID, parseLimit, paramObjectID, contactsList
- [x] Collapse 5 list handlers to one-liners calling contactsList
- [x] De-duplicate 6 action handlers using authUserID + paramObjectID
- [x] Fix BlockUnblockUser to read ?action= query param
- [x] Fix misleading comment on AcceptOrDeclineContactRequest
- [x] Normalize error messages to lowercase
- **Status:** complete

### Phase 4: Implementation — Service
- [x] Replace dual switch in GetContacts with contactFieldsByStatus table
- [x] Replace context.Background() with ctx throughout
- [x] Fix Projection → projection naming
- [x] Fix return types (models.GetUserProfileResponse{} → nil)
- [x] Fix checkContactRequest if/else → switch
- [x] Add default case to BlockUnblockUser switch
- [x] Add defer cursor.Close(ctx) to GetContacts
- **Status:** complete

### Phase 5: Verification & Delivery
- [x] go build ./... — passed
- [x] go vet ./... — passed
- [x] Update planning files
- [x] Present summary
- **Status:** in_progress

## Key Questions
1. Should GetContactRequests (incoming) and GetSentContactRequests (outgoing) return different data? → Deferred (currently both return pendingIn+pendingOut combined — documented as known limitation)

## Decisions Made
| Decision | Rationale |
|----------|-----------|
| Extract authUserID helper | 22 uses across 11 handlers |
| Extract contactsList generic helper | Eliminated ~200 lines of duplicate boilerplate |
| Extract paramObjectID helper | 10 instances of param→ObjectID conversion |
| Use table-driven status→fields mapping | Eliminated 60-line dual switch |
| BlockUnblock defaults to "block" when ?action absent | Maintains backward compatibility |
| contactFieldsByStatus uses ContactInfoEmbed | Matches actual model type |

## Errors Encountered
| Error | Attempt | Resolution |
|-------|---------|------------|
| models.ContactInfoIDs undefined | 1 | Checked contact.model.go — actual type is ContactInfoEmbed |

## Notes
- ApiBasePath = `/echo/v1/`
- All 11 contact endpoints are authenticated
- Route file (user.routes.go) was already clean from previous pass — only controller + service changed in this pass