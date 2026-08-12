# ECHO Android — UI Screens

## Screen Inventory

| # | Screen | Route | Type | Purpose |
|---|---|---|---|---|
| 1 | SplashScreen | `splash` | Full-screen | Token validation, auto-route |
| 2 | LoginScreen | `auth/login` | Full-screen | Email + password → JWT |
| 3 | SignupScreen | `auth/signup` | Full-screen | Create account |
| 4 | HomeScreen | `home` | Scaffold host | Bottom nav container |
| 5 | ChatsTab | `home/chats` | Tab | Chat list |
| 6 | ChatDetailScreen | `chat/{chatId}` | Full-screen | Messages, input, WS |
| 7 | ContactsTab | `home/contacts` | Tab | Contacts list + search |
| 8 | ContactRequestsScreen | `contacts/requests` | Full-screen | Pending in/out |
| 9 | UserProfileScreen | `user/{userId}` | Full-screen | View other user |
| 10 | CreateGroupScreen | `groups/create` | Full-screen | New group chat |
| 11 | GroupInfoScreen | `groups/{groupId}/info` | Full-screen | Members, invites |
| 12 | SettingsTab | `home/settings` | Tab | Settings menu |
| 13 | EditProfileScreen | `settings/edit-profile` | Full-screen | Edit own profile |
| — | InvitesSheet | Overlay | Bottom sheet | Manage invites |
| — | JoinGroupSheet | Overlay | Bottom sheet | Join by code |

---

## Navigation Graph

```
NavHost(startDestination = "splash")
│
├── "splash" → SplashScreen
│     └── navigate("auth/login") or navigate("home")
│
├── "auth/login" → LoginScreen
│     ├── navigate("auth/signup")
│     └── navigate("home") (on success)
│
├── "auth/signup" → SignupScreen
│     ├── navigate("auth/login") (on success — backend doesn't return token)
│     └── back() (go back)
│
├── "home" → HomeScreen (Scaffold with bottom nav)
│     ├── Tab 1: "chats" → ChatsTab
│     │     └── navigate("chat/{chatId}")
│     ├── Tab 2: "contacts" → ContactsTab
│     │     ├── navigate("contacts/requests")
│     │     └── navigate("user/{userId}")
│     └── Tab 3: "settings" → SettingsTab
│           ├── navigate("settings/edit-profile")
│           └── navigate("auth/login") (logout)
│
├── "chat/{chatId}" → ChatDetailScreen
│     ├── navigate("groups/{groupId}/info")
│     └── back()
│
├── "contacts/requests" → ContactRequestsScreen
│     └── back()
│
├── "user/{userId}" → UserProfileScreen
│     └── back()
│
├── "groups/create" → CreateGroupScreen
│     └── back()
│
├── "groups/{groupId}/info" → GroupInfoScreen
│     └── back()
│
└── "settings/edit-profile" → EditProfileScreen
      └── back()
```

Route sealed class:
```kotlin
sealed class Route(val path: String) {
    data object Splash : Route("splash")
    data object Login : Route("auth/login")
    data object Signup : Route("auth/signup")
    data object Home : Route("home")
    data object Chats : Route("home/chats")
    data object Contacts : Route("home/contacts")
    data object Settings : Route("home/settings")
    data class Chat(val chatId: String) : Route("chat/{chatId}")
    data object ContactRequests : Route("contacts/requests")
    data class UserProfile(val userId: String) : Route("user/{userId}")
    data object CreateGroup : Route("groups/create")
    data class GroupInfo(val groupId: String) : Route("groups/{groupId}/info")
    data object EditProfile : Route("settings/edit-profile")
}
```

---

## Screen Specifications

### 1. SplashScreen

**Purpose:** Validate stored JWT and route accordingly.

**State Machine:**
```
Loading → check token → token valid → navigate Home
                       → no token   → navigate Login
                       → token invalid → navigate Login (clear token)
```

**Composables:**
- `SplashScreen(viewModel: SplashViewModel = hiltViewModel())`
- `SplashContent(isLoading: Boolean)` — shows app logo + loading indicator

**ViewModel:**
- `init { checkAuth() }`
- `state: StateFlow<SplashState>` (Loading, Authenticated, NotAuthenticated)
- Uses `SecureTokenStore.getAccessToken()`

---

### 2. LoginScreen

**Purpose:** Email + password authentication.

**Fields:**
| Field | Type | Validation |
|---|---|---|
| Email | `OutlinedTextField` | `isValidEmail()`, not empty |
| Password | `OutlinedTextField` (password) | min 8 chars, not empty |
| Submit | `Button` | Disabled until valid |

