package utils

import (
	"gin/internal/models"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// NewUserWithDefaults creates a new User with all embedded structures properly initialized
func NewUserWithDefaults(id bson.ObjectID, email, passwordHash string) models.User {
	now := time.Now()

	return models.User{
		ID:           id,
		Email:        email,
		PasswordHash: passwordHash,
		CreatedAt:    now,
		UpdatedAt:    now,

		// Initialize Profile with defaults
		Profile: models.UserProfileEmbed{
			DisplayName:   email, // Use email as default display name
			Avatar:        "",
			StatusMessage: "",
			Bio:           "",
		},

		// Initialize Presence with defaults
		Presence: models.PresenceEmbed{
			Status:       "offline",
			IsOnline:     false,
			LastSeen:     now,
			LastActivity: now,
			DeviceInfo:   "",
			Location:     "",
		},

		// Initialize ContactInfo with empty arrays
		ContactInfo: models.ContactInfoEmbed{
			BlockedChats: make(map[bson.ObjectID]bson.ObjectID),
			PendingOut:   make(map[bson.ObjectID]bson.ObjectID),
			PendingIn:    make(map[bson.ObjectID]bson.ObjectID),
			Favorites:    make(map[bson.ObjectID]bson.ObjectID),
			Contacts:     make(map[bson.ObjectID]bson.ObjectID),
			UnreadCount:  make(map[bson.ObjectID]int),
			CreatedAt:    now,
			UpdatedAt:    now,
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
		AdminIDs:      []bson.ObjectID{},
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
		ReadReceipts:  make(map[bson.ObjectID]time.Time),
		TypingUsers:   make(map[bson.ObjectID]time.Time),
		ActiveClients: make(map[string]*models.Client),
		LastActivity:  now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func NewParticipantWithDefaults(requestStatus string, onlineStatus string, requestedBy bson.ObjectID, permissions []string, userInfo models.ContactUserInfo) models.ParticipantEmbed {
	return models.ParticipantEmbed{
		RequestStatus: requestStatus,
		OnlineStatus:  onlineStatus,
		RequestedBy:   requestedBy,
		Permissions:   permissions,
		IsBlocked:     false,
		UserInfo:      userInfo,
		LastSeen:      time.Now(),
		IsMuted:       false,
		Role:          "member",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		JoinedAt:      time.Now(),
		LeftAt:        time.Now(),
	}
}
