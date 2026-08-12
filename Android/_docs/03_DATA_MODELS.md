# ECHO Android — Data Models

## Layer Mapping Convention

| Backend (Go) | Android DTO (`data/model/`) | Android Domain (`domain/model/`) |
|---|---|---|
| `models.User` | `UserDto` | `User` |
| `models.LoginUserResponse` | `AuthResponse` / `UserDto` | `User` (auth) |
| `models.Chat` | `ChatDto` | `Chat` |
| `models.Message` | `MessageDto` | `Message` |
| `models.GetProfileResponse` | `ProfileDto` | `UserProfile` |
| `models.ContactInfoEmbed` | `ContactDto` | `Contact` |
| `models.PresenceEmbed` | `PresenceDto` | `Presence` |
| `models.ChatInvitationEmbed` | `InviteDto` | `Invite` |
| `utils.APIResponse` | `ApiResponse<T>` | N/A (only used at data layer) |

---

## Core: Response Envelope

```kotlin
// core/data/remote/ApiResponse.kt
package com.shoonya.echo.core.data.remote

import kotlinx.serialization.Serializable

@Serializable
data class ApiResponse<T>(
    val success: Boolean,
    val message: String? = null,
    val data: T? = null,
    val error: String? = null,
)

@Serializable
data class PaginatedData<T>(
    val items: List<T>,
    val pagination: PaginationInfo,
)

@Serializable
data class PaginationInfo(
    val limit: Int,
    val total: Int,
    val total_pages: Int,
)

// Extension to unwrap
fun <T> ApiResponse<T>.toResult(): Result<T> {
    return if (success && data != null) {
        Result.success(data)
    } else {
        Result.failure(ApiException(message ?: error ?: "Unknown error"))
    }
}
```

---

## Auth Feature Models

### DTOs (data/model)

```kotlin
// features/auth/data/model/SignupRequest.kt
@Serializable
data class SignupRequest(
    val email: String,
    val username: String,
    val password: String,
)

// features/auth/data/model/LoginRequest.kt
@Serializable
data class LoginRequest(
    val email: String,
    val password: String,
)

// features/auth/data/model/AuthResponse.kt
@Serializable
data class AuthResponse(
    val token: String,
    val user: LoginUserDto,
)

@Serializable
data class LoginUserDto(
    @SerialName("_id") val id: String,
    val email: String,
    val username: String,
    val profile: UserProfileEmbedDto? = null,
    val presence: PresenceDto? = null,
    val accountStatus: AccountStatusDto? = null,
)

@Serializable
data class UserProfileEmbedDto(
    val displayName: String = "",
    val avatar: String = "",
    val statusMessage: String = "",
    val bio: String = "",
)

@Serializable
data class PresenceDto(
    val status: String = "offline",  // "online", "away", "dnd", "invisible", "offline"
    val isOnline: Boolean = false,
    val lastSeen: String? = null,
    val lastActivity: String? = null,
)

@Serializable
data class AccountStatusDto(
    val isActive: Boolean = true,
    val isVerified: Boolean = false,
    val isBanned: Boolean = false,
)
```

### Domain Models (domain/model)

```kotlin
// features/auth/domain/model/AuthState.kt
sealed interface AuthState {
    data object Loading : AuthState
    data class Authenticated(val user: User) : AuthState
    data class Error(val message: String) : AuthState
    data object NotAuthenticated : AuthState
}

// Shared domain model (used across features)
// Could live in core/domain/model/ or the feature that owns it
data class User(
    val id: String,
    val email: String,
    val username: String,
    val displayName: String,
    val avatar: String,
    val statusMessage: String,
    val bio: String,
    val presence: Presence,
    val isActive: Boolean,
    val isVerified: Boolean,
    val isBanned: Boolean,
)

data class Presence(
    val status: PresenceStatus,
    val isOnline: Boolean,
    val lastSeen: Instant?,
)

enum class PresenceStatus {
    ONLINE, AWAY, DND, INVISIBLE, OFFLINE
}
```

---

## Contact Feature Models

### DTOs

```kotlin
// features/contacts/data/model/ContactDto.kt
@Serializable
data class ContactDto(
    @SerialName("_id") val id: String,
    val contactInfo: ContactInfoDto? = null,
)

@Serializable
data class ContactInfoDto(
    val profile: UserProfileEmbedDto? = null,
    val username: String = "",
    val presence: PresenceDto? = null,
    val isFavorite: Boolean = false,
)
```

