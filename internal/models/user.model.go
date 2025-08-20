// Package declaration for root directory compatibility
// Note: These optimized models should be moved to internal/models/ directory
// and use "package models" when integrated into your project structure
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ============ OPTIMIZED USER MODEL WITH EMBEDDED DATA ============
// Combines user info, presence, and contact data to reduce queries
type User struct {
	ID           bson.ObjectID `json:"_id,omitempty" bson:"_id,omitempty"`
	Username     string        `json:"username" bson:"username"`
	Email        string        `json:"email" bson:"email"`
	Phone        string        `json:"phone,omitempty" bson:"phone,omitempty"`
	PasswordHash string        `json:"-" bson:"passwordHash"`

	// Profile - frequently accessed together
	Profile UserProfileEmbed `json:"profile" bson:"profile"`

	// Presence - embedded to avoid separate queries
	Presence PresenceEmbed `json:"presence" bson:"presence"`

	// Contact data - embedded for quick access
	ContactInfo ContactInfoEmbed `json:"contactInfo" bson:"contactInfo"`

	// Account status - grouped for efficiency
	AccountStatus AccountStatusEmbed `json:"accountStatus" bson:"accountStatus"`

	// Settings - embedded to avoid separate collection
	Settings UserSettingsEmbed `json:"settings" bson:"settings"`

	// Chats used for quick access (chatID -> unread count)
	Chats map[bson.ObjectID]int `json:"chats" bson:"chats"`

	// Timestamps
	CreatedAt time.Time `json:"createdAt" bson:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt" bson:"updatedAt"`
}

type UserProfileEmbed struct {
	DisplayName   string `json:"displayName" bson:"displayName"`
	Avatar        string `json:"avatar" bson:"avatar"`
	StatusMessage string `json:"statusMessage" bson:"statusMessage"`
	Bio           string `json:"bio" bson:"bio"`
}

type PresenceEmbed struct {
	Status       string    `json:"status" bson:"status"` // "online", "away", "dnd", "invisible", "offline"
	IsOnline     bool      `json:"isOnline" bson:"isOnline"`
	LastSeen     time.Time `json:"lastSeen" bson:"lastSeen"`
	LastActivity time.Time `json:"lastActivity" bson:"lastActivity"`
	DeviceInfo   string    `json:"deviceInfo,omitempty" bson:"deviceInfo,omitempty"`
	Location     string    `json:"location,omitempty" bson:"location,omitempty"`
}

type AccountStatusEmbed struct {
	IsActive   bool `json:"isActive" bson:"isActive"` // if user account is active or not
	IsVerified bool `json:"isVerified" bson:"isVerified"`
	IsBanned   bool `json:"isBanned" bson:"isBanned"`
}

type UserSettingsEmbed struct {
	Theme              string                 `json:"theme" bson:"theme"`
	Language           string                 `json:"language" bson:"language"`
	SoundsOn           bool                   `json:"soundsOn" bson:"soundsOn"`
	Notifications      NotificationPrefsEmbed `json:"notifications" bson:"notifications"`
	Privacy            PrivacySettingsEmbed   `json:"privacy" bson:"privacy"`
	MessagePreferences MessagePrefsEmbed      `json:"messagePrefs" bson:"messagePrefs"`
}

type NotificationPrefsEmbed struct {
	PushEnabled    bool `json:"pushEnabled" bson:"pushEnabled"`
	EmailEnabled   bool `json:"emailEnabled" bson:"emailEnabled"`
	SoundEnabled   bool `json:"soundEnabled" bson:"soundEnabled"`
	MentionsOnly   bool `json:"mentionsOnly" bson:"mentionsOnly"`
	MessagePreview bool `json:"messagePreview" bson:"messagePreview"`
}

type PrivacySettingsEmbed struct {
	ShowOnlineStatus string `json:"showOnlineStatus" bson:"showOnlineStatus"` // "everyone", "contacts", "none"
	ShowLastSeen     bool   `json:"showLastSeen" bson:"showLastSeen"`
	AllowContactBy   string `json:"allowContactBy" bson:"allowContactBy"` // "everyone", "contacts", "none"
}

type MessagePrefsEmbed struct {
	AutoDownloadImages   bool `json:"autoDownloadImages" bson:"autoDownloadImages"`
	AutoDownloadFiles    bool `json:"autoDownloadFiles" bson:"autoDownloadFiles"`
	ShowEmojiSuggestions bool `json:"showEmojiSuggestions" bson:"showEmojiSuggestions"`
}
