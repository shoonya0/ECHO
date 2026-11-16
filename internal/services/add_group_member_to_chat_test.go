package services

import (
	"context"
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// A chat without an ID would make the update target the zero ObjectID and
// report a real group as not found, so it is refused before any database call.
func TestAddGroupMemberToChatRefusesChatWithoutID(t *testing.T) {
	owner := bson.NewObjectID()
	chat := models.Chat{
		ChatType:     string(constants.ChatTypeGroup),
		Participants: map[bson.ObjectID]models.ParticipantEmbed{owner: {Role: string(constants.ChatRoleOwner)}},
	}
	if err := AddGroupMemberToChat(context.Background(), owner, chat, []bson.ObjectID{bson.NewObjectID()}); err == nil {
		t.Fatal("AddGroupMemberToChat with a zero ChatID returned nil, want an error")
	}
}
