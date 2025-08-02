package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ============ GET PROFILE MODEL (is in use)============
type GetProfileRequest struct {
	ID            bson.ObjectID       `json:"_id" bson:"_id"`
	Username      *string             `json:"username" bson:"username"`
	Email         *string             `json:"email" bson:"email"`
	Phone         *string             `json:"phone" bson:"phone"`
	Presence      *PresenceEmbed      `json:"presence" bson:"presence"`
	Profile       *UserProfileEmbed   `json:"profile" bson:"profile"`
	AccountStatus *AccountStatusEmbed `json:"accountStatus" bson:"accountStatus"`
}

// ============ SEARCH USER MODEL PROFILE || MODELS FOR USER PROFILE (is in use) ============
type GetUserProfileRequest struct {
	ID            bson.ObjectID       `json:"_id" bson:"_id"`
	Profile       *UserProfileEmbed   `json:"profile" bson:"profile"`
	AccountStatus *AccountStatusEmbed `json:"accountStatus" bson:"accountStatus"`
}

// ============ UPDATE REQUEST MODELS (is in use) ============

// UpdateUserRequest handles partial updates for user profile
type UpdateUserRequest struct {
	// Basic info - using pointers to distinguish between zero values and unset values
	Username *string `json:"username,omitempty"`
	Email    *string `json:"email,omitempty"`
	Phone    *string `json:"phone,omitempty"`

	// Profile fields
	Profile *UserProfileEmbedRequest `json:"profile,omitempty"`

	// Presence fields
	Presence *UpdatePresenceRequest `json:"presence,omitempty"`

	// Contact info
	ContactInfo *ContactInfoEmbedRequest `json:"contactInfo,omitempty"`

	// Account status
	AccountStatus *AccountStatusEmbedRequest `json:"accountStatus,omitempty"`

	// Settings
	Settings *UserSettingsEmbedRequest `json:"settings,omitempty"`

	// Timestamps
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
}

type UserProfileEmbedRequest struct {
	DisplayName   *string `json:"displayName,omitempty"`
	Avatar        *string `json:"avatar,omitempty"`
	StatusMessage *string `json:"statusMessage,omitempty"`
	Bio           *string `json:"bio,omitempty"`
}

type AccountStatusEmbedRequest struct {
	IsActive   *bool `json:"isActive,omitempty"`
	IsVerified *bool `json:"isVerified,omitempty"`
	IsBanned   *bool `json:"isBanned,omitempty"`
}

type UserSettingsEmbedRequest struct {
	Theme         *string                        `json:"theme,omitempty"`
	Language      *string                        `json:"language,omitempty"`
	SoundsOn      *bool                          `json:"soundsOn,omitempty"`
	Notifications *NotificationPrefsEmbedRequest `json:"notifications,omitempty"`
	Privacy       *PrivacySettingsEmbedRequest   `json:"privacy,omitempty"`
	MessagePrefs  *MessagePrefsEmbedRequest      `json:"messagePrefs,omitempty"`
}

type NotificationPrefsEmbedRequest struct {
	PushEnabled    *bool `json:"pushEnabled,omitempty"`
	EmailEnabled   *bool `json:"emailEnabled,omitempty"`
	SoundEnabled   *bool `json:"soundEnabled,omitempty"`
	MentionsOnly   *bool `json:"mentionsOnly,omitempty"`
	MessagePreview *bool `json:"messagePreview,omitempty"`
}

type PrivacySettingsEmbedRequest struct {
	ShowOnlineStatus *bool   `json:"showOnlineStatus,omitempty"`
	ShowLastSeen     *bool   `json:"showLastSeen,omitempty"`
	AllowContactBy   *string `json:"allowContactBy,omitempty"`
}

type MessagePrefsEmbedRequest struct {
	AutoDownloadImages   *bool `json:"autoDownloadImages,omitempty"`
	AutoDownloadFiles    *bool `json:"autoDownloadFiles,omitempty"`
	ShowEmojiSuggestions *bool `json:"showEmojiSuggestions,omitempty"`
}

// ============ CONTACT MODEL (is in use) ============

type ContactRequest struct {
	ID          *bson.ObjectID           `json:"_id,omitempty" bson:"_id,omitempty"`
	ContactInfo *ContactInfoEmbedRequest `json:"contactInfo,omitempty" bson:"contactInfo,omitempty"`
}

type ContactInfoEmbedRequest struct {
	// All contact relationships in one document
	Relationships *map[string]ContactRelationshipRequest `json:"relationships" bson:"relationships"` // targetUserID -> relationship

	// Quick lookup arrays (duplicated for performance)
	BlockedUsers *[]bson.ObjectID `json:"blockedUsers" bson:"blockedUsers"`
	PendingOut   *[]bson.ObjectID `json:"pendingOut" bson:"pendingOut"`
	PendingIn    *[]bson.ObjectID `json:"pendingIn" bson:"pendingIn"`
	Favorites    *[]bson.ObjectID `json:"favorites" bson:"favorites"`
	ActiveChats  *[]bson.ObjectID `json:"activeChats" bson:"activeChats"`

	// Cached stats
	Stats *ContactStatsEmbedRequest `json:"stats" bson:"stats"`

	// Recent activity for sorting
	RecentInteractions *[]RecentInteractionEmbedRequest `json:"recentInteractions" bson:"recentInteractions"`

	// Timestamps
	CreatedAt time.Time `json:"createdAt" bson:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt" bson:"updatedAt"`
}

