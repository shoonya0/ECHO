# ECHO Android — Phase Plan

## Phase Overview

| Phase | Name | Focus | Key Files Created | Status |
|---|---|---|---|---|
| 0 | Project Scaffolding | Gradle, manifest, package structure, theme skeleton | `build.gradle.kts`, `libs.versions.toml`, `AndroidManifest.xml`, theme files | Planned |
| 1 | Core Infrastructure | Hilt setup, networking, security, error mapping | `NetworkModule`, `SecureTokenStore`, `AuthInterceptor`, `ApiResponse`, `ErrorMapper` | Planned |
| 2 | Auth Feature | Login, signup, session, splash routing | `LoginScreen`, `SignupScreen`, `SplashScreen`, `AuthViewModel` | Planned |
| 3 | Contacts Feature | Contact list, requests, search, favorites, block | `ContactsListScreen`, `ContactRequestsScreen`, `ContactViewModel` | Planned |
| 4 | Chat REST | Direct chat creation, group creation, message history, chat list | `ChatListScreen`, `ChatApiService`, `ChatRepository` | Planned |
| 5 | Chat WebSocket | Real-time messaging, typing, reactions, presence | `EchoWebSocketClient`, `WebSocketRepository`, real-time `ChatDetailScreen` | Planned |
| 6 | Group Management | Add members, invite codes, join by code | `CreateGroupScreen`, `GroupInvitesSheet`, `GroupViewModel` | Planned |
| 7 | Profile & Settings | Edit profile, settings, logout | `EditProfileScreen`, `SettingsScreen`, `ProfileViewModel` | Planned |
| 8 | Polish & Tests | Previews, unit tests, smoke tests, edge cases | Test classes, `Fake*` test doubles | Planned |

---

## Phase 0 — Project Scaffolding

### Goal
A compilable Android project with correct Gradle configuration, version catalog, package structure, and theme.

### Files to Create

```
gradle/
└── libs.versions.toml              # Version catalog

app/
├── build.gradle.kts                # Updated from androidbuild.gradle.kts
├── proguard-rules.pro
└── src/main/
    ├── AndroidManifest.xml
    ├── java/com/shoonya/echo/
    │   └── EchoApplication.kt      # @HiltAndroidApp
    └── res/
        └── values/
            └── strings.xml

core/theme/
├── Color.kt                        # Light + Dark color schemes
├── Type.kt                         # Typography
├── Shape.kt                        # Shapes
└── Theme.kt                        # EchoTheme composable
```

### Acceptance Criteria
- [ ] `./gradlew assembleDebug` succeeds
- [ ] App launches on emulator/device
- [ ] Material 3 theme renders (light + dark)
- [ ] Version catalog resolves all deps
- [ ] No hardcoded strings in UI

---

## Phase 1 — Core Infrastructure

### Goal
Global DI wired: OkHttpClient (with auth interceptor), Retrofit, SecureTokenStore, JSON parser, error mapper.

### Files to Create

```
core/
├── data/
│   ├── local/
│   │   └── SecureTokenStore.kt     # EncryptedSharedPreferences wrapper
│   └── remote/
│       ├── NetworkModule.kt        # @Module: OkHttpClient, Retrofit, Json
│       ├── AuthInterceptor.kt      # Injects Bearer token
│       └── ApiResponse.kt          # Response envelope + toResult()
├── di/
│   └── CoreModule.kt               # @Module: SecureTokenStore
└── util/
    ├── ErrorMapper.kt              # Exception/BackendError → user message
    ├── Extensions.kt               # String.isValidEmail(), etc.
    └── DateFormatter.kt            # ISO 8601 → relative time

build.gradle.kts update:
    - Add BuildConfig fields (API_BASE_URL, WS_BASE_URL, ENABLE_LOGGING)
    - Add network_security_config.xml
```

### Acceptance Criteria
- [ ] Hilt component graph builds successfully
- [ ] `SecureTokenStore` saves/reads/clears tokens
- [ ] `AuthInterceptor` injects `Authorization: Bearer` header
- [ ] `ApiResponse<T>` parses from backend JSON
- [ ] `ErrorMapper` maps all known backend error codes
- [ ] JSON parser handles unknown keys, coerces defaults

---

## Phase 2 — Auth Feature

### Goal
Complete login/signup flow: screens → ViewModel → use case → repository → API → token storage → navigation.

### Files to Create