**States:** Idle, Submitting, Error(errorMessage), Success

**Composable Structure:**
```
LoginScreen
├── Box(filterTouchesWhenObscured)
│   ├── AppLogo
│   ├── EmailField(value, onValueChange, isError, errorMessage)
│   ├── PasswordField(value, onValueChange, isError, errorMessage)
│   ├── ErrorBanner(error) — conditional
│   ├── LoginButton(onClick, enabled, isLoading)
│   └── SignupLink(onClick)
```

**Events:** Login(email, password) → ViewModel → LoginUseCase → AuthRepository → API

---

### 3. SignupScreen

**Purpose:** Create new account.

**Fields:**
| Field | Type | Validation |
|---|---|---|
| Email | `OutlinedTextField` | `isValidEmail()`, not empty |
| Username | `OutlinedTextField` | Not empty |
| Password | `OutlinedTextField` (password) | min 8 chars |
| Submit | `Button` | Disabled until valid |

**Post-signup:** Redirect to Login screen with a snackbar "Account created! Please log in."

---

### 4. HomeScreen (Scaffold)

**Structure:**
```kotlin
Scaffold(
    topBar = { EchoTopBar(currentRoute) },
    bottomBar = { EchoBottomBar(navController) }
) { paddingValues ->
    NavHost(modifier = Modifier.padding(paddingValues)) {
        composable("home/chats") { ChatsTab(...) }
        composable("home/contacts") { ContactsTab(...) }
        composable("home/settings") { SettingsTab(...) }
    }
}
```

**Bottom Navigation Items:**
| Tab | Icon | Label |
|---|---|---|
| Chats | `Icons.Default.Chat` | "Chats" |
| Contacts | `Icons.Default.People` | "Contacts" |
| Settings | `Icons.Default.Settings` | "Settings" |

---

### 5. ChatsTab

**Purpose:** List of all chats (direct + group) with last message preview.

**States:** Loading, Empty, Error, Content

**List Item:** `ChatListItem(chat: Chat)` showing:
- Avatar (group icon or user avatar)
- Chat name (user displayName or group name)
- Last message preview (truncated 1 line)
- Timestamp (relative: "2m ago", "yesterday")
- Unread badge (count)
- Online indicator for direct chats (green dot)
- Swipe-to-archive (optional, future)

**FAB:** `+` → navigate to contact picker or `CreateGroupScreen`

---

### 6. ChatDetailScreen

**Purpose:** Real-time message thread.

**Top Bar:**
- Back arrow
- Chat name + subtitle ("Online", "last seen...", "3 members")
- Menu: Group info, clear chat, block user

**Message List (`LazyColumn`, reversed):**
- `MessageBubble(message: Message, isMine: Boolean)`
  - Sender avatar (group only)
  - Content text (with link detection)
  - Image/file attachments (if any)
  - Timestamp
  - Read receipt icons (✓ sent, ✓✓ delivered, ✓✓✓ read — color-coded blue)
  - Reaction bar (👍 ❤️ 😂 😮 😢 🔥) below bubble
  - Long-press: reaction picker

**Input Bar (sticky bottom):**
- Attachment button (+)
- `TextField` (expandable, max 4 lines)
- Send button (enabled only when text is not blank)

**Real-time Indicators (from WS):**
- "User is typing..." below last message (when typing indicator active)
- New message scroll-to-bottom button (when scrolled up)
- Connection status indicator (top banner: "Reconnecting...")

**Pagination:** Load older messages when scrolling to top (`GET /messages/:chatID/messages?offset=...`)

---

### 7. ContactsTab

**Purpose:** Contact list with client-side search + filters. Discovery endpoints (`suggestions`, `nearby`, `popular`) are backend stubs and deliberately excluded.

**Sections:**
1. **Search bar** — client-side filter contacts by name/username
2. **Requests row** — "N pending requests" → navigate to ContactRequestsScreen
3. **Favorites** — horizontal row of favorited contacts
4. **All contacts** — vertical list, alphabetical

**List Item:**
- Avatar (via Coil)
- Display name + username
- Online status dot (green/gray)
- Star icon (favorite toggle)
- Tap → create/open direct chat

---

### 8. ContactRequestsScreen

**Purpose:** Manage incoming/outgoing contact requests.

**Tabs:** Incoming | Outgoing

**Incoming item:**
- Avatar + displayName
- "Wants to be your contact"
- Accept button + Decline button

