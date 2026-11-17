package services

import (
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// #117: the hub's membership re-check reads the participants of every chat it
// has a room for, in one query.
func TestChatMembers(t *testing.T) {
	ctx := useTestDB(t)

	alice, bob, carol := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	group, direct, unknown := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()

	chats := database.DB.Collection(string(constants.ChatColl))
	if _, err := chats.InsertMany(ctx, []any{
		bson.M{"_id": group, "participants": bson.M{
			alice.Hex(): bson.M{"status": "accepted"},
			bob.Hex():   bson.M{"status": "pending"},
		}},
		bson.M{"_id": direct, "participants": bson.M{carol.Hex(): bson.M{"status": "accepted"}}},
	}); err != nil {
		t.Fatalf("insert chats: %v", err)
	}

	members, err := ChatMembers(ctx, []bson.ObjectID{group, direct, unknown})
	if err != nil {
		t.Fatalf("ChatMembers: %v", err)
	}

	want := map[string][]string{
		group.Hex():  {alice.Hex(), bob.Hex()},
		direct.Hex(): {carol.Hex()},
	}
	if len(members) != len(want) {
		t.Fatalf("got %d chats, want %d (an unknown chat has no entry): %v", len(members), len(want), members)
	}
	for chatID, users := range want {
		if len(members[chatID]) != len(users) {
			t.Errorf("chat %s: got %v, want %v", chatID, members[chatID], users)
		}
		for _, userID := range users {
			if _, ok := members[chatID][userID]; !ok {
				t.Errorf("chat %s: %s is missing", chatID, userID)
			}
		}
	}

	if empty, err := ChatMembers(ctx, nil); err != nil || len(empty) != 0 {
		t.Errorf("no chat IDs: got %v, %v; want an empty result", empty, err)
	}
}
