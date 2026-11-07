package services

import (
	"context"
	"errors"
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// recordingBroadcaster counts published events instead of delivering them.
type recordingBroadcaster struct{ events []models.WebSocketMessage }

func (b *recordingBroadcaster) PublishToChat(_ string, message *models.WebSocketMessage) error {
	b.events = append(b.events, *message)
	return nil
}

func TestChatActionsRequireMembership(t *testing.T) {
	ctx := useTestDB(t)

	rec := &recordingBroadcaster{}
	SetBroadcaster(rec)
	t.Cleanup(func() { SetBroadcaster(nil) })

	member, outsider := bson.NewObjectID(), bson.NewObjectID()
	chatID, messageID := bson.NewObjectID(), bson.NewObjectID()

	for _, id := range []bson.ObjectID{member, outsider} {
		if _, err := database.DB.Collection(string(constants.UserColl)).InsertOne(ctx, bson.M{"_id": id, "username": "user-" + id.Hex()}); err != nil {
			t.Fatalf("insert user: %v", err)
		}
	}
	if _, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{
		"_id":          chatID,
		"participants": bson.M{member.Hex(): bson.M{"status": "accepted"}},
	}); err != nil {
		t.Fatalf("insert chat: %v", err)
	}
	if _, err := database.DB.Collection(string(constants.MessageColl)).InsertOne(ctx, bson.M{
		"_id":      messageID,
		"chatId":   chatID,
		"senderId": member,
		"content":  "private",
	}); err != nil {
		t.Fatalf("insert message: %v", err)
	}

	actions := []struct {
		name string
		call func(userID, chat bson.ObjectID) error
	}{
		{"typing", func(userID, chat bson.ObjectID) error {
			// A member sends false: true starts a 10-second cleanup timer that
			// would outlive this test and its database.
			return UpdateTypingStatus(ctx, chat, userID, userID != member)
		}},
		{"mark read", func(userID, chat bson.ObjectID) error {
			return MarkMessagesAsRead(chat, userID, []bson.ObjectID{messageID})
		}},
		{"add reaction", func(userID, chat bson.ObjectID) error {
			return AddMessageReaction(chat, messageID, userID, "ok")
		}},
		{"remove reaction", func(userID, chat bson.ObjectID) error {
			return RemoveMessageReaction(chat, messageID, userID, "ok")
		}},
	}

	for _, action := range actions {
		t.Run(action.name+": non-member is refused", func(t *testing.T) {
			rec.events = nil
			if err := action.call(outsider, chatID); !errors.Is(err, ErrNotParticipant) {
				t.Fatalf("err = %v, want ErrNotParticipant", err)
			}
			if len(rec.events) != 0 {
				t.Fatalf("published %d events for a non-member", len(rec.events))
			}
			assertUntouched(ctx, t, chatID, messageID, outsider)
		})

		t.Run(action.name+": unknown chat looks the same as a non-member", func(t *testing.T) {
			if err := action.call(member, bson.NewObjectID()); !errors.Is(err, ErrNotParticipant) {
				t.Fatalf("err = %v, want ErrNotParticipant", err)
			}
		})

		t.Run(action.name+": member is allowed", func(t *testing.T) {
			if err := action.call(member, chatID); err != nil {
				t.Fatalf("member refused: %v", err)
			}
		})
	}
}

func TestMarkMessagesAsReadCoversOnlyThisChat(t *testing.T) {
	ctx := useTestDB(t)

	rec := &recordingBroadcaster{}
	SetBroadcaster(rec)
	t.Cleanup(func() { SetBroadcaster(nil) })

	member := bson.NewObjectID()
	chatID, otherChatID := bson.NewObjectID(), bson.NewObjectID()
	ownMessage, otherMessage := bson.NewObjectID(), bson.NewObjectID()

	if _, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{
		"_id":          chatID,
		"participants": bson.M{member.Hex(): bson.M{"requestStatus": "accepted"}},
	}); err != nil {
		t.Fatalf("insert chat: %v", err)
	}
	if _, err := database.DB.Collection(string(constants.MessageColl)).InsertMany(ctx, []any{
		bson.M{"_id": ownMessage, "chatId": chatID},
		bson.M{"_id": otherMessage, "chatId": otherChatID},
	}); err != nil {
		t.Fatalf("insert messages: %v", err)
	}

	t.Run("only IDs from another chat are refused", func(t *testing.T) {
		rec.events = nil
		err := MarkMessagesAsRead(chatID, member, []bson.ObjectID{otherMessage, bson.NewObjectID()})
		if !errors.Is(err, ErrMessageNotFound) {
			t.Fatalf("err = %v, want ErrMessageNotFound", err)
		}
		if len(rec.events) != 0 {
			t.Fatalf("published %d events", len(rec.events))
		}
		count, err := database.DB.Collection(string(constants.ChatColl)).CountDocuments(ctx, bson.M{
			"_id": chatID, "readReceipts." + member.Hex(): bson.M{"$exists": true},
		})
		if err != nil {
			t.Fatalf("check chat: %v", err)
		}
		if count != 0 {
			t.Fatal("read receipt advanced with no message read")
		}
	})

	t.Run("mixed IDs broadcast only this chat's messages", func(t *testing.T) {
		rec.events = nil
		if err := MarkMessagesAsRead(chatID, member, []bson.ObjectID{ownMessage, otherMessage}); err != nil {
			t.Fatalf("MarkMessagesAsRead: %v", err)
		}
		if len(rec.events) != 1 {
			t.Fatalf("published %d events, want 1", len(rec.events))
		}
		data, ok := rec.events[0].Data.(map[string]interface{})
		if !ok {
			t.Fatalf("event data = %T", rec.events[0].Data)
		}
		ids, ok := data["messageIds"].([]string)
		if !ok || len(ids) != 1 || ids[0] != ownMessage.Hex() {
			t.Fatalf("messageIds = %v, want only %s", data["messageIds"], ownMessage.Hex())
		}
	})
}

// assertUntouched fails if userID left any trace on the chat or the message.
func assertUntouched(ctx context.Context, t *testing.T, chatID, messageID, userID bson.ObjectID) {
	t.Helper()

	chats := database.DB.Collection(string(constants.ChatColl))
	count, err := chats.CountDocuments(ctx, bson.M{"_id": chatID, "$or": bson.A{
		bson.M{"typingUsers." + userID.Hex(): bson.M{"$exists": true}},
		bson.M{"readReceipts." + userID.Hex(): bson.M{"$exists": true}},
	}})
	if err != nil {
		t.Fatalf("check chat: %v", err)
	}
	if count != 0 {
		t.Fatal("non-member changed the chat document")
	}

	messages := database.DB.Collection(string(constants.MessageColl))
	count, err = messages.CountDocuments(ctx, bson.M{"_id": messageID, "$or": bson.A{
		bson.M{"readBy." + userID.Hex(): bson.M{"$exists": true}},
		bson.M{"reactions.ok.users": userID},
		bson.M{"reactions.ok.count": bson.M{"$lt": 0}},
	}})
	if err != nil {
		t.Fatalf("check message: %v", err)
	}
	if count != 0 {
		t.Fatal("non-member changed the message document")
	}
}
