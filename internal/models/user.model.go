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

	// Cached data for performance (TTL: 1 hour)
	Cache UserCacheEmbed `json:"cache" bson:"cache"`

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
	IsActive   bool `json:"isActive" bson:"isActive"`
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
	ShowOnlineStatus bool   `json:"showOnlineStatus" bson:"showOnlineStatus"`
	ShowLastSeen     bool   `json:"showLastSeen" bson:"showLastSeen"`
	AllowContactBy   string `json:"allowContactBy" bson:"allowContactBy"` // "everyone", "contacts", "none"
}

type MessagePrefsEmbed struct {
	AutoDownloadImages   bool `json:"autoDownloadImages" bson:"autoDownloadImages"`
	AutoDownloadFiles    bool `json:"autoDownloadFiles" bson:"autoDownloadFiles"`
	ShowEmojiSuggestions bool `json:"showEmojiSuggestions" bson:"showEmojiSuggestions"`
}

type UserCacheEmbed struct {
	ActiveChats     []string  `json:"activeChats" bson:"activeChats"`         // Recently active chat IDs
	RecentContacts  []string  `json:"recentContacts" bson:"recentContacts"`   // Recently contacted user IDs
	UnreadCount     int       `json:"unreadCount" bson:"unreadCount"`         // Total unread messages
	LastCacheUpdate time.Time `json:"lastCacheUpdate" bson:"lastCacheUpdate"` // TTL reference
}

// ============ BATCH OPERATIONS MODELS ============
// Models designed for efficient batch operations means that we can send multiple messages at once by batching them into a single request

type BatchMessageOperation struct {
	Operation string               `json:"operation" bson:"operation"` // "insert", "update", "delete"
	Messages  []Message            `json:"messages" bson:"messages"`
	Updates   []MessageUpdateBatch `json:"updates,omitempty" bson:"updates,omitempty"`
	ChatIDs   []bson.ObjectID      `json:"chatIds" bson:"chatIds"`
	UserID    bson.ObjectID        `json:"userId" bson:"userId"`
	Timestamp time.Time            `json:"timestamp" bson:"timestamp"`
}

type MessageUpdateBatch struct {
	MessageID bson.ObjectID          `json:"messageId" bson:"messageId"`
	Updates   map[string]interface{} `json:"updates" bson:"updates"`
}

type BatchPresenceUpdate struct {
	UserUpdates []UserPresenceUpdate `json:"userUpdates" bson:"userUpdates"`
	Timestamp   time.Time            `json:"timestamp" bson:"timestamp"`
}

type UserPresenceUpdate struct {
	UserID   bson.ObjectID `json:"userId" bson:"userId"`
	Status   string        `json:"status" bson:"status"`
	IsOnline bool          `json:"isOnline" bson:"isOnline"`
	LastSeen time.Time     `json:"lastSeen" bson:"lastSeen"`
}

// ============ AGGREGATED VIEWS FOR PERFORMANCE ============
// Pre-computed views to avoid complex queries
type UserChatList struct {
	UserID bson.ObjectID  `json:"userId" bson:"userId"`
	Chats  []ChatListItem `json:"chats" bson:"chats"`

	// Aggregated data
	TotalUnread  int       `json:"totalUnread" bson:"totalUnread"`
	LastActivity time.Time `json:"lastActivity" bson:"lastActivity"`
	LastUpdated  time.Time `json:"lastUpdated" bson:"lastUpdated"`
}

type ChatListItem struct {
	ChatID       bson.ObjectID    `json:"chatId" bson:"chatId"`
	Type         string           `json:"type" bson:"type"`
	Name         string           `json:"name" bson:"name"`
	Avatar       string           `json:"avatar" bson:"avatar"`
	LastMessage  LastMessageEmbed `json:"lastMessage" bson:"lastMessage"`
	UnreadCount  int              `json:"unreadCount" bson:"unreadCount"`
	IsMuted      bool             `json:"isMuted" bson:"isMuted"`
	IsPinned     bool             `json:"isPinned" bson:"isPinned"`
	LastActivity time.Time        `json:"lastActivity" bson:"lastActivity"`

	// For direct chats - embedded other user info
	OtherUser *ContactUserInfo `json:"otherUser,omitempty" bson:"otherUser,omitempty"`
}

type SearchIndex struct {
	ID       bson.ObjectID `json:"_id,omitempty" bson:"_id,omitempty"`
	Type     string        `json:"type" bson:"type"` // "message", "user", "chat"
	EntityID bson.ObjectID `json:"entityId" bson:"entityId"`

	// Search fields
	Content  string   `json:"content" bson:"content"`
	Tags     []string `json:"tags" bson:"tags"`
	Keywords []string `json:"keywords" bson:"keywords"`

	// Context for results
	ChatID    *bson.ObjectID `json:"chatId,omitempty" bson:"chatId,omitempty"`
	UserID    *bson.ObjectID `json:"userId,omitempty" bson:"userId,omitempty"`
	MessageAt *time.Time     `json:"messageAt,omitempty" bson:"messageAt,omitempty"`

	// Relevance scoring
	Popularity float64   `json:"popularity" bson:"popularity"`
	Recency    float64   `json:"recency" bson:"recency"`
	UpdatedAt  time.Time `json:"updatedAt" bson:"updatedAt"`
}

// ============ API RESPONSE MODELS ============
// Optimized for single-query responses
type ChatDetailResponse struct {
	Chat         Chat                `json:"chat"`
	Messages     []Message           `json:"messages"`
	Participants []ParticipantDetail `json:"participants"`
	CanLoadMore  bool                `json:"canLoadMore"`

	// User-specific data
	UserRole     string    `json:"userRole"`
	LastReadAt   time.Time `json:"lastReadAt"`
	MuteSettings string    `json:"muteSettings"`
	PinStatus    bool      `json:"pinStatus"`
}

type ParticipantDetail struct {
	ParticipantEmbed
	// Extended info when needed
	IsContact      bool     `json:"isContact"`
	IsBlocked      bool     `json:"isBlocked"`
	MutualContacts []string `json:"mutualContacts,omitempty"`
}

// ============ OPTIMIZATION INDEXES ============
// Suggested MongoDB indexes for optimal performance

/*
Recommended Indexes:

OptimizedUser:
- { "username": 1 } (unique)
- { "email": 1 } (unique)
- { "presence.status": 1, "presence.isOnline": 1 }
- { "contactInfo.contacts": 1 }
- { "cache.activeChats": 1 }

OptimizedChat:
- { "participants.userId": 1 }
- { "lastMessage.createdAt": -1 }
- { "type": 1, "settings.isPrivate": 1 }

OptimizedMessage:
- { "chatId": 1, "createdAt": -1 }
- { "senderId": 1, "createdAt": -1 }
- { "searchContent": "text", "searchTags": 1 }
- { "thread.parentId": 1 }

OptimizedContact:
- { "userId": 1 } (unique)
- { "activeContacts": 1 }
- { "recentInteractions.lastMessageAt": -1 }

SearchIndex:
- { "content": "text", "tags": 1, "keywords": 1 }
- { "type": 1, "popularity": -1, "recency": -1 }

BatchNotification:
- { "userId": 1, "createdAt": -1 }
- { "notifications.isRead": 1 }

UserChatList:
- { "userId": 1 } (unique)
- { "lastActivity": -1 }
*/