### Domain Models

```kotlin
// features/contacts/domain/model/Contact.kt
data class Contact(
    val id: String,
    val username: String,
    val displayName: String,
    val avatar: String,
    val presence: Presence,
    val isFavorite: Boolean,
)

// features/contacts/domain/model/ContactRequest.kt
data class ContactRequest(
    val id: String,
    val username: String,
    val displayName: String,
    val avatar: String,
    val direction: RequestDirection,  // INCOMING, OUTGOING
)

enum class RequestDirection { INCOMING, OUTGOING }
```

### DTO → Domain Mapper

```kotlin
fun ContactDto.toDomain(): Contact = Contact(
    id = id,
    username = contactInfo?.username ?: "",
    displayName = contactInfo?.profile?.displayName ?: contactInfo?.username ?: "",
    avatar = contactInfo?.profile?.avatar ?: "",
    presence = contactInfo?.presence?.toDomain() ?: Presence(PresenceStatus.OFFLINE, false, null),
    isFavorite = contactInfo?.isFavorite ?: false,
)

fun PresenceDto.toDomain(): Presence = Presence(
    status = try { PresenceStatus.valueOf(status.uppercase()) } catch (_: Exception) { PresenceStatus.OFFLINE },
    isOnline = isOnline,
    lastSeen = lastSeen?.let { Instant.parse(it) },
)
```

---

## Chat Feature Models

### DTOs

```kotlin
// features/chat/data/model/ChatDto.kt
@Serializable
data class ChatDto(
    @SerialName("_id") val id: String,
    val chatType: String,       // "direct", "group", "channel"
    val name: String = "",
    val description: String = "",
    val avatar: String = "",
    val participants: Map<String, ParticipantDto> = emptyMap(),
    val ownerId: String? = null,
    val adminIds: List<String> = emptyList(),
    val lastMessageId: String? = null,
    val stats: ChatStatsDto? = null,
    val settings: ChatSettingsDto? = null,
    val createdAt: String? = null,
    val updatedAt: String? = null,
)

@Serializable
data class ParticipantDto(
    val requestStatus: String = "accepted",
    val onlineStatus: String = "offline",
    val role: String = "member",
    val userInfo: ContactUserInfoDto? = null,
    val isBlocked: Boolean = false,
    val isMuted: Boolean = false,
    val lastSeen: String? = null,
    val joinedAt: String? = null,
)

@Serializable
data class ContactUserInfoDto(
    val displayName: String = "",
    val username: String = "",
    val userId: String = "",
    val avatar: String = "",
)

@Serializable
data class ChatStatsDto(
    val participantCount: Int = 0,
    val messageCount: Long = 0,
    val unreadCount: Map<String, Int> = emptyMap(),
)

@Serializable
data class ChatSettingsDto(
    val isPrivate: Boolean = false,
    val allowInvites: Boolean = true,
    val allowFileSharing: Boolean = true,
    val messageRetention: Int = 0,
    val maxParticipants: Int = 50,
)
```

### Message DTOs

```kotlin
// features/chat/data/model/MessageDto.kt
@Serializable
data class MessageListResponse(
    val chat: String,
    val messages: List<MessageDto>,
    val pagination: MessagePagination,
)

@Serializable
data class MessagePagination(
    val limit: Int,
    val offset: Int,
    val count: Int,
    val totalCount: Int,
    val page: Int,
    val totalPages: Int,
)

@Serializable
data class MessageDto(
    @SerialName("_id") val id: String? = null,
    val chatId: String,
    val senderId: String,
    val sender: MessageSenderDto? = null,
    val content: String,
    val messageType: String = "text",  // "text", "image", "file", "audio", "video"
    val thread: ThreadDto? = null,
    val attachments: List<AttachmentDto> = emptyList(),
    val reactions: Map<String, ReactionDto> = emptyMap(),
    val mentions: MentionsDto? = null,
    val status: MessageStatusDto? = null,
    val readBy: Map<String, String> = emptyMap(),
    val createdAt: String? = null,
    val updatedAt: String? = null,
)

@Serializable
data class MessageSenderDto(
    val userId: String,
    val username: String,
    val displayName: String,
    val avatar: String,
)

@Serializable
data class ThreadDto(
    val parentId: String? = null,
    val threadId: String? = null,
    val replyCount: Int = 0,
    val lastReplyAt: String? = null,
)

@Serializable
data class AttachmentDto(
    val id: String,
    val originalName: String,
    val url: String,
    val mimeType: String,
    val size: Long,
    val width: Int? = null,
    val height: Int? = null,
    val duration: Int? = null,
)

@Serializable
data class ReactionDto(
    val count: Int = 0,
    val users: List<String> = emptyList(),
    val details: Map<String, String> = emptyMap(),  // userId → username
)

@Serializable
data class MentionsDto(
    val userIds: List<String> = emptyList(),
    val userNames: List<String> = emptyList(),
)

@Serializable
data class MessageStatusDto(
    val isEdited: Boolean = false,
    val isDeleted: Boolean = false,
    val isPinned: Boolean = false,
)
```

