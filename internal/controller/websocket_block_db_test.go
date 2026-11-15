package controller

import (
	"context"
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/services"
	"github.com/shoonya0/ECHO/internal/testutil"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// #27: a blocked user could keep messaging the blocker in their existing direct
// chat. Now neither side of a block can send there; nothing is stored or published.
func TestSendMessageInDirectChatRefusedWhenBlocked(t *testing.T) {
	ctx := testutil.UseTestDB(t)

	broadcaster := &countingBroadcaster{}
	services.SetBroadcaster(broadcaster)
	t.Cleanup(func() { services.SetBroadcaster(nil) })

	a, b := bson.NewObjectID(), bson.NewObjectID()
	chatID := bson.NewObjectID()
	users := database.DB.Collection(string(constants.UserColl))
	for _, id := range []bson.ObjectID{a, b} {
		if _, err := users.InsertOne(ctx, bson.M{"_id": id, "username": id.Hex()}); err != nil {
			t.Fatalf("insert user: %v", err)
		}
	}
	member := bson.M{"role": string(constants.ChatRoleMember)}
	if _, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{
		"_id":          chatID,
		"chatType":     string(constants.ChatTypeDirect),
		"participants": bson.M{a.Hex(): member, b.Hex(): member},
	}); err != nil {
		t.Fatalf("insert chat: %v", err)
	}
	if err := services.BlockUnblockUser(ctx, a, b, string(constants.StatusBlocked)); err != nil {
		t.Fatalf("A blocks B: %v", err)
	}

	for name, sender := range map[string]bson.ObjectID{"blocked user": b, "blocker": a} {
		t.Run(name, func(t *testing.T) {
			broadcaster.published = 0
			client := &models.Client{ID: "conn", UserID: sender, Send: make(chan models.WebSocketMessage, 4)}
			handleClientRequest(context.Background(), client, models.MessageRequest{
				Type: models.WSRequestTypeSendMessage, ChatID: chatID.Hex(), RequestID: "req-1", Content: "hello",
			})

			if broadcaster.published != 0 {
				t.Errorf("published %d events", broadcaster.published)
			}
			select {
			case reply := <-client.Send:
				errMsg, ok := reply.Data.(models.ErrorMessage)
				if !ok || errMsg.Code != "PERMISSION_DENIED" || errMsg.Message != "User is blocked" {
					t.Errorf("reply = %+v, want PERMISSION_DENIED \"User is blocked\"", reply.Data)
				}
			default:
				t.Fatal("no reply sent")
			}
		})
	}

	n, err := database.DB.Collection(string(constants.MessageColl)).CountDocuments(ctx, bson.M{"chatId": chatID})
	if err != nil || n != 0 {
		t.Fatalf("messages stored: n=%d err=%v, want 0", n, err)
	}
}
