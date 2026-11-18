package services

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
)

// Members made pending before #5 were never asked and had full access; the user
// chose to keep that, so the startup backfill accepts them. They are told apart
// from new invitees by having no chats entry for the group. An invitee whose
// account is gone is left alone (#35).
func TestAcceptLegacyPendingMembers(t *testing.T) {
	ctx := useTestDB(t)
	owner, fresh, legacy, gone := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	chatID := insertInviteGroup(t, ctx, owner, fresh) // fresh has a chats entry
	insertTestUsers(t, ctx, legacy)                   // legacy has none
	for _, id := range []bson.ObjectID{legacy, gone} {
		if _, err := database.DB.Collection(string(constants.ChatColl)).UpdateOne(ctx, bson.M{"_id": chatID},
			bson.M{"$set": bson.M{"participants." + id.Hex(): bson.M{"role": "member", "requestStatus": "pending"}}}); err != nil {
			t.Fatalf("add pending member: %v", err)
		}
	}

	n, err := AcceptLegacyPendingMembers(ctx)
	if err != nil {
		t.Fatalf("AcceptLegacyPendingMembers: %v", err)
	}
	if n != 1 {
		t.Fatalf("accepted %d, want 1", n)
	}
	for id, want := range map[bson.ObjectID]string{legacy: "accepted", fresh: "pending", gone: "pending"} {
		if got, _ := participantStatus(t, ctx, chatID, id); got != want {
			t.Errorf("member %s: status %q, want %q", id.Hex(), got, want)
		}
	}
	if !hasChatEntry(t, ctx, legacy, chatID) {
		t.Fatalf("accepted legacy member has no chats entry")
	}

	if n, err := AcceptLegacyPendingMembers(ctx); err != nil || n != 0 {
		t.Fatalf("second run: accepted %d, err %v; want 0, nil", n, err)
	}
}
