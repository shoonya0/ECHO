package controller

import (
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/testutil"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// #106: a request that checks membership first reads the chat once, not once for
// the check and again for the handler.
func TestChatHandlersReadTheChatOnce(t *testing.T) {
	ctx, reads := testutil.UseTestDBCountingReads(t)

	owner, member, newcomer := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	for _, id := range []bson.ObjectID{owner, member, newcomer} {
		if _, err := database.DB.Collection(string(constants.UserColl)).InsertOne(ctx, bson.M{"_id": id, "username": id.Hex()}); err != nil {
			t.Fatalf("insert user: %v", err)
		}
	}
	participant := func(role string) bson.M {
		return bson.M{"role": role, "requestStatus": "accepted",
			"permissions": []string{string(constants.ChatPermissionRead), string(constants.ChatPermissionWrite)}}
	}
	groupID, directID := bson.NewObjectID(), bson.NewObjectID()
	chats := database.DB.Collection(string(constants.ChatColl))
	if _, err := chats.InsertMany(ctx, []any{
		bson.M{"_id": groupID, "chatType": string(constants.ChatTypeGroup), "ownerId": owner,
			"participants": bson.M{owner.Hex(): participant(string(constants.ChatRoleOwner)), member.Hex(): participant(string(constants.ChatRoleMember))},
			"settings":     bson.M{"maxParticipants": 100}},
		bson.M{"_id": directID, "chatType": string(constants.ChatTypeDirect),
			"participants": bson.M{owner.Hex(): participant(string(constants.ChatRoleMember)), member.Hex(): participant(string(constants.ChatRoleMember))}},
	}); err != nil {
		t.Fatalf("insert chats: %v", err)
	}

	cases := []struct {
		name string
		run  func(client *models.Client)
	}{
		{"send_message in a group", func(c *models.Client) {
			handleSendMessage(ctx, c, models.MessageRequest{ChatID: groupID.Hex(), Content: "hi", RequestID: "r1"})
		}},
		{"send_message in a direct chat", func(c *models.Client) {
			handleSendMessage(ctx, c, models.MessageRequest{ChatID: directID.Hex(), Content: "hi", RequestID: "r1"})
		}},
		{"invite_user", func(c *models.Client) {
			handleInviteUser(ctx, c, models.MessageRequest{ChatID: groupID.Hex(), RequestID: "r1",
				Metadata: map[string]any{"userIds": []any{newcomer.Hex()}}})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &models.Client{ID: "c1", UserID: owner, Send: make(chan models.WebSocketMessage, 4)}
			reads.Reset()
			tc.run(client)

			reply := <-client.Send
			if reply.Type != models.WSMessageTypeResponse {
				t.Fatalf("reply = %+v, want a success response", reply.Data)
			}
			if n := reads.Reads(string(constants.ChatColl)); n != 1 {
				t.Fatalf("chat reads = %d, want 1", n)
			}
		})
	}
}
