package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type User struct {
	ID           bson.ObjectID `json:"id" bson:"_id,omitempty"`
	UserID       string        `json:"user_id" bson:"user_id"` // this Id is from the auth service
	Username     string        `json:"username" bson:"username"`
	Email        string        `json:"email" bson:"email"`
	Phone        *string       `json:"phone,omitempty" bson:"phone,omitempty"`
	PasswordHash string        `json:"-" bson:"password_hash"`

	// Profile information
	DisplayName   string  `json:"display_name" bson:"display_name"`
	Avatar        *string `json:"avatar,omitempty" bson:"avatar,omitempty"`
	Status        string  `json:"status" bson:"status"` // "online", "away", "dnd", "invisible"
	StatusMessage *string `json:"status_message,omitempty" bson:"status_message,omitempty"`
	Bio           *string `json:"bio,omitempty" bson:"bio,omitempty"`

	// Account status
	IsActive   bool `json:"is_active" bson:"is_active"`
	IsVerified bool `json:"is_verified" bson:"is_verified"`
	IsBanned   bool `json:"is_banned" bson:"is_banned"`

	// Privacy settings
	PrivacySettings map[string]interface{} `json:"privacy_settings" bson:"privacy_settings"`

	// Presence tracking
	LastSeen time.Time `json:"last_seen" bson:"last_seen"`
	IsOnline bool      `json:"is_online" bson:"is_online"`

	// Timestamps
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
}

type UserSettings struct {
	UserId        string `json:"user_id" bson:"user_id"`
	Theme         string `json:"theme" bson:"theme"`
	Notifications bool   `json:"notifications" bson:"notifications"`
}
