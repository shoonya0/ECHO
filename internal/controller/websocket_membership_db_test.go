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

// countingBroadcaster counts published events instead of delivering them.
type countingBroadcaster struct{ published int }

func (b *countingBroadcaster) PublishToChat(string, *models.WebSocketMessage) error {
	b.published++
	return nil
}

// TestChatRequestsFromNonMemberAreDenied sends every request that takes a
// chatId as a user who isn't in the chat. join_chat isn't listed: it ignores
// chatId (#48). edit_message, delete_message and update_chat aren't built yet.
func TestChatRequestsFromNonMemberAreDenied(t *testing.T) {
	ctx := testutil.UseTestDB(t)

	broadcaster := &countingBroadcaster{}
	services.SetBroadcaster(broadcaster)
	t.Cleanup(func() { services.SetBroadcaster(nil) })

	member, outsider := bson.NewObjectID(), bson.NewObjectID()
	chatID, messageID := bson.NewObjectID(), bson.NewObjectID()

	if _, err := database.DB.Collection(string(constants.UserColl)).InsertOne(ctx, bson.M{"_id": outsider, "username": "outsider"}); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{
		"_id":      chatID,
		"chatType": string(constants.ChatTypeGroup),
		"participants": bson.M{member.Hex(): bson.M{
			"role":   string(constants.ChatRoleOwner),
			"status": "accepted",
		}},
	}); err != nil {
		t.Fatalf("insert chat: %v", err)
	}
	if _, err := database.DB.Collection(string(constants.MessageColl)).InsertOne(ctx, bson.M{
		"_id": messageID, "chatId": chatID, "senderId": member, "content": "private",
	}); err != nil {
		t.Fatalf("insert message: %v", err)
	}

	reaction := map[string]interface{}{"messageId": messageID.Hex(), "emoji": "ok"}
	users := map[string]interface{}{"userIds": []interface{}{member.Hex()}}

	requests := []models.MessageRequest{
		{Type: models.WSRequestTypeSendMessage, Content: "hello"},
		{Type: models.WSRequestTypeLeaveChat},
		{Type: models.WSRequestTypeSetTyping, Metadata: map[string]interface{}{"isTyping": true}},
		{Type: models.WSRequestTypeMarkRead, Metadata: map[string]interface{}{"messageIds": []interface{}{messageID.Hex()}}},
		{Type: models.WSRequestTypeAddReaction, Metadata: reaction},
		{Type: models.WSRequestTypeRemoveReaction, Metadata: reaction},
		{Type: models.WSRequestTypeInviteUser, Metadata: users},
		{Type: models.WSRequestTypeRemoveUser, Metadata: users},
	}

	for _, request := range requests {
		t.Run(request.Type, func(t *testing.T) {
			broadcaster.published = 0
			request.ChatID = chatID.Hex()
			request.RequestID = "req-1"

			client := &models.Client{ID: "outsider-conn", UserID: outsider, Send: make(chan models.WebSocketMessage, 4)}
			handleClientRequest(context.Background(), client, request)

			if broadcaster.published != 0 {
				t.Errorf("published %d events for a non-member", broadcaster.published)
			}

			select {
			case reply := <-client.Send:
				errMsg, ok := reply.Data.(models.ErrorMessage)
				if !ok {
					t.Fatalf("reply = %+v, want an error", reply)
				}
				if errMsg.Code != "PERMISSION_DENIED" {
					t.Errorf("code = %q, want PERMISSION_DENIED", errMsg.Code)
				}
				if reply.RequestID != "req-1" {
					t.Errorf("requestId = %q, want req-1", reply.RequestID)
				}
			default:
				t.Fatal("no reply sent")
			}
		})
	}
}
