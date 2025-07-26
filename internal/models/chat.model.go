package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Chat struct {
	// Hear ID is act as chat id as well as invite code
	ChatID      bson.ObjectID `json:"_id,omitempty" bson:"_id,omitempty"`
	Type        string        `json:"type" bson:"type"` // "direct", "group", "channel"
	Name        *string       `json:"name,omitempty" bson:"name,omitempty"`
	Description *string       `json:"description,omitempty" bson:"description,omitempty"`
	Avatar      *string       `json:"avatar,omitempty" bson:"avatar,omitempty"`

	// Participants
	Participants map[string]string `json:"participants" bson:"participants"` // User IDs and roles

	// Group/Channel specific
	OwnerID  *string  `json:"ownerId,omitempty" bson:"ownerId,omitempty"`
	AdminIDs []string `json:"adminIds" bson:"adminIds"`

	// Last message info for quick retrieval
	LastMessageID *string    `json:"lastMessageId,omitempty" bson:"lastMessageId,omitempty"`
	LastMessageAt *time.Time `json:"lastMessageAt,omitempty" bson:"lastMessageAt,omitempty"`

	// Settings
	IsPrivate bool `json:"isPrivate" bson:"isPrivate"`

	// Metadata
	MessageCount int64     `json:"messageCount" bson:"messageCount"`
	CreatedAt    time.Time `json:"createdAt" bson:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt" bson:"updatedAt"`
}
