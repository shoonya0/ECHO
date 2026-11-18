package services

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
)

// #69: before #9 was fixed, a forged invite code wrote chats.<groupID> on the user
// without making them a participant; a failed cleanup write still can. The startup
// repair removes every entry whose chat doesn't list the user (or is gone), keeps real
// ones (accepted and pending members, unread counts included), and runs on every start.
// Batches of 2 make the four users span several batches.
func TestRemoveStaleChatEntries(t *testing.T) {
	ctx := useTestDB(t)
	users := database.DB.Collection(string(constants.UserColl))
	old := staleEntriesBatch
	staleEntriesBatch = 2
	t.Cleanup(func() { staleEntriesBatch = old })

	owner, invitee, forger, other := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	group := insertInviteGroup(t, ctx, owner, invitee) // owner accepted, invitee pending
	forged := insertInviteGroup(t, ctx, owner)         // forger and other aren't participants
	missing := bson.NewObjectID()                      // a chat that no longer exists
	if _, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{
		"_id": missing, "chatType": "group", "participants": bson.A{forger.Hex()}, // malformed
	}); err != nil {
		t.Fatalf("insert malformed chat: %v", err)
	}
	gone := bson.NewObjectID()

	for id, chats := range map[bson.ObjectID]bson.M{
		owner:   {group.Hex(): 4, forged.Hex(): 0},
		invitee: {group.Hex(): 0},
		forger:  {group.Hex(): 0, forged.Hex(): 2, missing.Hex(): 0, gone.Hex(): 1, "not-an-id": 0},
		other:   {forged.Hex(): 0},
	} {
		if _, err := users.UpdateOne(ctx, bson.M{"_id": id},
			bson.M{"$set": bson.M{"username": id.Hex(), "chats": chats}},
			options.UpdateOne().SetUpsert(true),
		); err != nil {
			t.Fatalf("upsert user: %v", err)
		}
	}

	n, err := RemoveStaleChatEntries(ctx)
	if err != nil {
		t.Fatalf("RemoveStaleChatEntries: %v", err)
	}
	if n != 5 {
		t.Fatalf("removed %d entries, want 5", n)
	}
	for _, c := range []struct {
		user, chat bson.ObjectID
		want       bool
	}{
		{owner, group, true}, {owner, forged, true}, {invitee, group, true},
		{forger, group, false}, {forger, forged, false}, {forger, missing, false}, {forger, gone, false},
		{other, forged, false},
	} {
		if got := hasChatEntry(t, ctx, c.user, c.chat); got != c.want {
			t.Errorf("user %s, chat %s: entry %v, want %v", c.user.Hex(), c.chat.Hex(), got, c.want)
		}
	}
	var o struct {
		Chats map[string]int `bson:"chats"`
	}
	if err := users.FindOne(ctx, bson.M{"_id": owner}).Decode(&o); err != nil {
		t.Fatalf("load owner: %v", err)
	}
	if o.Chats[group.Hex()] != 4 {
		t.Fatalf("owner's unread count = %d, want 4 kept", o.Chats[group.Hex()])
	}

	// It runs on every start: an entry a failed cleanup leaves later is removed by
	// the next run, and a run with nothing stale removes nothing.
	later := bson.NewObjectID()
	if _, err := users.UpdateOne(ctx, bson.M{"_id": invitee}, bson.M{"$set": bson.M{"chats." + later.Hex(): 0}}); err != nil {
		t.Fatalf("add entry: %v", err)
	}
	if n, err := RemoveStaleChatEntries(ctx); err != nil || n != 1 {
		t.Fatalf("second run: removed %d, err %v; want 1, nil", n, err)
	}
	if hasChatEntry(t, ctx, invitee, later) || !hasChatEntry(t, ctx, invitee, group) {
		t.Fatal("second run: want the stale entry removed and the pending invite kept")
	}
	if n, err := RemoveStaleChatEntries(ctx); err != nil || n != 0 {
		t.Fatalf("third run: removed %d, err %v; want 0, nil", n, err)
	}
}