### Domain Models

```kotlin
// features/chat/domain/model/Chat.kt
data class Chat(
    val id: String,
    val type: ChatType,
    val name: String,
    val description: String,
    val avatar: String,
    val participants: List<Participant>,
    val ownerId: String?,
    val participantCount: Int,
    val messageCount: Long,
    val unreadCount: Int,        // For current user
    val lastMessage: Message?,    // Preview
    val settings: ChatSettings,
    val createdAt: Instant,
)

enum class ChatType { DIRECT, GROUP, CHANNEL }

data class Participant(
    val userId: String,
    val username: String,
    val displayName: String,
    val avatar: String,
    val role: ChatRole,
    val isOnline: Boolean,
    val isBlocked: Boolean,
    val lastSeen: Instant?,
)

enum class ChatRole { OWNER, ADMIN, MEMBER }

data class ChatSettings(
    val isPrivate: Boolean,
    val allowInvites: Boolean,
    val allowFileSharing: Boolean,
)

// features/chat/domain/model/Message.kt
data class Message(
    val id: String,
    val chatId: String,
    val senderId: String,
    val senderName: String,
    val senderDisplayName: String,
    val senderAvatar: String,
    val content: String,
    val type: MessageType,
    val attachments: List<Attachment>,
    val reactions: Map<String, Reaction>,  // emoji → Reaction
    val isEdited: Boolean,
    val readByUserIds: Set<String>,
    val createdAt: Instant,
)

enum class MessageType { TEXT, IMAGE, FILE, AUDIO, VIDEO }

data class Attachment(
    val id: String,
    val name: String,
    val url: String,
    val mimeType: String,
    val size: Long,
)

data class Reaction(
    val count: Int,
    val userNames: List<String>,
    val hasReacted: Boolean,  // Current user reacted?
)
```

---

## Group Feature Models

### DTOs

```kotlin
// features/groups/data/model/InviteDto.kt
@Serializable
data class CreateGroupRequest(
    val name: String,
    val description: String = "",
    val participants: List<String>,  // userIds
)

@Serializable
data class AddMembersRequest(
    val userIDs: List<String>,
)

@Serializable
data class InviteDto(
    val inviteCode: String,
    val userIDs: List<String> = emptyList(),
    val expiredDate: String? = null,
    val status: String = "active",
    val deleted: Boolean = false,
)

@Serializable
data class JoinedUserDto(
    @SerialName("_id") val id: String,
    val username: String,
    val profile: UserProfileEmbedDto? = null,
    val joinedAt: String? = null,
)
```

### Domain Models

```kotlin
// features/groups/domain/model/Invite.kt
data class Invite(
    val code: String,
    val recipientUserIds: List<String>,
    val expiresAt: Instant?,
    val isActive: Boolean,
)

data class GroupMember(
    val userId: String,
    val username: String,
    val displayName: String,
    val avatar: String,
    val joinedAt: Instant?,
)
```

---

## Settings/Profile Models

### DTOs

```kotlin
// features/settings/data/model/UpdateProfileRequest.kt
@Serializable
data class UpdateProfileRequest(
    val profile: UserProfileEmbedDto? = null,
    val settings: UserSettingsDto? = null,
)

@Serializable
data class ProfileDto(
    @SerialName("_id") val id: String,
    val username: String,
    val email: String,
    val phone: String = "",
    val presence: PresenceDto? = null,
    val profile: UserProfileEmbedDto? = null,
    val accountStatus: AccountStatusDto? = null,
)

// From backend model UserSettingsEmbed
@Serializable
data class UserSettingsDto(
    val theme: String = "system",
    val language: String = "en",
    val soundsOn: Boolean = true,
    val notifications: NotificationPrefsDto? = null,
    val privacy: PrivacySettingsDto? = null,
)

@Serializable
data class NotificationPrefsDto(
    val pushEnabled: Boolean = true,
    val emailEnabled: Boolean = true,
    val soundEnabled: Boolean = true,
    val mentionsOnly: Boolean = false,
    val messagePreview: Boolean = true,
)

@Serializable
data class PrivacySettingsDto(
    val showOnlineStatus: String = "everyone",
    val showLastSeen: Boolean = true,
    val allowContactBy: String = "everyone",
)
```

