package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Friendship struct {
	ID          bson.ObjectID `json:"id" bson:"_id,omitempty"`
	UserID1     string        `json:"user_id_1" bson:"user_id_1"`
	UserID2     string        `json:"user_id_2" bson:"user_id_2"`
	Status      string        `json:"status" bson:"status"`             // "pending", "accepted", "blocked"
	RequestedBy string        `json:"requested_by" bson:"requested_by"` // Who sent the friend request

	// Timestamps
	CreatedAt  time.Time  `json:"created_at" bson:"created_at"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty" bson:"accepted_at,omitempty"`
}

type Invite struct {
	InviteId    string `json:"invite_id" bson:"invite_id"`
	GroupId     string `json:"group_id" bson:"group_id"`
	Code        string `json:"code" bson:"code"`
	Status      string `json:"status" bson:"status"`             // "pending", "accepted", "blocked"
	RequestedBy string `json:"requested_by" bson:"requested_by"` // Who sent the friend request

	// timestamps
	CreatedAt time.Time  `json:"created_at" bson:"created_at"`
	ExpiresAt *time.Time `json:"expires_at" bson:"expires_at"` // Expiration time for the invite
}
