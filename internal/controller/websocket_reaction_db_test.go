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

// A reaction request that changes nothing, such as a repeat add or a remove by
// someone who never reacted, still gets a success reply, but the reply says
// changed: false and no reaction event goes to the chat (#25).
func TestReactionNoOpSendsNoEvent(t *testing.T) {
	ctx := testutil.UseTestDB(t)

	broadcaster := &countingBroadcaster{}
	services.SetBroadcaster(broadcaster)
	t.Cleanup(func() { services.SetBroadcaster(nil) })

	alice, bob := bson.NewObjectID(), bson.NewObjectID()
	chatID, messageID := bson.NewObjectID(), bson.NewObjectID()
	for _, id := range []bson.ObjectID{alice, bob} {
		if _, err := database.DB.Collection(string(constants.UserColl)).InsertOne(ctx, bson.M{"_id": id, "username": id.Hex()}); err != nil {
			t.Fatalf("insert user: %v", err)
		}
	}
	if _, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{
		"_id":          chatID,
		"participants": bson.M{alice.Hex(): bson.M{"status": "accepted"}, bob.Hex(): bson.M{"status": "accepted"}},
	}); err != nil {
		t.Fatalf("insert chat: %v", err)
	}
	if _, err := database.DB.Collection(string(constants.MessageColl)).InsertOne(ctx, bson.M{
		"_id": messageID, "chatId": chatID, "senderId": alice, "content": "hi", "reactions": bson.M{},
	}); err != nil {
		t.Fatalf("insert message: %v", err)
	}

	steps := []struct {
		name        string
		user        bson.ObjectID
		requestType string
		wantChanged bool
	}{
		{"alice adds", alice, models.WSRequestTypeAddReaction, true},
		{"alice adds again", alice, models.WSRequestTypeAddReaction, false},
		{"bob removes, never reacted", bob, models.WSRequestTypeRemoveReaction, false},
		{"alice removes", alice, models.WSRequestTypeRemoveReaction, true},
		{"alice removes again", alice, models.WSRequestTypeRemoveReaction, false},
	}
	for _, step := range steps {
		broadcaster.published = 0
		client := &models.Client{ID: "conn", UserID: step.user, Send: make(chan models.WebSocketMessage, 4)}
		handleClientRequest(context.Background(), client, models.MessageRequest{
			Type:      step.requestType,
			ChatID:    chatID.Hex(),
			RequestID: "req-1",
			Metadata:  map[string]any{"messageId": messageID.Hex(), "emoji": "👍"},
		})

		wantEvents := 0
		if step.wantChanged {
			wantEvents = 1
		}
		if broadcaster.published != wantEvents {
			t.Errorf("%s: published %d events, want %d", step.name, broadcaster.published, wantEvents)
		}
		select {
		case reply := <-client.Send:
			resp, ok := reply.Data.(models.MessageResponse)
			if !ok || !resp.Success {
				t.Fatalf("%s: reply = %+v, want a success response", step.name, reply)
			}
			if got, ok := resp.Data["changed"]; !ok || got != step.wantChanged {
				t.Errorf("%s: reply changed = %v (present %v), want %v", step.name, got, ok, step.wantChanged)
			}
		default:
			t.Fatalf("%s: no reply sent", step.name)
		}
	}
}
