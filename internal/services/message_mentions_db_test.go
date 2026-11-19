package services

import (
	"slices"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// #40: a message keeps only mentions of the chat's participants, each once, with
// user IDs and usernames in step. Others were stored, so history showed the
// username of any account whose ID a member sent.
func TestSendMessageKeepsOnlyParticipantMentions(t *testing.T) {
	ctx := useTestDB(t)
	SetBroadcaster(&recordingBroadcaster{})
	t.Cleanup(func() { SetBroadcaster(nil) })

	owner, member, outsider := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, owner, member, outsider)
	chat := newTestGroup(t, ctx, owner, member)

	mentions := []string{
		member.Hex(),
		outsider.Hex(),           // has an account, isn't in the chat
		bson.NewObjectID().Hex(), // no such account
		"not-an-id",
		member.Hex(), // twice
		owner.Hex(),
	}
	msg, err := SendMessage(ctx, chat.ChatID, owner, "hi", "text", nil, mentions)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	wantIDs := []bson.ObjectID{member, owner}
	wantNames := []string{member.Hex(), owner.Hex()} // insertTestUsers names users by ID
	if !slices.Equal(msg.Mentions.UserIDs, wantIDs) || !slices.Equal(msg.Mentions.UserNames, wantNames) {
		t.Fatalf("mentions = %v %v, want %v %v", msg.Mentions.UserIDs, msg.Mentions.UserNames, wantIDs, wantNames)
	}
}
