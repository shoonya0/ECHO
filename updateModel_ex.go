// Package declaration for root directory compatibility
// Note: These optimized models should be moved to internal/models/ directory
// and use "package models" when integrated into your project structure
package main

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ============ OPTIMIZED USER MODEL WITH EMBEDDED DATA ============
// Combines user info, presence, and contact data to reduce queries
type OptimizedUser struct {
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

type ContactInfoEmbed struct {
	// Arrays of user IDs for quick lookup without joins
	Contacts     []bson.ObjectID `json:"contacts" bson:"contacts"`
	BlockedUsers []bson.ObjectID `json:"blockedUsers" bson:"blockedUsers"`
	PendingOut   []bson.ObjectID `json:"pendingOut" bson:"pendingOut"` // Sent requests
	PendingIn    []bson.ObjectID `json:"pendingIn" bson:"pendingIn"`   // Received requests
	Favorites    []bson.ObjectID `json:"favorites" bson:"favorites"`
	ContactCount int             `json:"contactCount" bson:"contactCount"` // Cached count
}

type AccountStatusEmbed struct {
	IsActive   bool `json:"isActive" bson:"isActive"`
	IsVerified bool `json:"isVerified" bson:"isVerified"`
	IsBanned   bool `json:"isBanned" bson:"isBanned"`
}

type UserSettingsEmbed struct {
	Theme              string            `json:"theme" bson:"theme"`
	Language           string            `json:"language" bson:"language"`
	Notifications      NotificationPrefs `json:"notifications" bson:"notifications"`
	Privacy            PrivacySettings   `json:"privacy" bson:"privacy"`
	MessagePreferences MessagePrefs      `json:"messagePrefs" bson:"messagePrefs"`
}

type NotificationPrefs struct {
	PushEnabled    bool `json:"pushEnabled" bson:"pushEnabled"`
	EmailEnabled   bool `json:"emailEnabled" bson:"emailEnabled"`
	SoundEnabled   bool `json:"soundEnabled" bson:"soundEnabled"`
	MentionsOnly   bool `json:"mentionsOnly" bson:"mentionsOnly"`
	MessagePreview bool `json:"messagePreview" bson:"messagePreview"`
}

type PrivacySettings struct {
	ShowOnlineStatus bool   `json:"showOnlineStatus" bson:"showOnlineStatus"`
	ShowLastSeen     bool   `json:"showLastSeen" bson:"showLastSeen"`
	AllowContactBy   string `json:"allowContactBy" bson:"allowContactBy"` // "everyone", "contacts", "none"
}

type MessagePrefs struct {
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

// ============ OPTIMIZED CHAT MODEL WITH EMBEDDED PARTICIPANT DATA ============
// Embeds participant info and last message to reduce queries
type OptimizedChat struct {
	ChatID      bson.ObjectID `json:"_id,omitempty" bson:"_id,omitempty"`
	Type        string        `json:"type" bson:"type"` // "direct", "group", "channel"
	Name        string        `json:"name,omitempty" bson:"name,omitempty"`
	Description string        `json:"description,omitempty" bson:"description,omitempty"`
	Avatar      string        `json:"avatar,omitempty" bson:"avatar,omitempty"`

	// Embedded participant data to avoid joins
	Participants map[string]ParticipantEmbed `json:"participants" bson:"participants"` // userID -> participant data

	// Owner/Admin info
	OwnerID  bson.ObjectID   `json:"ownerId,omitempty" bson:"ownerId,omitempty"`
	AdminIDs []bson.ObjectID `json:"adminIds" bson:"adminIds"`

	// Last message embedded for quick access
	LastMessage LastMessageEmbed `json:"lastMessage" bson:"lastMessage"`

	// Aggregated data
	Stats ChatStatsEmbed `json:"stats" bson:"stats"`

	// Settings
	Settings ChatSettingsEmbed `json:"settings" bson:"settings"`

	// Read receipts aggregated
	ReadReceipts map[string]time.Time `json:"readReceipts" bson:"readReceipts"` // userID -> last read timestamp

	// Typing indicators (TTL: 10 seconds)
	TypingUsers map[string]time.Time `json:"typingUsers" bson:"typingUsers"` // userID -> typing timestamp

	// Timestamps
	CreatedAt time.Time `json:"createdAt" bson:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt" bson:"updatedAt"`
}

type ParticipantEmbed struct {
	UserID      bson.ObjectID `json:"userId" bson:"userId"`
	Username    string        `json:"username" bson:"username"`
	DisplayName string        `json:"displayName" bson:"displayName"`
	Avatar      string        `json:"avatar" bson:"avatar"`
	Role        string        `json:"role" bson:"role"` // "member", "admin", "owner"
	JoinedAt    time.Time     `json:"joinedAt" bson:"joinedAt"`
	LastActive  time.Time     `json:"lastActive" bson:"lastActive"`
	IsMuted     bool          `json:"isMuted" bson:"isMuted"`
	Permissions []string      `json:"permissions" bson:"permissions"`
}

type LastMessageEmbed struct {
	MessageID   bson.ObjectID `json:"messageId" bson:"messageId"`
	Content     string        `json:"content" bson:"content"`
	SenderID    bson.ObjectID `json:"senderId" bson:"senderId"`
	SenderName  string        `json:"senderName" bson:"senderName"`
	MessageType string        `json:"messageType" bson:"messageType"`
	CreatedAt   time.Time     `json:"createdAt" bson:"createdAt"`
	IsDeleted   bool          `json:"isDeleted" bson:"isDeleted"`
}

type ChatStatsEmbed struct {
	MessageCount     int64          `json:"messageCount" bson:"messageCount"`
	ParticipantCount int            `json:"participantCount" bson:"participantCount"`
	FileCount        int64          `json:"fileCount" bson:"fileCount"`
	ImageCount       int64          `json:"imageCount" bson:"imageCount"`
	UnreadCount      map[string]int `json:"unreadCount" bson:"unreadCount"` // userID -> unread count
}

type ChatSettingsEmbed struct {
	IsPrivate        bool `json:"isPrivate" bson:"isPrivate"`
	AllowInvites     bool `json:"allowInvites" bson:"allowInvites"`
	AllowFileSharing bool `json:"allowFileSharing" bson:"allowFileSharing"`
	MessageRetention int  `json:"messageRetention" bson:"messageRetention"` // days, 0 = forever
	MaxParticipants  int  `json:"maxParticipants" bson:"maxParticipants"`
}

// ============ OPTIMIZED MESSAGE MODEL WITH EMBEDDED THREAD DATA ============
// Embeds thread replies and reactions to reduce queries
type OptimizedMessage struct {
	ID       bson.ObjectID `json:"_id,omitempty" bson:"_id,omitempty"`
	ChatID   bson.ObjectID `json:"chatId" bson:"chatId"`
	SenderID bson.ObjectID `json:"senderId" bson:"senderId"`

	// Embedded sender info to avoid user lookup
	Sender MessageSenderEmbed `json:"sender" bson:"sender"`

	// Message content
	Content     string `json:"content" bson:"content"`
	MessageType string `json:"messageType" bson:"messageType"` // "text", "image", "file", "audio", "video"

	// Thread/Reply data embedded
	Thread MessageThreadEmbed `json:"thread" bson:"thread"`

	// File/Media embedded
	Attachments []AttachmentEmbed `json:"attachments,omitempty" bson:"attachments,omitempty"`

	// Reactions aggregated
	Reactions map[string]ReactionEmbed `json:"reactions" bson:"reactions"` // emoji -> reaction data

	// Mentions
	Mentions MentionsEmbed `json:"mentions" bson:"mentions"`

	// Message status
	Status MessageStatusEmbed `json:"status" bson:"status"`

	// Read receipts (for group chats)
	ReadBy map[string]time.Time `json:"readBy" bson:"readBy"` // userID -> read timestamp

	// Search optimization
	SearchContent string   `json:"-" bson:"searchContent"` // Lowercased content
	SearchTags    []string `json:"-" bson:"searchTags"`    // Extracted hashtags, mentions

	// Timestamps
	CreatedAt time.Time `json:"createdAt" bson:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt" bson:"updatedAt"`
}

type MessageSenderEmbed struct {
	UserID      bson.ObjectID `json:"userId" bson:"userId"`
	Username    string        `json:"username" bson:"username"`
	DisplayName string        `json:"displayName" bson:"displayName"`
	Avatar      string        `json:"avatar" bson:"avatar"`
}

type MessageThreadEmbed struct {
	ParentID    *bson.ObjectID `json:"parentId,omitempty" bson:"parentId,omitempty"`
	ThreadID    *bson.ObjectID `json:"threadId,omitempty" bson:"threadId,omitempty"`
	ReplyCount  int            `json:"replyCount" bson:"replyCount"`
	LastReplyAt *time.Time     `json:"lastReplyAt,omitempty" bson:"lastReplyAt,omitempty"`
	// Embed recent replies to avoid separate queries
	RecentReplies []MessageReplyEmbed `json:"recentReplies,omitempty" bson:"recentReplies,omitempty"`
}

type MessageReplyEmbed struct {
	ID        bson.ObjectID `json:"id" bson:"id"`
	Content   string        `json:"content" bson:"content"`
	SenderID  bson.ObjectID `json:"senderId" bson:"senderId"`
	Username  string        `json:"username" bson:"username"`
	CreatedAt time.Time     `json:"createdAt" bson:"createdAt"`
}

type AttachmentEmbed struct {
	ID           bson.ObjectID `json:"id" bson:"id"`
	OriginalName string        `json:"originalName" bson:"originalName"`
	URL          string        `json:"url" bson:"url"`
	MimeType     string        `json:"mimeType" bson:"mimeType"`
	Size         int64         `json:"size" bson:"size"`
	Width        *int          `json:"width,omitempty" bson:"width,omitempty"`
	Height       *int          `json:"height,omitempty" bson:"height,omitempty"`
	Duration     *int          `json:"duration,omitempty" bson:"duration,omitempty"`
}

type ReactionEmbed struct {
	Count   int               `json:"count" bson:"count"`
	Users   []bson.ObjectID   `json:"users" bson:"users"`
	Details map[string]string `json:"details" bson:"details"` // userID -> username for quick display
}

type MentionsEmbed struct {
	UserIDs          []bson.ObjectID `json:"userIds" bson:"userIds"`
	UserNames        []string        `json:"userNames" bson:"userNames"` // Cached for display
	RoleIDs          []string        `json:"roleIds" bson:"roleIds"`
	IsChannelMention bool            `json:"isChannelMention" bson:"isChannelMention"`
}

type MessageStatusEmbed struct {
	IsEdited  bool       `json:"isEdited" bson:"isEdited"`
	IsDeleted bool       `json:"isDeleted" bson:"isDeleted"`
	IsPinned  bool       `json:"isPinned" bson:"isPinned"`
	EditedAt  *time.Time `json:"editedAt,omitempty" bson:"editedAt,omitempty"`
	DeletedAt *time.Time `json:"deletedAt,omitempty" bson:"deletedAt,omitempty"`
}

// ============ OPTIMIZED CONTACT MODEL - DENORMALIZED ============
// Single model for all contact-related operations
type OptimizedContact struct {
	UserID bson.ObjectID `json:"userId" bson:"userId"`

	// All contact relationships in one document
	Relationships map[string]ContactRelationship `json:"relationships" bson:"relationships"` // targetUserID -> relationship

	// Quick lookup arrays (duplicated for performance)
	ActiveContacts []bson.ObjectID `json:"activeContacts" bson:"activeContacts"`
	BlockedUsers   []bson.ObjectID `json:"blockedUsers" bson:"blockedUsers"`
	PendingOut     []bson.ObjectID `json:"pendingOut" bson:"pendingOut"`
	PendingIn      []bson.ObjectID `json:"pendingIn" bson:"pendingIn"`
	Favorites      []bson.ObjectID `json:"favorites" bson:"favorites"`

	// Cached stats
	Stats ContactStatsEmbed `json:"stats" bson:"stats"`

	// Recent activity for sorting
	RecentInteractions []RecentInteractionEmbed `json:"recentInteractions" bson:"recentInteractions"`

	// Timestamps
	CreatedAt time.Time `json:"createdAt" bson:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt" bson:"updatedAt"`
}

type ContactRelationship struct {
	TargetUserID bson.ObjectID `json:"targetUserId" bson:"targetUserId"`
	Status       string        `json:"status" bson:"status"` // "active", "blocked", "pending_out", "pending_in"
	RequestedBy  bson.ObjectID `json:"requestedBy" bson:"requestedBy"`
	IsFavorite   bool          `json:"isFavorite" bson:"isFavorite"`

	// Embedded user data for quick access
	UserInfo ContactUserInfo `json:"userInfo" bson:"userInfo"`

	// Interaction stats
	MessageCount    int64     `json:"messageCount" bson:"messageCount"`
	LastInteraction time.Time `json:"lastInteraction" bson:"lastInteraction"`

	// Timestamps
	CreatedAt  time.Time  `json:"createdAt" bson:"createdAt"`
	AcceptedAt *time.Time `json:"acceptedAt,omitempty" bson:"acceptedAt,omitempty"`
}

type ContactUserInfo struct {
	Username    string    `json:"username" bson:"username"`
	DisplayName string    `json:"displayName" bson:"displayName"`
	Avatar      string    `json:"avatar" bson:"avatar"`
	Status      string    `json:"status" bson:"status"`
	IsOnline    bool      `json:"isOnline" bson:"isOnline"`
	LastSeen    time.Time `json:"lastSeen" bson:"lastSeen"`
}

type ContactStatsEmbed struct {
	TotalContacts   int `json:"totalContacts" bson:"totalContacts"`
	BlockedCount    int `json:"blockedCount" bson:"blockedCount"`
	PendingOutCount int `json:"pendingOutCount" bson:"pendingOutCount"`
	PendingInCount  int `json:"pendingInCount" bson:"pendingInCount"`
	FavoriteCount   int `json:"favoriteCount" bson:"favoriteCount"`
}

type RecentInteractionEmbed struct {
	UserID        bson.ObjectID `json:"userId" bson:"userId"`
	LastMessageAt time.Time     `json:"lastMessageAt" bson:"lastMessageAt"`
	MessageCount  int           `json:"messageCount" bson:"messageCount"` // in last 24h
}

// ============ BATCH OPERATIONS MODELS ============
// Models designed for efficient batch operations

type BatchMessageOperation struct {
	Operation string               `json:"operation" bson:"operation"` // "insert", "update", "delete"
	Messages  []OptimizedMessage   `json:"messages" bson:"messages"`
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

type BatchNotification struct {
	UserID        bson.ObjectID      `json:"userId" bson:"userId"`
	Notifications []NotificationItem `json:"notifications" bson:"notifications"`
	TotalCount    int                `json:"totalCount" bson:"totalCount"`
	UnreadCount   int                `json:"unreadCount" bson:"unreadCount"`
	CreatedAt     time.Time          `json:"createdAt" bson:"createdAt"`
}

type NotificationItem struct {
	ID        bson.ObjectID  `json:"id" bson:"id"`
	Type      string         `json:"type" bson:"type"`
	Title     string         `json:"title" bson:"title"`
	Message   string         `json:"message" bson:"message"`
	ChatID    *bson.ObjectID `json:"chatId,omitempty" bson:"chatId,omitempty"`
	SenderID  *bson.ObjectID `json:"senderId,omitempty" bson:"senderId,omitempty"`
	IsRead    bool           `json:"isRead" bson:"isRead"`
	CreatedAt time.Time      `json:"createdAt" bson:"createdAt"`
	ExpiresAt *time.Time     `json:"expiresAt,omitempty" bson:"expiresAt,omitempty"`
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
	Chat         OptimizedChat       `json:"chat"`
	Messages     []OptimizedMessage  `json:"messages"`
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

// ============ CONSTANTS AND ENUMS ============

const (
	// Chat Types
	ChatTypeDirect  = "direct"
	ChatTypeGroup   = "group"
	ChatTypeChannel = "channel"

	// Message Types
	MessageTypeText     = "text"
	MessageTypeImage    = "image"
	MessageTypeFile     = "file"
	MessageTypeAudio    = "audio"
	MessageTypeVideo    = "video"
	MessageTypeLocation = "location"

	// Contact Status
	ContactStatusActive     = "active"
	ContactStatusBlocked    = "blocked"
	ContactStatusPendingOut = "pending_out"
	ContactStatusPendingIn  = "pending_in"

	// User Status
	UserStatusOnline    = "online"
	UserStatusAway      = "away"
	UserStatusDND       = "dnd"
	UserStatusInvisible = "invisible"
	UserStatusOffline   = "offline"

	// Notification Types
	NotificationTypeMessage       = "message"
	NotificationTypeMention       = "mention"
	NotificationTypeFriendRequest = "friend_request"
	NotificationTypeGroupInvite   = "group_invite"
	NotificationTypeCall          = "call"

	// Cache TTL (seconds)
	CacheUserProfile     = 3600 // 1 hour
	CacheChatList        = 1800 // 30 minutes
	CachePresence        = 300  // 5 minutes
	CacheTypingIndicator = 10   // 10 seconds
)

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
