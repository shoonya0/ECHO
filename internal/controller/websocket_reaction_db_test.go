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

// A member sending an emoji that would reach other fields (#58) gets
// INVALID_REQUEST, and no reaction event goes to the chat.
func TestReactionWithUnsafeEmojiIsInvalidRequest(t *testing.T) {
	ctx := testutil.UseTestDB(t)

	broadcaster := &countingBroadcaster{}
	services.SetBroadcaster(broadcaster)
	t.Cleanup(func() { services.SetBroadcaster(nil) })

	member := bson.NewObjectID()
	chatID, messageID := bson.NewObjectID(), bson.NewObjectID()
	if _, err := database.DB.Collection(string(constants.UserColl)).InsertOne(ctx, bson.M{"_id": member, "username": "member"}); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{
		"_id":          chatID,
		"participants": bson.M{member.Hex(): bson.M{"status": "accepted"}},
	}); err != nil {
		t.Fatalf("insert chat: %v", err)
	}
	if _, err := database.DB.Collection(string(constants.MessageColl)).InsertOne(ctx, bson.M{
		"_id": messageID, "chatId": chatID, "senderId": member, "content": "hi",
	}); err != nil {
		t.Fatalf("insert message: %v", err)
	}

	for _, requestType := range []string{models.WSRequestTypeAddReaction, models.WSRequestTypeRemoveReaction} {
		t.Run(requestType, func(t *testing.T) {
			broadcaster.published = 0
			client := &models.Client{ID: "member-conn", UserID: member, Send: make(chan models.WebSocketMessage, 4)}
			handleClientRequest(context.Background(), client, models.MessageRequest{
				Type:      requestType,
				ChatID:    chatID.Hex(),
				RequestID: "req-1",
				Metadata:  map[string]any{"messageId": messageID.Hex(), "emoji": "ok.details"},
			})

			if broadcaster.published != 0 {
				t.Errorf("published %d events for a refused emoji", broadcaster.published)
			}
			select {
			case reply := <-client.Send:
				errMsg, ok := reply.Data.(models.ErrorMessage)
				if !ok {
					t.Fatalf("reply = %+v, want an error", reply)
				}
				if errMsg.Code != "INVALID_REQUEST" {
					t.Errorf("code = %q (%s), want INVALID_REQUEST", errMsg.Code, errMsg.Message)
				}
			default:
				t.Fatal("no reply sent")
			}
		})
	}
}
