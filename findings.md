# Findings: User Routes Cleanup

## Discovery Date
2026-08-08

## API Path Trace

### Profile Management (`/echo/v1/profile`)
| # | Method | Path | Controller | Service | DB |
|---|--------|------|------------|---------|-----|
| 1 | GET | `/profile/` | `GetProfile` | none | none (reads from context) |
| 2 | PUT | `/profile/` | `UpdateProfile` | `services.UpdateProfile` | users collection |
| 3 | DELETE | `/profile/delete` | `DeleteProfile` | `services.DeleteProfile` | users collection |

### User Discovery (`/echo/v1/users`)
| # | Method | Path | Controller | Notes |
|---|--------|------|------------|-------|
| 4 | GET | `/users/suggestions` | `GetUserSuggestions` | **Stub** — returns nil data |
| 5 | GET | `/users/nearby` | `GetNearbyUsers` | **Stub** — returns nil data |
| 6 | GET | `/users/popular` | `GetPopularUsers` | **Stub** — returns nil data |
| 7 | GET | `/users/:id` | `GetUserProfile` | `services.GetUserProfile` |

### Contacts (`/echo/v1/users/contacts`)
| # | Method | Path | Controller |
|---|--------|------|------------|
| 8 | GET | `/contacts/` | `GetUsersContacts` |
| 9 | GET | `/contacts/requests` | `GetContactRequests` |
| 10 | GET | `/contacts/sent-requests` | `GetSentContactRequests` |
| 11 | GET | `/contacts/blocked` | `GetBlockedUsers` |
| 12 | GET | `/contacts/favorites` | `GetFavoriteContacts` |
| 13 | POST | `/contacts/:targetUserId` | `SendContactRequest` |
| 14 | PUT | `/contacts/:requestId` | `AcceptOrDeclineContactRequest` |
| 15 | DELETE | `/contacts/:contactId` | `RemoveContact` |
| 16 | POST | `/contacts/blockUnblock/:userId` | `BlockUnblockUser` |
| 17 | POST | `/contacts/favorite/:userId` | `AddToFavorites` |
| 18 | DELETE | `/contacts/favorite/:userId` | `RemoveFromFavorites` |

## Issues Found

### 1. Route Shadowing Risk
Current registration order: `/:id` before `/suggestions`, `/nearby`, `/popular`. Gin's httprouter generally handles static-vs-parameter precedence correctly, but registering static routes first is the documented best practice and prevents ambiguity.

### 2. Inconsistent Path Naming
`blockUnblock` uses camelCase while the rest of the codebase uses kebab-case (`sent-requests`, `add-members`).

### 3. Misleading Variable Name
`userRoutes` holds discovery/search endpoints — renamed to `discoveryRoutes` for clarity.

### 4. Controller Stubs
`GetUserSuggestions`, `GetNearbyUsers`, `GetPopularUsers` are stub implementations returning nil data. Documented as placeholders.

### 5. Inconsistent Comment Style
Some route lines have verbose inline comments, some have none. Normalized to short, purposeful comments.

## Relevant Conventions (from gin-api.md rule)
- All routes mount under `objects.ApiBasePath` (`/echo/v1/`)
- Handlers must be thin — parse → validate → service → response
- Responses use `utils.SuccessResponse` / `utils.ErrorResponse`
- One registration function per module