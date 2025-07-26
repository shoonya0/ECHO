package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type User struct {
	ID           bson.ObjectID `json:"_id,omitempty" bson:"_id,omitempty"` // this Id is from the auth service
	Username     string        `json:"userName,omitempty" bson:"userName,omitempty"`
	Email        string        `json:"email,omitempty" bson:"email,omitempty"`
	Phone        string        `json:"phone,omitempty" bson:"phone,omitempty"`
	PasswordHash string        `json:"-" bson:"passwordHash,omitempty"`

	// Profile information
	DisplayName   string `json:"displayName,omitempty" bson:"displayName,omitempty"`
	Avatar        string `json:"avatar,omitempty" bson:"avatar,omitempty"`
	Status        string `json:"status,omitempty" bson:"status,omitempty"`               // "online", "away", "dnd", "invisible"
	StatusMessage string `json:"statusMessage,omitempty" bson:"statusMessage,omitempty"` // this is the status message of the user means what the user is doing
	Bio           string `json:"bio,omitempty" bson:"bio,omitempty"`

	// Account status
	IsActive   bool `json:"isActive,omitempty" bson:"isActive,omitempty"`
	IsVerified bool `json:"isVerified,omitempty" bson:"isVerified,omitempty"`
	IsBanned   bool `json:"isBanned,omitempty" bson:"isBanned,omitempty"`

	// Privacy settings
	PrivacySettings map[string]interface{} `json:"privacySettings,omitempty" bson:"privacySettings,omitempty"`

	// Presence tracking
	LastSeen time.Time `json:"lastSeen,omitempty" bson:"lastSeen,omitempty"`
	IsOnline bool      `json:"isOnline,omitempty" bson:"isOnline,omitempty"`

	// Timestamps
	CreatedAt time.Time `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
}

type UserSettings struct {
	UserId        string `json:"userId" bson:"userId"`
	Theme         string `json:"theme" bson:"theme"`
	Notifications bool   `json:"notifications" bson:"notifications"`
}
