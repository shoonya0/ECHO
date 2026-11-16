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

	send := func(chatID bson.ObjectID) func(*models.Client) {
		return func(c *models.Client) {
			handleSendMessage(ctx, c, models.MessageRequest{ChatID: chatID.Hex(), Content: "hi", RequestID: "r1"})
		}
	}
	invite := func(chatID bson.ObjectID) func(*models.Client) {
		return func(c *models.Client) {
			handleInviteUser(ctx, c, models.MessageRequest{ChatID: chatID.Hex(), RequestID: "r1",
				Metadata: map[string]any{"userIds": []any{newcomer.Hex()}}})
		}
	}
	outsider := bson.NewObjectID()

	// wantCode "" is a success reply. The error cases keep the codes they had
	// before #106; a non-member gets PERMISSION_DENIED whether or not the chat
	// exists (#59).
	cases := []struct {
		name     string
		caller   bson.ObjectID
		run      func(client *models.Client)
		wantCode string
	}{
		{"send_message in a group", owner, send(groupID), ""},
		{"send_message in a direct chat", owner, send(directID), ""},
		{"send_message by a non-member", outsider, send(groupID), "PERMISSION_DENIED"},
		{"send_message to a missing chat", owner, send(bson.NewObjectID()), "PERMISSION_DENIED"},
		{"invite_user", owner, invite(groupID), ""},
		{"invite_user by a plain member", member, invite(groupID), "PERMISSION_DENIED"},
		{"invite_user to a direct chat", owner, invite(directID), "INVALID_REQUEST"},
		{"invite_user by a non-member", outsider, invite(groupID), "PERMISSION_DENIED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &models.Client{ID: "c1", UserID: tc.caller, Send: make(chan models.WebSocketMessage, 4)}
			reads.Reset()
			tc.run(client)

			if tc.wantCode == "" {
				if reply := <-client.Send; reply.Type != models.WSMessageTypeResponse {
					t.Fatalf("reply = %+v, want a success response", reply.Data)
				}
			} else if data := errorData(t, client); data.Code != tc.wantCode {
				t.Fatalf("code = %q (%q), want %q", data.Code, data.Message, tc.wantCode)
			}
			if n := reads.Reads(string(constants.ChatColl)); n != 1 {
				t.Fatalf("chat reads = %d, want 1", n)
			}
		})
	}
}
