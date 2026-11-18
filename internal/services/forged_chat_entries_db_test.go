package services

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
)

// #69: before #9 was fixed, a forged invite code wrote chats.<groupID> on the user
// without making them a participant. The startup repair removes every entry whose
// chat doesn't list the user (or is gone), keeps real ones (accepted and pending
// members, unread counts included), and runs only once.
func TestRemoveForgedChatEntries(t *testing.T) {
	ctx := useTestDB(t)
	users := database.DB.Collection(string(constants.UserColl))

	owner, invitee, forger := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	group := insertInviteGroup(t, ctx, owner, invitee) // owner accepted, invitee pending
	forged := insertInviteGroup(t, ctx, owner)         // forger isn't a participant
	missing := bson.NewObjectID()                      // a chat that no longer exists

	for id, chats := range map[bson.ObjectID]bson.M{
		owner:   {group.Hex(): 4, forged.Hex(): 0},
		invitee: {group.Hex(): 0},
		forger:  {group.Hex(): 0, forged.Hex(): 2, missing.Hex(): 0},
	} {
		if _, err := users.UpdateOne(ctx, bson.M{"_id": id},
			bson.M{"$set": bson.M{"username": id.Hex(), "chats": chats}},
			options.UpdateOne().SetUpsert(true),
		); err != nil {
			t.Fatalf("upsert user: %v", err)
		}
	}

	n, err := RemoveForgedChatEntries(ctx)
	if err != nil {
		t.Fatalf("RemoveForgedChatEntries: %v", err)
	}
	if n != 3 {
		t.Fatalf("removed %d entries, want 3", n)
	}
	for _, c := range []struct {
		user, chat bson.ObjectID
		want       bool
	}{
		{owner, group, true}, {owner, forged, true}, {invitee, group, true},
		{forger, group, false}, {forger, forged, false}, {forger, missing, false},
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

	// It runs once: an entry written afterwards is left alone.
	later := bson.NewObjectID()
	if _, err := users.UpdateOne(ctx, bson.M{"_id": forger}, bson.M{"$set": bson.M{"chats." + later.Hex(): 0}}); err != nil {
		t.Fatalf("add entry: %v", err)
	}
	if n, err := RemoveForgedChatEntries(ctx); err != nil || n != 0 {
		t.Fatalf("second run: removed %d, err %v; want 0, nil", n, err)
	}
	if !hasChatEntry(t, ctx, forger, later) {
		t.Fatal("second run removed an entry")
	}
}
