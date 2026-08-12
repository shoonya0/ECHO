# ECHO Android — Project Overview

## App Identity

- **Name:** ECHO
- **Package:** `com.shoonya.echo`
- **Namespace:** `com.Shoonya.Echo` (from build.gradle.kts)
- **Language:** Kotlin 2.x
- **Min SDK:** 34
- **Target SDK:** 36
- **Compile SDK:** 37

## Tech Stack

| Layer | Technology |
|---|---|
| UI | Jetpack Compose + Material 3 (BOM-managed) |
| DI | Hilt (`@HiltAndroidApp`, `@HiltViewModel`, `@Inject`) |
| HTTP | Retrofit 2.11 + OkHttp 4.12 |
| Serialization | kotlinx.serialization (JSON) |
| WebSocket | OkHttp `WebSocket` (`/echo/v1/ws/chat`) |
| Auth | JWT Bearer token, stored in `EncryptedSharedPreferences` |
| Images | Coil (`AsyncImage`) |
| Logging | Timber |
| Async | Coroutines (`viewModelScope`) + `StateFlow`/`SharedFlow` |
| Testing | JUnit 4, Compose UI test, `StandardTestDispatcher` |

## Architecture

Clean Architecture with feature-first organization:

```
com.shoonya.echo/
├── EchoApplication.kt           # @HiltAndroidApp
├── MainActivity.kt               # Single Activity, Compose host
├── MainNavigation.kt             # NavHost + Route sealed class
│
├── core/
│   ├── data/
│   │   ├── local/SecureTokenStore.kt
│   │   └── remote/
│   │       ├── NetworkModule.kt       # OkHttpClient, Retrofit singleton
│   │       ├── AuthInterceptor.kt     # Injects JWT Bearer header
│   │       └── ApiResponse.kt         # {success, message, data, error}
│   ├── di/CoreModule.kt               # Global @Provides @Singleton
│   ├── theme/                         # MaterialTheme, Color, Typography
│   └── util/
│       ├── ErrorMapper.kt             # Exception/BackendError → user string
│       ├── Extensions.kt
│       └── DateFormatter.kt
│
└── features/
    ├── auth/         # Login, Signup, session
    ├── chat/         # Chat list, chat detail, WebSocket
    ├── contacts/     # Contact list, requests, favorites, block
    ├── groups/       # Group creation, invites, member management
    └── settings/     # Profile edit, settings, logout
```

Each feature has 3 layers: `data/`, `domain/`, `presentation/`.

### Layer Rules

- **data/** — DTOs, Retrofit interfaces, repository impls. Depends on domain interfaces.
- **domain/** — Clean data classes, repository interfaces, use cases. No Android deps.
- **presentation/** — Composables, ViewModels, UiState, UiEvent. No Retrofit calls directly.

### Dependency Direction

```
presentation → domain ← data
    ↑                      ↑
    └────── core/ ─────────┘
```

Core provides shared infrastructure (DI modules, theme, networking client, utilities).
Features share via `core/` only — never by direct cross-feature imports.

## Backend Integration

### REST API

Base URL: `{host}/echo/v1/` (from `objects.ApiBasePath`)

Response envelope:
```json
{
  "success": true,
  "message": "...",
  "data": { ... },
  "error": "..."
}
```

Pagination envelope:
```json
{
  "success": true,
  "message": "...",
  "data": {
    "items": [ ... ],
    "pagination": { "limit": 10, "total": 25, "total_pages": 3 }
  }
}
```

### WebSocket

Endpoint: `ws://{host}/echo/v1/ws/chat` (JWT via `Authorization: Bearer` header)

Protocol: typed JSON frames with `requestId` correlation.
See [05_WEBSOCKET_CLIENT.md](./05_WEBSOCKET_CLIENT.md) for full protocol.

### Key Backend Gaps (noted for future)

1. **No "get my chats" REST endpoint** — chat list must be derived from contacts + `join_chat` responses.
2. **Signup returns no JWT** — must redirect to Login after signup succeeds.

### Out of Scope (Backend Stubs / Not Implemented)

These backend endpoints exist but are **not yet implemented** (return empty data or `NOT_IMPLEMENTED` errors). The Android app does **not** build UI for them:

| Endpoint / Feature | Backend Status |
|---|---|
| `GET /users/suggestions` | STUB — returns `{ success: true, data: null }` |
| `GET /users/nearby` | STUB — returns `{ success: true, data: null }` |
| `GET /users/popular` | STUB — returns `{ success: true, data: null }` |
| WS `edit_message` | Returns `NOT_IMPLEMENTED` error |
| WS `delete_message` | Returns `NOT_IMPLEMENTED` error |
| WS `invite_user` | Returns `NOT_IMPLEMENTED` error |
| WS `remove_user` | Returns `NOT_IMPLEMENTED` error |
| WS `update_chat` | Returns `NOT_IMPLEMENTED` error |

These may be added in future phases once backend support is complete.

## Navigation

Single-activity architecture with Jetpack Navigation Compose:

```
SplashScreen → LoginScreen/SignupScreen ⇄ HomeScreen (Scaffold with bottom nav)
                                            ├── ChatsTab → ChatDetailScreen
                                            ├── ContactsTab → ContactRequestsScreen
                                            │              → UserProfileScreen
                                            └── SettingsTab → EditProfileScreen

Overlays: CreateGroupScreen (dialog/fullscreen), InvitesSheet (bottom sheet)
```

Sealed `Route` class in `MainNavigation.kt` defines all routes — no hardcoded strings in composables.

## What We're Building

ECHO is a real-time chat app supporting:

- **Direct messages** — 1:1 chats auto-created on first message
- **Group chats** — 3+ participants, role-based permissions (owner/admin/member)
- **Contact management** — requests, favorites, block/unblock
- **Real-time features** — typing indicators, read receipts, message reactions, presence
- **Group invites** — invite codes, join by code
- **Profile & settings** — display name, bio, avatar, notification preferences