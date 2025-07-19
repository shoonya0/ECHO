package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Message struct {
	ID          bson.ObjectID `json:"id" bson:"_id,omitempty"`
	ChatID      string        `json:"chat_id" bson:"chat_id"`     // Group ID or DM thread ID
	ChatType    string        `json:"chat_type" bson:"chat_type"` // "direct" or "group" or "channel"
	SenderID    string        `json:"sender_id" bson:"sender_id"`
	Content     string        `json:"content" bson:"content"`
	MessageType string        `json:"message_type" bson:"message_type"` // "text", "image", "file", "audio", "video"

	// Thread/Reply functionality
	ParentID *string `json:"parent_id,omitempty" bson:"parent_id,omitempty"` // For replies
	ThreadID *string `json:"thread_id,omitempty" bson:"thread_id,omitempty"` // For thread organization means if the message is a reply to a message

	// File/Media information
	FileURL  *string `json:"file_url,omitempty" bson:"file_url,omitempty"`   // file url
	FileName *string `json:"file_name,omitempty" bson:"file_name,omitempty"` // file name
	FileSize *int64  `json:"file_size,omitempty" bson:"file_size,omitempty"` // file size
	MimeType *string `json:"mime_type,omitempty" bson:"mime_type,omitempty"` // file mime type

	// Message status and metadata
	IsEdited  bool       `json:"is_edited" bson:"is_edited"`
	IsDeleted bool       `json:"is_deleted" bson:"is_deleted"`
	IsPinned  bool       `json:"is_pinned" bson:"is_pinned"`
	EditedAt  *time.Time `json:"edited_at,omitempty" bson:"edited_at,omitempty"`

	// Reactions
	Reactions map[string][]string `json:"reactions" bson:"reactions"` // emoji -> [userID1, userID2]

	// Mentions and formatting
	Mentions     []string `json:"mentions" bson:"mentions"`           // User IDs mentioned
	RoleMentions []string `json:"role_mentions" bson:"role_mentions"` // Role IDs mentioned

	// Read receipts
	ReadBy map[string]time.Time `json:"read_by" bson:"read_by"` // userID -> timestamp

	// Timestamps
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`

	// Search optimization
	SearchContent string `json:"-" bson:"search_content"` // Lowercased content for search
}

type Attachment struct {
	ID           bson.ObjectID `json:"id" bson:"_id,omitempty"`
	OriginalName string        `json:"original_name" bson:"original_name"`
	StoragePath  string        `json:"storage_path" bson:"storage_path"`
	URL          string        `json:"url" bson:"url"`
	MimeType     string        `json:"mime_type" bson:"mime_type"`
	Size         int64         `json:"size" bson:"size"`
	UploaderID   string        `json:"uploader_id" bson:"uploader_id"`

	// Image/Video specific metadata
	Width    *int `json:"width,omitempty" bson:"width,omitempty"`
	Height   *int `json:"height,omitempty" bson:"height,omitempty"`
	Duration *int `json:"duration,omitempty" bson:"duration,omitempty"` // For audio/video in seconds

	// Timestamps
	CreatedAt time.Time  `json:"created_at" bson:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty" bson:"expires_at,omitempty"`
}