**Outgoing item:**
- Avatar + displayName
- "Request sent" label
- Cancel button

---

### 9. UserProfileScreen

**Purpose:** View another user's public profile.

**Content:**
- Large avatar
- Display name
- Username
- Bio
- Status/online indicator
- Actions:
  - Send Message → create/get direct chat → navigate to ChatDetail
  - Add Contact / Accept / Pending status → contact management
  - Block User (with confirmation)

---

### 10. CreateGroupScreen

**Fields:**
- Group name (`OutlinedTextField`, required)
- Description (`OutlinedTextField`, optional)
- Member picker — select from contacts (multi-select checkboxes)
- Create button

---

### 11. GroupInfoScreen

**Sections:**
- Group avatar + name + description
- Participant list (with roles: owner/admin/member)
- Invite codes section:
  - "Create Invite" button → generates code
  - List existing codes with expiry + copy/share
  - "Send to User" per invite code
- Leave group button (with confirmation)

---

### 12. SettingsTab

**Content:**
- Profile card (avatar + displayName + email) → tap to EditProfile
- Account section: Change password (stub), Delete account
- Preferences: Theme (light/dark toggle), Notifications toggle
- About: App version, Terms, Privacy
- Logout button (red, with confirmation dialog)

---

### 13. EditProfileScreen

**Purpose:** Update current user's profile via `PUT /echo/v1/profile/`.

**Fields:**
- Avatar (tappable → image picker / URL input)
- Display name (`OutlinedTextField`)
- Bio (`OutlinedTextField`, multiline)
- Status message (`OutlinedTextField`)
- Save button → calls `UpdateProfileUseCase`

**Note:** User suggestions, nearby, and popular user lists are not included — these endpoints are backend stubs (return `null` data) and will be added when backend support is complete.


---

### Overlays

#### InvitesSheet (Bottom Sheet)
- Create new invite (button → auto-generates)
- List of active invites with copy/share actions
- Delete invite (swipe or long-press)

#### JoinGroupSheet (Bottom Sheet)
- `OutlinedTextField` for invite code
- Join button
- Error/success feedback

---

## Material 3 Theming

```kotlin
// Colors — warm, modern palette
LightColorScheme:
    primary: #6C5CE7 (purple)
    secondary: #00B894 (teal)
    surface: #FFFFFF
    background: #F8F9FA
    error: #D63031

DarkColorScheme:
    primary: #A29BFE (soft purple)
    secondary: #55EFC4 (mint)
    surface: #1A1A2E
    background: #0F0F23
    error: #FF7675

// Typography
displayLarge: Bold 28sp
headlineMedium: SemiBold 22sp
titleLarge: SemiBold 18sp
bodyLarge: Regular 16sp
bodyMedium: Regular 14sp
labelSmall: Medium 12sp
```

## UI State Pattern

Every screen follows the UDF (Unidirectional Data Flow) pattern:

```kotlin
// ViewModel
data class ChatListUiState(
    val isLoading: Boolean = false,
    val chats: List<Chat> = emptyList(),
    val error: String? = null,
)

sealed interface ChatListEvent {
    data object Refresh : ChatListEvent
    data class ChatClicked(val chatId: String) : ChatListEvent
    data class CreateDirectChat(val userId: String) : ChatListEvent
}

// Composable
@Composable
fun ChatListScreen(
    viewModel: ChatListViewModel = hiltViewModel(),
    onChatClicked: (String) -> Unit,
    onCreateGroup: () -> Unit,
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    
    ChatListContent(
        chats = state.chats,
        isLoading = state.isLoading,
        error = state.error,
        onRefresh = { viewModel.onEvent(ChatListEvent.Refresh) },
        onChatClicked = onChatClicked,
        onCreateGroup = onCreateGroup,
    )
}
```

## Compose Rules Applied

1. **No `var` in composables for business state** — all in ViewModel
2. **Stateless content composables** — accept all inputs as parameters
3. **`remember` only for UI-internal** (text field, scroll, animation)
4. **`collectAsStateWithLifecycle()`** — lifecycle-aware
5. **`LazyColumn` with `key = { it.id }`** — stable keys
6. **`@Preview` light + dark** on every screen composable
7. **Material 3 tokens** (`MaterialTheme.colorScheme`, `MaterialTheme.typography`) — never hardcoded
8. **Coil's `AsyncImage`** for all remote images
9. **`derivedStateOf`** for computed values (unread count, filtered lists)