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

// The send_message reply carries requestId at the top level, like every other
// reply, so a client that matches replies on that field finds it (#47).
func TestSendMessageReplyHasTopLevelRequestID(t *testing.T) {
	ctx := testutil.UseTestDB(t)

	services.SetBroadcaster(&countingBroadcaster{})
	t.Cleanup(func() { services.SetBroadcaster(nil) })

	member, chatID := bson.NewObjectID(), bson.NewObjectID()
	if _, err := database.DB.Collection(string(constants.UserColl)).InsertOne(ctx, bson.M{"_id": member, "username": "member"}); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{
		"_id":          chatID,
		"chatType":     string(constants.ChatTypeDirect),
		"participants": bson.M{member.Hex(): bson.M{"role": string(constants.ChatRoleMember)}},
	}); err != nil {
		t.Fatalf("insert chat: %v", err)
	}

	client := &models.Client{ID: "conn", UserID: member, Send: make(chan models.WebSocketMessage, 4)}
	handleClientRequest(context.Background(), client, models.MessageRequest{
		Type:      models.WSRequestTypeSendMessage,
		ChatID:    chatID.Hex(),
		Content:   "hello",
		RequestID: "r1",
	})

	select {
	case reply := <-client.Send:
		if reply.Type != models.WSMessageTypeResponse {
			t.Fatalf("reply = %+v, want a response", reply)
		}
		if reply.RequestID != "r1" {
			t.Errorf("top-level requestId = %q, want %q", reply.RequestID, "r1")
		}
		// #43 code review: the sender learns its message's seq even when its own
		// live event is dropped. The chat has no stats, as before #43: seeded, 1.
		resp, ok := reply.Data.(models.MessageResponse)
		if !ok || resp.Data["seq"] != int64(1) {
			t.Errorf("reply data = %+v, want seq 1", reply.Data)
		}
	default:
		t.Fatal("no reply sent")
	}
}
