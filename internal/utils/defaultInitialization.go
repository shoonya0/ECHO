package utils

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// NewUserWithDefaults creates a new User with all embedded structures properly initialized
// and a new server-generated ID. The caller can't choose the ID (#34), and the
// returned user carries the same ID that will be inserted.
func NewUserWithDefaults(email, username, passwordHash string) models.User {
	now := time.Now()

	return models.User{
		ID:           bson.NewObjectID(),
		Email:        email,
		Username:     username,
		PasswordHash: passwordHash,
		CreatedAt:    now,
		UpdatedAt:    now,

		// Initialize Profile with defaults
		Profile: models.UserProfileEmbed{
			DisplayName:   username, // The username is the default display name
			Avatar:        "",
			StatusMessage: "",
			Bio:           "",
		},

		// Initialize Presence with defaults
		Presence: models.PresenceEmbed{
			Status:       string(constants.UserStatusOnline),
			IsOnline:     false,
			LastSeen:     now,
			LastActivity: now,
			DeviceInfo:   "",
			Location:     "",
		},

		// Initialize ContactInfo with empty arrays
		ContactInfo: models.ContactInfoEmbed{
			Blocked:    make([]bson.ObjectID, 0),
			BlockedBy:  make([]bson.ObjectID, 0),
			PendingOut: make([]bson.ObjectID, 0),
			PendingIn:  make([]bson.ObjectID, 0),
			Favorites:  make([]bson.ObjectID, 0),
			Contacts:   make([]bson.ObjectID, 0),
			CreatedAt:  now,
			UpdatedAt:  now,
		},

		// Initialize AccountStatus with defaults
		AccountStatus: models.AccountStatusEmbed{
			IsActive:   true,
			IsVerified: false,
			IsBanned:   false,
		},

		// Initialize Settings with defaults
		Settings: models.UserSettingsEmbed{
			Theme:    "system",
			Language: "en",
			SoundsOn: true,
			Notifications: models.NotificationPrefsEmbed{
				PushEnabled:    true,
				EmailEnabled:   true,
				SoundEnabled:   true,
				MentionsOnly:   false,
				MessagePreview: true,
			},
			Privacy: models.PrivacySettingsEmbed{
				ShowOnlineStatus: "everyone",
				ShowLastSeen:     false,
				AllowContactBy:   "everyone",
			},
			MessagePreferences: models.MessagePrefsEmbed{
				AutoDownloadImages:   true,
				AutoDownloadFiles:    false,
				ShowEmojiSuggestions: true,
			},
		},
		Chats:           make(map[bson.ObjectID]int),
		ChatInvitations: make([]models.ChatInvitationEmbed, 0),
	}
}

func NewChatWithDefaults(id bson.ObjectID, chatType string) models.Chat {
	now := time.Now()

	return models.Chat{
		ChatID:        bson.NewObjectIDFromTimestamp(now),
		ChatType:      chatType,
		Name:          "",
		Description:   "",
		Avatar:        "",
		Participants:  make(map[bson.ObjectID]models.ParticipantEmbed),
		OwnerID:       id,
		AdminIDs:      []bson.ObjectID{id},
		LastMessageID: bson.ObjectID{},
		Stats: models.ChatStatsEmbed{
			ParticipantCount: 0,
			MessageCount:     0,
			UnreadCount:      make(map[bson.ObjectID]int),
			ImageCount:       0,
			FileCount:        0,
		},
		Settings: models.ChatSettingsEmbed{
			IsPrivate:        true,
			AllowInvites:     true,
			AllowFileSharing: true,
			MessageRetention: 0,
			MaxParticipants:  0,
		},
		InviteCode:    make([]models.InviteCodeEmbed, 0),
		ReadReceipts:  make(map[bson.ObjectID]time.Time),
		TypingUsers:   make(map[bson.ObjectID]time.Time),
		ActiveClients: make(map[string]*models.Client),
		LastActivity:  now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

// NewParticipantWithDefaults builds a chat participant. role is a constants.ChatRole
// ("owner", "admin", "member"); callers must choose it, since the owner/admin checks in
// AddGroupMember and RemoveGroupMember read it (#18).
func NewParticipantWithDefaults(role string, requestStatus string, onlineStatus string, requestedBy bson.ObjectID, permissions []string, userInfo models.ContactUserInfo) models.ParticipantEmbed {
	return models.ParticipantEmbed{
		RequestStatus: requestStatus,
		OnlineStatus:  onlineStatus,
		RequestedBy:   requestedBy,
		Permissions:   permissions,
		IsBlocked:     false,
		UserInfo:      userInfo,
		LastSeen:      time.Now(),
		IsMuted:       false,
		Role:          role,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		JoinedAt:      time.Now(),
		LeftAt:        time.Now(),
	}
}

// inviteCodeBytes is the number of random bytes in an invite code (#15).
const inviteCodeBytes = 16

// NewInviteCodeWithDefaults makes a new active invite code for chatID that
// expires at expireTime. The code is inviteCodeBytes random bytes, hex-encoded:
// it carries nothing, and the invite routes find the group by the stored code.
// Before #15 it was the chat ID, the creator's ID and display name and the
// expiry, encrypted with a key that was in this public repo. user is unused
// now; it stays so callers don't change.
func NewInviteCodeWithDefaults(user models.LoginUserResponse, chatID bson.ObjectID, expireTime time.Time) (models.InviteCodeEmbed, bool) {

	raw := make([]byte, inviteCodeBytes)
	if _, err := rand.Read(raw); err != nil {
		return models.InviteCodeEmbed{}, false
	}
	inviteCode := hex.EncodeToString(raw)

	status := "active"
	deleted := false

	invite := models.InviteCodeEmbed{
		InviteCode:  inviteCode,
		UserIDs:     []bson.ObjectID{},
		ExpiredDate: expireTime,
		Status:      status,
		Deleted:     deleted,
	}

	return invite, true
}
