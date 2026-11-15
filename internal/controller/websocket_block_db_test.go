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
// chat, and send typing, read and reaction events there. Now neither side of a
// block can do any of these; nothing is stored or published.
func TestDirectChatRequestsRefusedWhenBlocked(t *testing.T) {
	ctx := testutil.UseTestDB(t)

	broadcaster := &countingBroadcaster{}
	services.SetBroadcaster(broadcaster)
	t.Cleanup(func() { services.SetBroadcaster(nil) })

	a, b := bson.NewObjectID(), bson.NewObjectID()
	chatID, messageID := bson.NewObjectID(), bson.NewObjectID()
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
	messages := database.DB.Collection(string(constants.MessageColl))
	if _, err := messages.InsertOne(ctx, bson.M{
		"_id": messageID, "chatId": chatID, "senderId": a, "content": "before the block", "reactions": bson.M{},
	}); err != nil {
		t.Fatalf("insert message: %v", err)
	}
	if err := services.BlockUnblockUser(ctx, a, b, string(constants.StatusBlocked)); err != nil {
		t.Fatalf("A blocks B: %v", err)
	}

	reaction := map[string]interface{}{"messageId": messageID.Hex(), "emoji": "👍"}
	requests := []models.MessageRequest{
		{Type: models.WSRequestTypeSendMessage, Content: "hello"},
		{Type: models.WSRequestTypeSetTyping, Metadata: map[string]interface{}{"isTyping": true}},
		{Type: models.WSRequestTypeMarkRead, Metadata: map[string]interface{}{"messageIds": []interface{}{messageID.Hex()}}},
		{Type: models.WSRequestTypeAddReaction, Metadata: reaction},
		{Type: models.WSRequestTypeRemoveReaction, Metadata: reaction},
	}

	for name, sender := range map[string]bson.ObjectID{"blocked user": b, "blocker": a} {
		for _, request := range requests {
			t.Run(name+"/"+request.Type, func(t *testing.T) {
				broadcaster.published = 0
				request.ChatID = chatID.Hex()
				request.RequestID = "req-1"
				client := &models.Client{ID: "conn", UserID: sender, Send: make(chan models.WebSocketMessage, 4)}
				handleClientRequest(context.Background(), client, request)

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
	}

	if n, err := messages.CountDocuments(ctx, bson.M{"chatId": chatID}); err != nil || n != 1 {
		t.Fatalf("messages in chat: n=%d err=%v, want only the 1 from before the block", n, err)
	}
	var stored struct {
		Reactions map[string]bson.Raw `bson:"reactions"`
		ReadBy    map[string]bson.Raw `bson:"readBy"`
	}
	if err := messages.FindOne(ctx, bson.M{"_id": messageID}).Decode(&stored); err != nil {
		t.Fatalf("load message: %v", err)
	}
	if len(stored.Reactions) != 0 || len(stored.ReadBy) != 0 {
		t.Errorf("reactions = %v, readBy = %v, want both empty", stored.Reactions, stored.ReadBy)
	}
}
