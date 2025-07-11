package models

type PrivateMessages struct {
	PrivateMessagemId string `json:"private_message_id" bson:"private_message_id"`
	UserId1           string `json:"user_id_1" bson:"user_id_1"`     // UserId1
	UserId2           string `json:"user_id_2" bson:"user_id_2"`     // UserId2
	LastMsgId         string `json:"last_msg_id" bson:"last_msg_id"` // Last message ID in the conversation
	IsBlock           bool   `json:"is_block" bson:"is_block"`       // Indicates if the conversation is blocked
	CreatedAt         string `json:"created_at" bson:"created_at"`
}