```
features/auth/
├── data/
│   ├── model/
│   │   ├── SignupRequest.kt        # @Serializable
│   │   ├── LoginRequest.kt
│   │   └── AuthResponse.kt         # {token, user}
│   ├── remote/
│   │   └── AuthApiService.kt       # Retrofit interface
│   ├── di/
│   │   └── AuthDataModule.kt       # @Provides @Singleton
│   └── repository/
│       └── AuthRepositoryImpl.kt
├── domain/
│   ├── model/
│   │   └── AuthState.kt            # sealed interface: Loading/Authenticated/Error/NotAuthenticated
│   ├── repository/
│   │   └── AuthRepository.kt       # Interface
│   └── usecase/
│       ├── LoginUseCase.kt
│       ├── SignupUseCase.kt
│       └── LogoutUseCase.kt
└── presentation/
    ├── SplashScreen.kt             # Token check → Login or Home
    ├── LoginScreen.kt
    ├── SignupScreen.kt
    ├── LoginViewModel.kt
    └── LoginUiState.kt

app/
└── MainNavigation.kt               # Route sealed class + NavHost
```

### Acceptance Criteria
- [ ] Splash checks token, routes to Login or Home
- [ ] Login with valid credentials stores JWT → navigates to Home
- [ ] Login with invalid credentials shows error
- [ ] Signup creates account → redirects to Login (no auto-login, backend doesn't return token)
- [ ] Logout clears token → navigates to Login
- [ ] Text fields show validation errors (empty email, short password)
- [ ] `filterTouchesWhenObscured` applied to prevent overlay attacks

---

## Phase 3 — Contacts Feature

### Goal
Full contact management: list view, request management, favorites, block/unblock.

**Note:** Only real working backend endpoints are implemented. `GET /users/suggestions`, `GET /users/nearby`, and `GET /users/popular` are backend stubs (return `null` data) and are excluded from this phase.

### Files to Create

```
features/contacts/
├── data/
│   ├── model/
│   │   ├── ContactDto.kt           # DTOs matching backend response
│   │   └── ContactListResponse.kt
│   ├── remote/
│   │   └── ContactsApiService.kt   # All /contacts/ endpoints
│   ├── di/
│   │   └── ContactsDataModule.kt
│   └── repository/
│       └── ContactsRepositoryImpl.kt
├── domain/
│   ├── model/
│   │   ├── Contact.kt              # Clean domain model
│   │   └── ContactRequest.kt
│   ├── repository/
│   │   └── ContactsRepository.kt   # Interface
│   └── usecase/
│       ├── GetContactsUseCase.kt
│       ├── GetContactRequestsUseCase.kt
│       ├── SendContactRequestUseCase.kt
│       ├── AcceptContactRequestUseCase.kt
│       ├── BlockUserUseCase.kt
│       └── AddToFavoritesUseCase.kt
└── presentation/
    ├── ContactsListScreen.kt
    ├── ContactRequestsScreen.kt
    ├── UserProfileScreen.kt
    ├── ContactsViewModel.kt
    ├── ContactsUiState.kt
    └── ContactsEvent.kt
```

### Acceptance Criteria
- [ ] Contacts list renders with presence indicator (online dot)
- [ ] Contact requests (incoming/outgoing) display in separate tabs
- [ ] Accept/decline contact requests works
- [ ] Send contact request to another user by ID
- [ ] Block/unblock user — toggles correctly
- [ ] Add/remove from favorites — star toggle
- [ ] User profile view (view any user by ID) via `GET /users/:id`
- [ ] Client-side search/filter over real contacts list (no discovery endpoints)

---

## Phase 4 — Chat REST

### Goal
Chat list screen, direct/group chat creation, message history with pagination.

### Files to Create

```
features/chat/
├── data/
│   ├── model/
│   │   ├── ChatDto.kt
│   │   ├── MessageDto.kt
│   │   ├── CreateGroupRequest.kt
│   │   └── MessageListResponse.kt
│   ├── remote/
│   │   ├── ChatApiService.kt       # /chats/ endpoints
│   │   └── MessageApiService.kt    # /messages/ endpoint
│   ├── di/
│   │   └── ChatDataModule.kt
│   └── repository/
│       └── ChatRepositoryImpl.kt
├── domain/
│   ├── model/
│   │   ├── Chat.kt                 # Clean domain model
│   │   ├── Message.kt
│   │   ├── MessageType.kt          # enum: TEXT, IMAGE, FILE, AUDIO, VIDEO
│   │   └── ChatType.kt             # enum: DIRECT, GROUP, CHANNEL
│   ├── repository/
│   │   └── ChatRepository.kt       # Interface
│   └── usecase/
│       ├── GetChatListUseCase.kt
│       ├── CreateDirectChatUseCase.kt
│       ├── CreateGroupChatUseCase.kt
│       └── GetChatMessagesUseCase.kt
└── presentation/
    ├── chatlist/
    │   ├── ChatListScreen.kt
    │   ├── ChatListViewModel.kt
    │   └── ChatListUiState.kt
    └── chatdetail/
        ├── ChatDetailScreen.kt     # Message list + input (no WS yet)
        ├── ChatDetailViewModel.kt
        └── ChatDetailUiState.kt
```

### Chat List Strategy (backend gap workaround)
Since there's no `GET /echo/v1/chats` endpoint:
1. On login, load contacts list
2. For each contact, attempt `POST /chats/direct/:userId` → if existing chat returned, add to list
3. Store discovered chat IDs in local cache (DataStore)
4. Group chats: load from group membership (via invites joined)
5. Poll `GET /messages/:chatID/messages?limit=1` for last message preview

### Acceptance Criteria
- [ ] Chat list shows direct + group chats with last message preview
- [ ] Create direct chat from contact (opens chat)
- [ ] Create group chat with name, description, initial members
- [ ] Chat detail loads message history with pagination (scroll up to load more)
- [ ] Message input field with send button (REST-based send for now — WS in Phase 5)
- [ ] Empty state shown when no chats exist

---

## Phase 5 — Chat WebSocket

### Goal
Real-time messaging, typing indicators, read receipts, reactions, presence updates, reconnection.

### Files to Create/Update

```
features/chat/
├── data/
│   ├── remote/
│   │   ├── EchoWebSocketClient.kt      # OkHttp WebSocket lifecycle
│   │   └── ReconnectManager.kt         # Exponential backoff + jitter
│   └── repository/
│       └── WebSocketRepository.kt      # requestId correlation, frame routing
├── domain/
│   ├── model/
│   │   ├── WebSocketCommand.kt         # Sealed interface: SendMessage, JoinChat, etc.
│   │   ├── ServerFrame.kt              # Sealed interface: ChatMessage, Typing, Reaction, etc.
│   │   └── DomainEvent.kt              # sealed: NewMessage, TypingUpdate, PresenceUpdate, etc.
│   └── usecase/
│       ├── SendMessageUseCase.kt       # Via WS (replaces REST send)
│       ├── JoinChatUseCase.kt
│       ├── SetTypingUseCase.kt
│       ├── MarkReadUseCase.kt
│       └── AddReactionUseCase.kt
├── presentation/
│   └── chatdetail/
│       ├── ChatDetailScreen.kt         # Updated: WS-driven updates
│       └── ChatDetailViewModel.kt      # Updated: collects WS events
└── data/
    └── di/
        └── WebSocketModule.kt          # @Qualifier WsClient OkHttpClient
```

### Acceptance Criteria
- [ ] WebSocket connects on entering any chat screen (via ViewModel init)
- [ ] Messages sent via WS appear in real-time (no polling)
- [ ] Incoming messages update the chat list immediately
- [ ] Typing indicator shows when other user is typing (debounced outgoing: 300ms)
- [ ] Read receipts (double-check marks) update in real-time
- [ ] Reactions (👍 ❤️ 😂 etc.) appear/disappear on messages
- [ ] Presence updates (online/offline dot) update contact list
- [ ] `join_chat` auto-creates direct chat if needed
- [ ] `leave_chat` on navigating away or screen destroy
- [ ] Reconnection with exponential backoff (1s, 2s, 4s, 8s, 16s max 5 retries)
- [ ] `NOT_IMPLEMENTED` error for `edit_message`/`delete_message` shown as "Coming soon"
- [ ] Graceful disconnect on app background (optional: stay connected for notifications)

---

## Phase 6 — Group Management

### Goal
Add members to groups, create/delete invite codes, send invites to users, join via invite code, view user invites.

### Files to Create

```
features/groups/
├── data/
│   ├── model/
│   │   ├── InviteDto.kt
│   │   ├── AddMembersRequest.kt
│   │   └── JoinedUserDto.kt
│   ├── remote/
│   │   └── GroupApiService.kt      # /groups/ endpoints
│   ├── di/
│   │   └── GroupDataModule.kt
│   └── repository/
│       └── GroupRepositoryImpl.kt
├── domain/
│   ├── model/
│   │   ├── Invite.kt
│   │   └── GroupMember.kt
│   ├── repository/
│   │   └── GroupRepository.kt
│   └── usecase/
│       ├── AddGroupMembersUseCase.kt
│       ├── CreateInviteUseCase.kt
│       ├── JoinGroupByInviteUseCase.kt
│       ├── GetGroupInvitesUseCase.kt
│       └── GetUserInvitesUseCase.kt
└── presentation/
    ├── CreateGroupScreen.kt
    ├── GroupInfoScreen.kt          # Member list, invite management
    ├── InvitesSheet.kt             # Bottom sheet: create/delete/share invites
    ├── JoinGroupSheet.kt           # Join by invite code
    └── GroupViewModel.kt
```

### Acceptance Criteria
- [ ] Add members to existing group (admin only)
- [ ] Create invite code for a group
- [ ] View all invite codes for a group
- [ ] Send invite to specific user
- [ ] View all invites sent to current user
- [ ] Join group by invite code
- [ ] Delete invite code
- [ ] View members who joined through an invite
- [ ] Group info screen shows participant list with roles

---

## Phase 7 — Profile & Settings

### Goal
Edit profile (displayName, bio, avatar), view settings, logout confirmation.

### Files to Create

```
features/settings/
├── data/
│   ├── model/
│   │   └── UpdateProfileRequest.kt
│   ├── remote/
│   │   └── ProfileApiService.kt    # /profile/ endpoints
│   ├── di/
│   │   └── SettingsDataModule.kt
│   └── repository/
│       └── SettingsRepositoryImpl.kt
├── domain/
│   ├── model/
│   │   └── UserProfile.kt
│   ├── repository/
│   │   └── SettingsRepository.kt
│   └── usecase/
│       ├── GetProfileUseCase.kt
│       ├── UpdateProfileUseCase.kt
│       └── DeleteAccountUseCase.kt
└── presentation/
    ├── SettingsScreen.kt
    ├── EditProfileScreen.kt
    ├── SettingsViewModel.kt
    └── SettingsUiState.kt
```

### Acceptance Criteria
- [ ] Settings screen shows: theme toggle (light/dark), notification prefs, account info
- [ ] Edit profile: update displayName, bio, avatar URL
- [ ] Get profile loads on settings screen entry
- [ ] Logout button with confirmation dialog → clears token → navigates to Login
- [ ] Delete account (confirmation dialog) — backend has `DELETE /profile/delete`

---

## Phase 8 — Polish & Tests

### Goal
Previews for all screens, unit tests for use cases and ViewModels, Compose UI smoke tests, crash loop prevention.

### Files to Create

```
features/*/presentation/*Preview.kt       # @Preview for every screen-level composable
app/src/test/
├── fakes/
│   ├── FakeAuthRepository.kt
│   ├── FakeChatRepository.kt
│   ├── FakeContactsRepository.kt
│   ├── FakeWebSocketClient.kt
│   └── TestData.kt                       # Pre-built domain objects
├── auth/
│   ├── LoginUseCaseTest.kt
│   └── LoginViewModelTest.kt
├── chat/
│   ├── GetChatListUseCaseTest.kt
│   ├── ChatListViewModelTest.kt
│   └── SendMessageUseCaseTest.kt
└── contacts/
    ├── GetContactsUseCaseTest.kt
    └── ContactsViewModelTest.kt

app/src/androidTest/
└── ui/
    ├── LoginScreenTest.kt
    ├── ChatListScreenTest.kt
    └── ChatDetailScreenTest.kt
```

### Acceptance Criteria
- [ ] All screen-level composables have `@Preview` (light + dark)
- [ ] Use case tests: success + failure paths (min 2 per use case)
- [ ] ViewModel tests: state transitions verified
- [ ] Compose UI tests: Login flow, Chat list rendering, Send message flow
- [ ] `SafeScreen` crash loop guard composable implemented
- [ ] All tests pass: `./gradlew testDebug`
- [ ] UI tests pass: `./gradlew connectedAndroidTest`

---

## Phase Dependencies

```
Phase 0 ──► Phase 1 ──► Phase 2 ──┬──► Phase 3 ──► Phase 4 ──► Phase 5
                                  │                     │
                                  │                     └──► Phase 6
                                  │
                                  └──► Phase 7

Phase 8 (Tests) — runs in parallel with all phases, finalized last
```

- Phase 1 (Core) blocks all feature phases
- Phase 2 (Auth) blocks everything (no token = no API access)
- Phase 3 (Contacts) and Phase 4 (Chat REST) can run in parallel after Auth
- Phase 5 (WebSocket) depends on Phase 4 (Chat detail screen exists)
- Phase 6 (Groups) depends on Phase 4 (group chat creation exists)
- Phase 7 (Settings) depends only on Phase 2 (Auth) — can run any time

## Milestone Summary

| Milestone | Phases | What Works |
|---|---|---|
| M1: Foundation | 0-2 | App launches, login/signup works |
| M2: Social | 0-3, 7 | Login + contacts + profile complete |
| M3: Messenger | 0-5 | Full chat with REST history + WS real-time |
| M4: Groups | 0-6 | Complete group management |
| M5: Release-ready | 0-8 | All features + tested + polished |