type ContactRelationshipRequest struct {
	TargetUserID *bson.ObjectID `json:"targetUserId,omitempty" bson:"targetUserId,omitempty"`
	Status       *string        `json:"status,omitempty" bson:"status,omitempty"` // "active", "blocked", "pending_out", "pending_in"
	RequestedBy  *bson.ObjectID `json:"requestedBy,omitempty" bson:"requestedBy,omitempty"`
	IsFavorite   *bool          `json:"isFavorite,omitempty" bson:"isFavorite,omitempty"`

	ChatID *bson.ObjectID `json:"chatId,omitempty" bson:"chatId,omitempty"`
	// Embedded user data for quick access
	UserInfo *ContactUserInfoRequest `json:"userInfo,omitempty" bson:"userInfo,omitempty"`

	// Interaction stats
	// MessageCount    int64     `json:"messageCount" bson:"messageCount"`
	// LastInteraction time.Time `json:"lastInteraction" bson:"lastInteraction"`

	// Timestamps
	CreatedAt  *time.Time `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	AcceptedAt *time.Time `json:"acceptedAt,omitempty" bson:"acceptedAt,omitempty"`
}

type ContactUserInfoRequest struct {
	Username    *string `json:"username,omitempty" bson:"username,omitempty"`
	DisplayName *string `json:"displayName,omitempty" bson:"displayName,omitempty"`
	Avatar      *string `json:"avatar,omitempty" bson:"avatar,omitempty"`
	// IsOnline    bool      `json:"isOnline" bson:"isOnline"`
	// LastSeen    time.Time `json:"lastSeen" bson:"lastSeen"`
}

type ContactStatsEmbedRequest struct {
	TotalContacts   *int `json:"totalContacts,omitempty" bson:"totalContacts,omitempty"`
	BlockedCount    *int `json:"blockedCount,omitempty" bson:"blockedCount,omitempty"`
	PendingOutCount *int `json:"pendingOutCount,omitempty" bson:"pendingOutCount,omitempty"`
	PendingInCount  *int `json:"pendingInCount,omitempty" bson:"pendingInCount,omitempty"`
	FavoriteCount   *int `json:"favoriteCount,omitempty" bson:"favoriteCount,omitempty"`
}

type RecentInteractionEmbedRequest struct {
	UserID        *bson.ObjectID `json:"userId,omitempty" bson:"userId,omitempty"`
	LastMessageAt *time.Time     `json:"lastMessageAt,omitempty" bson:"lastMessageAt,omitempty"`
	MessageCount  *int           `json:"messageCount,omitempty" bson:"messageCount,omitempty"` // in last 24h
}

//
//
//
//
//
//
//
//
//
//

// ============ REQUEST/RESPONSE MODELS FOR PRESENCE ============

// UpdatePresenceRequest represents a request to update presence
type UpdatePresenceRequest struct {
	Status       *string `json:"status" binding:"required"` // "online", "away", "dnd", "invisible"
	CustomStatus *string `json:"custom_status,omitempty"`
	DeviceInfo   *string `json:"device_info,omitempty"`
	Location     *string `json:"location,omitempty"`
}

// SetCustomStatusRequest represents a request to set custom status
type SetCustomStatusRequest struct {
	CustomStatus string `json:"custom_status" binding:"required"`
}

// UpdateAvailabilityRequest represents a request to update availability
type UpdateAvailabilityRequest struct {
	Status string `json:"status" binding:"required"` // "online", "away", "dnd", "invisible"
}

// PresenceResponse represents a presence response
type PresenceResponse struct {
	UserID       string    `json:"user_id"`
	IsOnline     bool      `json:"is_online"`
	Status       string    `json:"status"`
	CustomStatus string    `json:"custom_status,omitempty"`
	LastSeen     time.Time `json:"last_seen"`
	LastActivity time.Time `json:"last_activity"`
}

// PresenceHistoryResponse represents a presence history response
type PresenceHistoryResponse struct {
	History    []PresenceHistory `json:"history"`
	TotalCount int64             `json:"total_count"`
	Page       int               `json:"page"`
	Limit      int               `json:"limit"`
}

// Constants for presence status
const (
	StatusOnline    = "online"
	StatusAway      = "away"
	StatusDND       = "dnd" // Do Not Disturb
	StatusInvisible = "invisible"
	StatusOffline   = "offline"
)

// Constants for presence event types
const (
	EventPresenceUpdate     = "presence_update"
	EventCustomStatusUpdate = "custom_status_update"
	EventUserLogin          = "user_login"
	EventUserLogout         = "user_logout"
	EventUserActivity       = "user_activity"
)

// ============ REQUEST/RESPONSE MODELS FOR CHAT ============
type SendMessageRequest struct {
	ChatID      string  `json:"chat_id" binding:"required"`
	Content     string  `json:"content" binding:"required"`
	MessageType string  `json:"message_type"` // "text", "image", "file", "audio", "video"
	ParentID    *string `json:"parent_id,omitempty"`
	FileID      *string `json:"file_id,omitempty"`
}

type Pagination struct {
	Page      int    `json:"page"`
	Limit     int    `json:"limit"`
	SortBy    string `json:"sort_by"`
	SortOrder string `json:"sort_order"`
}

// ============ TYPING INDICATOR MODEL ============
type TypingIndicator struct {
	ChatID    string    `json:"chat_id" bson:"chat_id"`
	UserID    string    `json:"user_id" bson:"user_id"`
	IsTyping  bool      `json:"is_typing" bson:"is_typing"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
}

// ============ USER MODEL ============
type UserRegister struct {
	FirstName string `json:"first_name" binding:"required" bson:"first_name"`
	LastName  string `json:"last_name" binding:"required" bson:"last_name"`
	Email     string `json:"email" binding:"required" bson:"email"`
	Password  string `json:"password" binding:"required" bson:"password"`
}

type UserLogin struct {
	Email    string `json:"email" binding:"required" bson:"email"`
	Password string `json:"password" binding:"required" bson:"password"`
}
