package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ============ OPTIMIZED MESSAGE MODEL WITH EMBEDDED THREAD DATA ============
// Embeds thread replies and reactions to reduce queries
type Message struct {
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
