package models

import "time"

type Report struct {
	ReportId       string `json:"report_id" bson:"report_id"`
	ReportedUserId string `json:"reported_user_id" bson:"reported_user_id"`
	ReportedBy     string `json:"reported_by" bson:"reported_by"`
	Reason         string `json:"reason" bson:"reason"`
	CreatedAt      string `json:"created_at" bson:"created_at"`
}

type ModerationActions struct {
	ActionId    string    `json:"action_id" bson:"action_id"`
	UserId      string    `json:"user_id" bson:"user_id"`
	ActionType  string    `json:"action_type" bson:"action_type"`
	Description string    `json:"description" bson:"description"`
	CreatedAt   time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" bson:"updated_at"`
	IsActive    bool      `json:"is_active" bson:"is_active"`
}

type Bans struct {
	BanId    int       `json:"ban_id" bson:"ban_id"`
	GroupId  int       `json:"group_id" bson:"group_id"`
	UserId   int       `json:"user_id" bson:"user_id"`
	Reason   string    `json:"reason" bson:"reason"`
	BannedAt time.Time `json:"banned_at" bson:"banned_at"`
}
