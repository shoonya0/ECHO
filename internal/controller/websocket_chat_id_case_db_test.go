package controller

import (
	"context"
	"strings"
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/services"
	"github.com/shoonya0/ECHO/internal/testutil"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// recordingBroadcaster keeps the chat ID each event was published to, and the
// chat ID inside the event.
type recordingBroadcaster struct{ channels, eventChatIDs []string }

func (b *recordingBroadcaster) PublishToChat(chatID string, message *models.WebSocketMessage) error {
	b.channels = append(b.channels, chatID)
	b.eventChatIDs = append(b.eventChatIDs, message.ChatID)
	return nil
}

// A chatId in uppercase hex parses to the same chat, so the change is saved. Its
// event must go to the lowercase channel members follow, not one named after the
// uppercase text, which nobody follows (#60).
func TestUppercaseChatIDEventsReachTheChat(t *testing.T) {
	ctx := testutil.UseTestDB(t)

	broadcaster := &recordingBroadcaster{}
	services.SetBroadcaster(broadcaster)
	t.Cleanup(func() { services.SetBroadcaster(nil) })

	member, chatID, messageID := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
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
	if _, err := database.DB.Collection(string(constants.MessageColl)).InsertOne(ctx, bson.M{
		"_id": messageID, "chatId": chatID, "senderId": member, "content": "hi", "reactions": bson.M{},
	}); err != nil {
		t.Fatalf("insert message: %v", err)
	}

	upper := strings.ToUpper(chatID.Hex())
	client := &models.Client{ID: "conn", UserID: member, Send: make(chan models.WebSocketMessage, 4)}
	handleClientRequest(context.Background(), client, models.MessageRequest{
		Type:      models.WSRequestTypeAddReaction,
		ChatID:    upper,
		RequestID: "r1",
		Metadata:  map[string]any{"messageId": messageID.Hex(), "emoji": "👍"},
	})

	if reply := <-client.Send; reply.Type != models.WSMessageTypeResponse {
		t.Fatalf("reply = %+v, want a success response", reply)
	}
	if len(broadcaster.channels) != 1 {
		t.Fatalf("published %d events, want 1", len(broadcaster.channels))
	}
	if got := broadcaster.channels[0]; got != chatID.Hex() {
		t.Errorf("published to chat %q, want %q", got, chatID.Hex())
	}
	if got := broadcaster.eventChatIDs[0]; got != chatID.Hex() {
		t.Errorf("event chatId = %q, want %q", got, chatID.Hex())
	}
}
