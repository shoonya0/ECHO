package services

import (
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestGetUserChatsRequiresParticipant covers #9. The hub subscribes a client
// to every chat GetUserChats returns. Before the fix, a forged invite code
// added a group to the user's chats list without making them a participant,
// and those entries are still in the database, so the list alone can't be
// trusted.
func TestGetUserChatsRequiresParticipant(t *testing.T) {
	ctx := useTestDB(t)

	user, other := bson.NewObjectID(), bson.NewObjectID()
	member, planted, missing := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()

	if _, err := database.DB.Collection(string(constants.UserColl)).InsertOne(ctx, bson.M{
		"_id": user,
		"chats": bson.M{
			member.Hex():  0, // a chat the user is in
			planted.Hex(): 0, // left by a forged invite: not a participant
			missing.Hex(): 0, // a chat that no longer exists
		},
	}); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := database.DB.Collection(string(constants.ChatColl)).InsertMany(ctx, []any{
		bson.M{"_id": member, "participants": bson.M{user.Hex(): bson.M{}, other.Hex(): bson.M{}}},
		bson.M{"_id": planted, "participants": bson.M{other.Hex(): bson.M{}}},
	}); err != nil {
		t.Fatalf("insert chats: %v", err)
	}

	chats, err := GetUserChats(ctx, user)
	if err != nil {
		t.Fatalf("GetUserChats: %v", err)
	}
	if len(chats) != 1 || chats[0] != member {
		t.Fatalf("GetUserChats = %v, want only the chat the user is a participant of (%s)", chats, member.Hex())
	}
}
