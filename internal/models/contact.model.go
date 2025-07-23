package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// here user id 1 and user id 2 save the refference of the user from the user collection
// type Contact struct {
// 	ID          bson.ObjectID `json:"id" bson:"_id,omitempty"`
// 	ID1         string        `json:"id_1" bson:"id_1"`                 // this is the user id of the user (reff ID) who is the contact
// 	ID2         string        `json:"id_2" bson:"id_2"`                 // this is the user id of the user (reff ID) who is the contact
// 	Status      string        `json:"status" bson:"status"`             // "pending", "accepted", "blocked"
// 	RequestedBy string        `json:"requested_by" bson:"requested_by"` // Who sent the friend request

// 	// Timestamps
// 	CreatedAt  time.Time  `json:"created_at" bson:"created_at"`
// 	AcceptedAt *time.Time `json:"accepted_at,omitempty" bson:"accepted_at,omitempty"`
// }

type ContactStatus struct {
	Status      string     `json:"status" bson:"status"`             // "pending", "accepted", "blocked", "favorite"
	ChatId      string     `json:"chat_id" bson:"chat_id"`           // this is the mongo chat id of the chat between the two users
	RequestedBy string     `json:"requested_by" bson:"requested_by"` // Who sent the friend request
	AcceptedAt  *time.Time `json:"accepted_at,omitempty" bson:"accepted_at,omitempty"`
}

type Contact struct {
	ID        bson.ObjectID            `json:"id" bson:"_id,omitempty"`
	Contacts  map[string]ContactStatus `json:"contacts" bson:"contacts"`
	CreatedAt time.Time                `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time                `json:"updated_at" bson:"updated_at"`
}

type Invite struct {
	InviteId    string `json:"invite_id" bson:"invite_id"`
	ChatId      string `json:"chat_id" bson:"chat_id"`
	Code        string `json:"code" bson:"code"`
	Status      string `json:"status" bson:"status"`             // "pending", "accepted", "blocked"
	RequestedBy string `json:"requested_by" bson:"requested_by"` // Who sent the friend request

	// timestamps
	CreatedAt time.Time  `json:"created_at" bson:"created_at"`
	ExpiresAt *time.Time `json:"expires_at" bson:"expires_at"` // Expiration time for the invite
}
