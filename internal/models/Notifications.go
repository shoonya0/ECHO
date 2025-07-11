package models

import "time"

type Notifications struct {
	NotificationId string    `json:"notification_id"`
	UserId         string    `json:"user_id"`
	Message        string    `json:"message"`
	IsRead         bool      `json:"is_read"`
	CreatedAt      time.Time `json:"created_at"`
}
