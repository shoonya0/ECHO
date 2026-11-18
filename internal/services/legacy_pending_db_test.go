package services

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
)

func addPendingMember(t *testing.T, chatID, userID bson.ObjectID, fields bson.M) {
	t.Helper()
	entry := bson.M{"role": "member", "requestStatus": "pending"}
	for k, v := range fields {
		entry[k] = v
	}
	if _, err := database.DB.Collection(string(constants.ChatColl)).UpdateOne(t.Context(), bson.M{"_id": chatID},
		bson.M{"$set": bson.M{"participants." + userID.Hex(): entry}}); err != nil {
		t.Fatalf("add pending member: %v", err)
	}
}

// Members made pending before #5 were never asked and had full access; the user
// chose to keep that, so the backfill accepts them. They are told apart from
// invitees since #5 by when they became pending, against a cutoff stored on the
// first run, not by their chats entry: a new invitee's entry can be missing (a
// failed or not yet finished write), and they must never be accepted for them
// (code review of #5). Such an invitee gets the entry instead, so the invite shows.
func TestAcceptLegacyPendingMembers(t *testing.T) {
	ctx := useTestDB(t)
	cutoff := time.Now()
	before, after := cutoff.Add(-time.Hour), cutoff.Add(time.Minute)

	owner := bson.NewObjectID()
	legacy, legacyWithEntry, legacyNoDate, fresh, freshNoEntry, gone :=
		bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	chatID := insertInviteGroup(t, ctx, owner, legacyWithEntry, fresh) // these two have entries
	insertTestUsers(t, ctx, legacy, legacyNoDate, freshNoEntry)
	addPendingMember(t, chatID, legacy, bson.M{"createdAt": before})
	addPendingMember(t, chatID, legacyWithEntry, bson.M{"createdAt": before})
	addPendingMember(t, chatID, legacyNoDate, nil)
	addPendingMember(t, chatID, fresh, bson.M{"createdAt": after, "requestedBy": owner})
	addPendingMember(t, chatID, freshNoEntry, bson.M{"createdAt": after})
	addPendingMember(t, chatID, gone, bson.M{"createdAt": before})

	n, err := AcceptLegacyPendingMembers(ctx, cutoff)
	if err != nil {
		t.Fatalf("AcceptLegacyPendingMembers: %v", err)
	}
	if n != 3 {
		t.Fatalf("accepted %d, want 3", n)
	}
	for id, want := range map[bson.ObjectID]string{
		legacy: "accepted", legacyWithEntry: "accepted", legacyNoDate: "accepted",
		fresh: "pending", freshNoEntry: "pending", gone: "pending",
	} {
		if got, _ := participantStatus(t, ctx, chatID, id); got != want {
			t.Errorf("member %s: status %q, want %q", id.Hex(), got, want)
		}
	}
	for _, id := range []bson.ObjectID{legacy, legacyNoDate, freshNoEntry} {
		if !hasChatEntry(t, ctx, id, chatID) {
			t.Errorf("member %s has no chats entry", id.Hex())
		}
	}

	// A later start keeps the first cutoff: an invite made between the two starts
	// stays pending.
	between := bson.NewObjectID()
	insertTestUsers(t, ctx, between)
	addPendingMember(t, chatID, between, bson.M{"createdAt": cutoff.Add(2 * time.Minute)})
	if n, err := AcceptLegacyPendingMembers(ctx, cutoff.Add(time.Hour)); err != nil || n != 0 {
		t.Fatalf("second run: accepted %d, err %v; want 0, nil", n, err)
	}
	if got, _ := participantStatus(t, ctx, chatID, between); got != "pending" {
		t.Fatalf("invite made after the first cutoff: status %q, want pending", got)
	}
}
