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
			Relationships: make(map[string]models.ContactRelationship),
			BlockedChats:  make([]bson.ObjectID, 0),
			PendingOut:    make([]bson.ObjectID, 0),
			PendingIn:     make([]bson.ObjectID, 0),
			Favorites:     make([]bson.ObjectID, 0),
			ActiveChats:   make([]bson.ObjectID, 0),
			Stats: models.ContactStatsEmbed{
				TotalContacts:   0,
				BlockedCount:    0,
				PendingOutCount: 0,
				PendingInCount:  0,
				FavoriteCount:   0,
			},
			RecentInteractions: make([]models.RecentInteractionEmbed, 0),
			CreatedAt:          now,
			UpdatedAt:          now,
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
				ShowOnlineStatus: true,
				ShowLastSeen:     true,
				AllowContactBy:   "everyone",
			},
			MessagePreferences: models.MessagePrefsEmbed{
				AutoDownloadImages:   true,
				AutoDownloadFiles:    false,
				ShowEmojiSuggestions: true,
			},
		},

		// Initialize Cache with empty arrays
		Cache: models.UserCacheEmbed{
			ActiveChats:     make([]bson.ObjectID, 0),
			RecentContacts:  make([]bson.ObjectID, 0),
			UnreadCount:     0,
			LastCacheUpdate: now,
		},
	}
}
