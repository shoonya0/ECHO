package models

import "time"

type ModerationActions struct {
	ActionId    string    `json:"action_id"`
	UserId      string    `json:"user_id"`
	ActionType  string    `json:"action_type"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	IsActive    bool      `json:"is_active"`
}
