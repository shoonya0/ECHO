package services

import (
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// #32: showLastSeen was stored off for every account while nothing read it.
// The startup migration turns it on once; after that, a user's choice stays.
func TestEnableShowLastSeen(t *testing.T) {
	ctx := useTestDB(t)
	users := database.DB.Collection(string(constants.UserColl))
	off, missing, noSettings, on, nullSettings := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	if _, err := users.InsertMany(ctx, []any{
		bson.M{"_id": off, "settings": bson.M{"privacy": bson.M{"showOnlineStatus": "contacts", "showLastSeen": false}}},
		bson.M{"_id": missing, "settings": bson.M{"privacy": bson.M{"showOnlineStatus": "everyone"}}},
		bson.M{"_id": noSettings, "username": "legacy"},
		bson.M{"_id": on, "settings": bson.M{"privacy": bson.M{"showLastSeen": true}}},
		bson.M{"_id": nullSettings, "settings": nil},
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	n, err := EnableShowLastSeen(ctx)
	if err != nil || n != 3 {
		t.Fatalf("first run: %d changed, %v; want 3", n, err)
	}
	read := func(id bson.ObjectID) (showLastSeen bool, scope string) {
		var doc struct {
			Settings struct {
				Privacy struct {
					ShowLastSeen     bool   `bson:"showLastSeen"`
					ShowOnlineStatus string `bson:"showOnlineStatus"`
				} `bson:"privacy"`
			} `bson:"settings"`
		}
		if err := users.FindOne(ctx, bson.M{"_id": id}).Decode(&doc); err != nil {
			t.Fatalf("read %s: %v", id.Hex(), err)
		}
		return doc.Settings.Privacy.ShowLastSeen, doc.Settings.Privacy.ShowOnlineStatus
	}
	for _, id := range []bson.ObjectID{off, missing, noSettings, on} {
		if shown, _ := read(id); !shown {
			t.Errorf("%s: showLastSeen still off", id.Hex())
		}
	}
	if _, scope := read(off); scope != "contacts" {
		t.Errorf("other privacy settings changed: showOnlineStatus = %q", scope)
	}

	// A user turns it off after the migration: a later start leaves it off.
	if _, err := users.UpdateOne(ctx, bson.M{"_id": off}, bson.M{"$set": bson.M{"settings.privacy.showLastSeen": false}}); err != nil {
		t.Fatalf("turn off: %v", err)
	}
	if n, err := EnableShowLastSeen(ctx); err != nil || n != 0 {
		t.Fatalf("second run: %d changed, %v; want 0", n, err)
	}
	if shown, _ := read(off); shown {
		t.Fatal("second run turned a user's choice back on")
	}
}
