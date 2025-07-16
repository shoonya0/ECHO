package models

import "time"

// ============ REQUEST/RESPONSE MODELS ============
type SendMessageRequest struct {
	ChatID      string  `json:"chat_id" binding:"required"`
	Content     string  `json:"content" binding:"required"`
	MessageType string  `json:"message_type"` // "text", "image", "file", "audio", "video"
	ParentID    *string `json:"parent_id,omitempty"`
	FileID      *string `json:"file_id,omitempty"`
}

type Pagination struct {
	Page      int    `json:"page"`
	Limit     int    `json:"limit"`
	SortBy    string `json:"sort_by"`
	SortOrder string `json:"sort_order"`
}

// ============ TYPING INDICATOR MODEL ============
type TypingIndicator struct {
	ChatID    string    `json:"chat_id" bson:"chat_id"`
	UserID    string    `json:"user_id" bson:"user_id"`
	IsTyping  bool      `json:"is_typing" bson:"is_typing"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
}

// ============ USER MODEL ============
type UserRegister struct {
	FirstName string `json:"first_name" binding:"required" bson:"first_name"`
	LastName  string `json:"last_name" binding:"required" bson:"last_name"`
	Email     string `json:"email" binding:"required" bson:"email"`
	Password  string `json:"password" binding:"required" bson:"password"`
}

type UserLogin struct {
	Email    string `json:"email" binding:"required" bson:"email"`
	Password string `json:"password" binding:"required" bson:"password"`
}
