package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// here user id 1 and user id 2 save the refference of the user from the user collection
type Contact struct {
	ID        bson.ObjectID `json:"id" bson:"_id,omitempty"`
	ID1       string        `json:"id_1" bson:"id_1"`             // this is the user id of the user (reff ID) who is the contact
	ID2       string        `json:"id_2" bson:"id_2"`             // this is the user id of the user (reff ID) who is the contact
	Status    string        `json:"status" bson:"status"`         // "pending", "accepted", "blocked"
	Username1 string        `json:"username_1" bson:"username_1"` // this is the username of the user who is the contact
	Username2 string        `json:"username_2" bson:"username_2"` // this is the username of the user who is the contact
	Avatar1   *string       `json:"avatar_1" bson:"avatar_1"`     // this is the avatar of the user who is the contact
	Avatar2   *string       `json:"avatar_2" bson:"avatar_2"`     // this is the avatar of the user who is the contact

	RequestedBy string `json:"requested_by" bson:"requested_by"` // Who sent the friend request

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