### Domain Models

```kotlin
data class UserProfile(
    val id: String,
    val username: String,
    val email: String,
    val phone: String,
    val displayName: String,
    val avatar: String,
    val statusMessage: String,
    val bio: String,
    val presence: Presence,
    val isActive: Boolean,
)
```

---

## WebSocket Protocol Models

See [05_WEBSOCKET_CLIENT.md](./05_WEBSOCKET_CLIENT.md) for full WS frame models.

### Key WS Domain Models

```kotlin
// features/chat/domain/model/WebSocketCommand.kt
@Serializable
sealed interface WebSocketCommand {
    val requestId: String

    @Serializable @SerialName("send_message")
    data class SendMessage(
        val chatId: String, val content: String, val messageType: String = "text",
        val attachments: List<Attachment> = emptyList(), val mentions: List<String> = emptyList(),
        override val requestId: String = generateRequestId(),
    ) : WebSocketCommand

    @Serializable @SerialName("join_chat")
    data class JoinChat(val chatId: String, val senderId: String, override val requestId: String = generateRequestId()) : WebSocketCommand

    @Serializable @SerialName("leave_chat")
    data class LeaveChat(val chatId: String, override val requestId: String = generateRequestId()) : WebSocketCommand

    @Serializable @SerialName("set_typing")
    data class SetTyping(val chatId: String, val metadata: TypingMetadata, override val requestId: String = "typing_$chatId") : WebSocketCommand

    @Serializable @SerialName("mark_read")
    data class MarkRead(val chatId: String, val metadata: MarkReadMetadata, override val requestId: String = generateRequestId()) : WebSocketCommand

    @Serializable @SerialName("add_reaction")
    data class AddReaction(val chatId: String, val metadata: ReactionMetadata, override val requestId: String = generateRequestId()) : WebSocketCommand

    @Serializable @SerialName("remove_reaction")
    data class RemoveReaction(val chatId: String, val metadata: ReactionMetadata, override val requestId: String = generateRequestId()) : WebSocketCommand

    companion object {
        private var counter = 0L
        fun generateRequestId(): String = "req_${++counter}_${System.currentTimeMillis()}"
    }
}

@Serializable data class TypingMetadata(val isTyping: Boolean)
@Serializable data class MarkReadMetadata(val messageIds: List<String>)
@Serializable data class ReactionMetadata(val messageId: String, val emoji: String)
```

---

## Mapping Functions (DTO → Domain)

All mapping lives in the data layer (`data/model/*Mapper.kt` or extension functions on DTOs):

```kotlin
// features/chat/data/model/ChatMappers.kt
fun ChatDto.toDomain(myUserId: String): Chat = Chat(
    id = id,
    type = when (chatType) { "group" -> ChatType.GROUP; "channel" -> ChatType.CHANNEL; else -> ChatType.DIRECT },
    name = name.ifEmpty {
        // For direct chats: use the other participant's display name
        participants.values
            .firstOrNull { it.userInfo?.userId != myUserId }
            ?.userInfo?.displayName ?: "Unknown"
    },
    // ... etc
)
```

---

## Date Formatting

Backend sends ISO 8601 timestamps (e.g., `"2026-08-08T16:00:00Z"`).

```kotlin
// core/util/DateFormatter.kt
fun String.toInstant(): Instant = Instant.parse(this)

fun Instant.toRelativeTime(now: Instant = Clock.System.now()): String {
    val duration = Duration.between(this, now)
    return when {
        duration.isNegative() -> "just now"
        duration.seconds < 60 -> "just now"
        duration.toMinutes() < 60 -> "${duration.toMinutes()}m ago"
        duration.toHours() < 24 -> "${duration.toHours()}h ago"
        duration.toDays() < 7 -> "${duration.toDays()}d ago"
        else -> {
            val local = this.toLocalDateTime(TimeZone.currentSystemDefault())
            "${local.month.number}/${local.dayOfMonth}/${local.year}"
        }
    }
}