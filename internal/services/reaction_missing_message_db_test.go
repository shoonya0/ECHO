package services

import (
	"errors"
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// A reaction on a message that isn't there must return ErrMessageNotFound, so
// the WebSocket mapper can answer MESSAGE_NOT_FOUND without comparing error
// text (#41).
func TestReactionOnMissingMessageIsErrMessageNotFound(t *testing.T) {
	ctx := useTestDB(t)

	alice, chatID := bson.NewObjectID(), bson.NewObjectID()
	if _, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{
		"_id":          chatID,
		"participants": bson.M{alice.Hex(): bson.M{"status": "accepted"}},
	}); err != nil {
		t.Fatalf("insert chat: %v", err)
	}
	missing := bson.NewObjectID()

	if err := AddMessageReaction(chatID, missing, alice, "👍"); !errors.Is(err, ErrMessageNotFound) {
		t.Errorf("AddMessageReaction on a missing message = %v, want ErrMessageNotFound", err)
	}
	if err := RemoveMessageReaction(chatID, missing, alice, "👍"); !errors.Is(err, ErrMessageNotFound) {
		t.Errorf("RemoveMessageReaction on a missing message = %v, want ErrMessageNotFound", err)
	}
}
