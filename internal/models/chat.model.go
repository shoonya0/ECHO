package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Chat struct {
	ID          bson.ObjectID `json:"id" bson:"_id,omitempty"`
	Type        string        `json:"type" bson:"type"` // "direct", "group", "channel"
	Name        *string       `json:"name,omitempty" bson:"name,omitempty"`
	Description *string       `json:"description,omitempty" bson:"description,omitempty"`
	Avatar      *string       `json:"avatar,omitempty" bson:"avatar,omitempty"`

	// Participants
	Participants map[string]string `json:"participants" bson:"participants"` // User IDs and roles

	// Group/Channel specific
	OwnerID  *string  `json:"owner_id,omitempty" bson:"owner_id,omitempty"`
	AdminIDs []string `json:"admin_ids" bson:"admin_ids"`

	// Last message info for quick retrieval
	LastMessageID *string    `json:"last_message_id,omitempty" bson:"last_message_id,omitempty"`
	LastMessageAt *time.Time `json:"last_message_at,omitempty" bson:"last_message_at,omitempty"`

	// Settings
	IsPrivate  bool    `json:"is_private" bson:"is_private"`
	InviteCode *string `json:"invite_code,omitempty" bson:"invite_code,omitempty"`

	// Metadata
	MessageCount int64     `json:"message_count" bson:"message_count"`
	CreatedAt    time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" bson:"updated_at"`
}
