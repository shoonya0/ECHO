package models

type Group struct {
	GroupId     string            `json:"group_id" bson:"group_id"`
	Members     []string          `json:"members" bson:"members"`
	LastMsgId   string            `json:"last_msg_id" bson:"last_msg_id"`
	Role        map[string]string `json:"role" bson:"role"`
	Description string            `json:"description" bson:"description"`
	CreatedAt   string            `json:"created_at" bson:"created_at"`
	UpdatedAt   string            `json:"updated_at" bson:"updated_at"`
}
type Table struct {
	InviteId  string `json:"invite_id" bson:"invite_id"`
	GroupId   string `json:"group_id" bson:"group_id"`
	Code      string `json:"code" bson:"code"`
	ExpiresAt string `json:"expires_at" bson:"expires_at"` // Expiration time for the invite
}
