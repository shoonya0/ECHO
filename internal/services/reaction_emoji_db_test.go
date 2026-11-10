package services

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// The emoji becomes part of a MongoDB field path ("reactions.<emoji>.users"), so a
// value that isn't a single safe field name must be refused before any write (#58).
func TestReactionEmojiMustBeOneFieldName(t *testing.T) {
	ctx := useTestDB(t)

	alice, bob := bson.NewObjectID(), bson.NewObjectID()
	chatID, messageID := bson.NewObjectID(), bson.NewObjectID()
	for _, id := range []bson.ObjectID{alice, bob} {
		if _, err := database.DB.Collection(string(constants.UserColl)).InsertOne(ctx, bson.M{"_id": id, "username": "user-" + id.Hex()}); err != nil {
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
		"_id": messageID, "chatId": chatID, "senderId": alice, "content": "hi",
	}); err != nil {
		t.Fatalf("insert message: %v", err)
	}

	// Both members react with 👍, so the message has reactions.👍.count, .users
	// and .details. The issue used "ok", which is now refused for being letters.
	for _, id := range []bson.ObjectID{alice, bob} {
		if err := AddMessageReaction(chatID, messageID, id, "👍"); err != nil {
			t.Fatalf("seed reaction: %v", err)
		}
	}

	message := func(t *testing.T) bson.M {
		t.Helper()
		var doc bson.M
		if err := database.DB.Collection(string(constants.MessageColl)).FindOne(ctx, bson.M{"_id": messageID}).Decode(&doc); err != nil {
			t.Fatalf("find message: %v", err)
		}
		return doc
	}

	bad := []struct {
		name  string
		emoji string
	}{
		{"path into another field", "👍.details"},
		{"path from the issue", "ok.details"},
		{"dot", "a.b"},
		{"leading dollar", "$x"},
		{"dollar inside", "a$b"},
		{"NUL", "a\x00b"},
		{"too long", strings.Repeat("a", 50)},
		{"empty", ""},
		{"not UTF-8", "\xff\xfe"},
	}
	calls := []struct {
		name string
		call func(emoji string) error
	}{
		{"add", func(emoji string) error { return AddMessageReaction(chatID, messageID, bob, emoji) }},
		{"remove", func(emoji string) error { return RemoveMessageReaction(chatID, messageID, bob, emoji) }},
	}
	for _, c := range calls {
		for _, tc := range bad {
			t.Run(c.name+": "+tc.name, func(t *testing.T) {
				before := message(t)
				if err := c.call(tc.emoji); !errors.Is(err, ErrInvalidEmoji) {
					t.Fatalf("err = %v, want ErrInvalidEmoji", err)
				}
				if after := message(t); !reflect.DeepEqual(before, after) {
					t.Fatalf("message changed:\nbefore %v\nafter  %v", before, after)
				}
			})
		}
	}

	reactionKeys := func(t *testing.T) map[string]bson.Raw {
		t.Helper()
		var doc struct {
			Reactions map[string]bson.Raw `bson:"reactions"`
		}
		if err := database.DB.Collection(string(constants.MessageColl)).FindOne(ctx, bson.M{"_id": messageID}).Decode(&doc); err != nil {
			t.Fatalf("find message: %v", err)
		}
		return doc.Reactions
	}

	good := []string{"😂", "👍🏽", "❤️", "\U0001F937\u200d\u2640\ufe0f" /* 🤷‍♀️, 13 bytes */}
	for _, emoji := range good {
		t.Run("real emoji works: "+emoji, func(t *testing.T) {
			if err := AddMessageReaction(chatID, messageID, alice, emoji); err != nil {
				t.Fatalf("add: %v", err)
			}
			if _, ok := reactionKeys(t)[emoji]; !ok {
				t.Fatalf("reaction %q not stored: %v", emoji, reactionKeys(t))
			}
			if err := RemoveMessageReaction(chatID, messageID, alice, emoji); err != nil {
				t.Fatalf("remove: %v", err)
			}
			if _, ok := reactionKeys(t)[emoji]; ok {
				t.Fatalf("reaction %q still stored after its only user removed it", emoji)
			}
		})
	}
}
