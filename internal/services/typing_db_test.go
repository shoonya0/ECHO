package services

import (
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// set_typing only broadcasts: storing it cost a MongoDB write per event, and
// the 10-second timer that cleared it removed users who were still typing (#40).
func TestTypingIsBroadcastNotStored(t *testing.T) {
	ctx := useTestDB(t)

	rec := &recordingBroadcaster{}
	SetBroadcaster(rec)
	t.Cleanup(func() { SetBroadcaster(nil) })

	member, chatID := bson.NewObjectID(), bson.NewObjectID()
	if _, err := database.DB.Collection(string(constants.UserColl)).InsertOne(ctx, bson.M{"_id": member, "username": "typer"}); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{
		"_id":          chatID,
		"participants": bson.M{member.Hex(): bson.M{"status": "accepted"}},
	}); err != nil {
		t.Fatalf("insert chat: %v", err)
	}

	if err := UpdateTypingStatus(ctx, chatID, member, true); err != nil {
		t.Fatalf("UpdateTypingStatus: %v", err)
	}

	if len(rec.events) != 1 {
		t.Fatalf("published %d events, want 1", len(rec.events))
	}
	typing, ok := rec.events[0].Data.(models.TypingIndicator)
	if !ok || !typing.IsTyping || typing.UserID != member.Hex() || typing.Username != "typer" {
		t.Fatalf("event data = %+v", rec.events[0].Data)
	}

	stored, err := database.DB.Collection(string(constants.ChatColl)).CountDocuments(ctx, bson.M{
		"_id": chatID, "typingUsers." + member.Hex(): bson.M{"$exists": true},
	})
	if err != nil {
		t.Fatalf("check chat: %v", err)
	}
	if stored != 0 {
		t.Fatal("typing status was written to the chat document")
	}
}
