package services

import (
	"context"
	"testing"
	"time"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// setReadState gives each user a read receipt and a read seq on the chat.
func setReadState(t *testing.T, ctx context.Context, chatID bson.ObjectID, users ...bson.ObjectID) {
	t.Helper()
	set := bson.M{}
	for _, u := range users {
		set["readReceipts."+u.Hex()] = time.Now()
		set[readSeqField(u)] = int64(7)
	}
	if _, err := database.DB.Collection(string(constants.ChatColl)).UpdateOne(ctx,
		bson.M{"_id": chatID}, bson.M{"$set": set}); err != nil {
		t.Fatalf("set read state: %v", err)
	}
}

// readStateKeys returns the user IDs with a read receipt and those with a read seq.
func readStateKeys(t *testing.T, ctx context.Context, chatID bson.ObjectID) (receipts, seqs map[string]bool) {
	t.Helper()
	var chat struct {
		ReadReceipts map[string]time.Time `bson:"readReceipts"`
		ReadSeqs     map[string]int64     `bson:"readSeqs"`
	}
	if err := database.DB.Collection(string(constants.ChatColl)).FindOne(ctx, bson.M{"_id": chatID}).Decode(&chat); err != nil {
		t.Fatalf("read chat: %v", err)
	}
	receipts, seqs = map[string]bool{}, map[string]bool{}
	for k := range chat.ReadReceipts {
		receipts[k] = true
	}
	for k := range chat.ReadSeqs {
		seqs[k] = true
	}
	return receipts, seqs
}

// checkReadState fails unless only keep's read state is left on the chat.
func checkReadState(t *testing.T, ctx context.Context, chatID, keep, gone bson.ObjectID) {
	t.Helper()
	receipts, seqs := readStateKeys(t, ctx, chatID)
	if receipts[gone.Hex()] || seqs[gone.Hex()] {
		t.Fatalf("the user who left still has read state: receipts %v, seqs %v", receipts, seqs)
	}
	if !receipts[keep.Hex()] || !seqs[keep.Hex()] {
		t.Fatalf("a member's read state went too: receipts %v, seqs %v", receipts, seqs)
	}
}

// #42: a member's readReceipts and readSeqs entries stayed on the chat after they
// were removed, declined or deleted their account, so the maps only grew.
func TestLeaversLoseTheirReadState(t *testing.T) {
	ctx := useTestDB(t)

	t.Run("removed", func(t *testing.T) {
		owner, member := bson.NewObjectID(), bson.NewObjectID()
		insertTestUsers(t, ctx, owner, member)
		chat := newTestGroup(t, ctx, owner, member)
		setReadState(t, ctx, chat.ChatID, owner, member)
		if _, err := RemoveGroupMember(ctx, owner, chat.ChatID, []bson.ObjectID{member}); err != nil {
			t.Fatalf("RemoveGroupMember: %v", err)
		}
		checkReadState(t, ctx, chat.ChatID, owner, member)
	})

	t.Run("declined", func(t *testing.T) {
		owner, invitee := bson.NewObjectID(), bson.NewObjectID()
		insertTestUsers(t, ctx, owner, invitee)
		chat := newTestGroup(t, ctx, owner, invitee)
		setReadState(t, ctx, chat.ChatID, owner, invitee)
		if err := DeclineGroupInvite(ctx, invitee, chat.ChatID); err != nil {
			t.Fatalf("DeclineGroupInvite: %v", err)
		}
		checkReadState(t, ctx, chat.ChatID, owner, invitee)
	})

	t.Run("deleted their account", func(t *testing.T) {
		owner, member := bson.NewObjectID(), bson.NewObjectID()
		insertTestUsers(t, ctx, owner, member)
		chat := newTestGroup(t, ctx, owner, member)
		setReadState(t, ctx, chat.ChatID, owner, member)
		if err := leaveChatOnDelete(ctx, chat.ChatID, member); err != nil {
			t.Fatalf("leaveChatOnDelete: %v", err)
		}
		checkReadState(t, ctx, chat.ChatID, owner, member)
	})
}

// A read that passed its membership check just before the reader was removed
// must not write their read state back.
func TestReadStateIsOnlyWrittenForParticipants(t *testing.T) {
	ctx := useTestDB(t)
	_, chatID, _ := seedReadableChat(ctx, t)
	gone := bson.NewObjectID()

	if _, err := updateChatReadReceipts(ctx, chatID, gone, time.Now(), 3); err == nil {
		t.Fatal("updateChatReadReceipts for a non-participant = nil, want an error")
	}
	receipts, seqs := readStateKeys(t, ctx, chatID)
	if receipts[gone.Hex()] || seqs[gone.Hex()] {
		t.Fatalf("read state written for a non-participant: receipts %v, seqs %v", receipts, seqs)
	}
}

// Entries left before the fix are removed once at startup; a chat without readSeqs
// doesn't get an empty one.
func TestRemoveStaleReadState(t *testing.T) {
	ctx := useTestDB(t)
	member, stale := bson.NewObjectID(), bson.NewObjectID()
	withSeqs, withoutSeqs := bson.NewObjectID(), bson.NewObjectID()
	chats := database.DB.Collection(string(constants.ChatColl))
	if _, err := chats.InsertMany(ctx, []any{
		bson.M{"_id": withSeqs, "participants": bson.M{member.Hex(): bson.M{}}},
		bson.M{"_id": withoutSeqs, "participants": bson.M{member.Hex(): bson.M{}},
			"readReceipts": bson.M{member.Hex(): time.Now(), stale.Hex(): time.Now()}},
	}); err != nil {
		t.Fatalf("insert chats: %v", err)
	}
	setReadState(t, ctx, withSeqs, member, stale)

	n, err := RemoveStaleReadState(ctx)
	if err != nil || n != 2 {
		t.Fatalf("RemoveStaleReadState = (%d, %v), want (2, nil)", n, err)
	}
	checkReadState(t, ctx, withSeqs, member, stale)
	receipts, _ := readStateKeys(t, ctx, withoutSeqs)
	if receipts[stale.Hex()] || !receipts[member.Hex()] {
		t.Fatalf("receipts %v: want only the member's", receipts)
	}
	if err := chats.FindOne(ctx, bson.M{"_id": withoutSeqs, "readSeqs": bson.M{"$exists": true}}).Err(); err == nil {
		t.Fatal("a chat without readSeqs got one")
	}

	setReadState(t, ctx, withSeqs, stale) // after the marker: left alone
	if n, err := RemoveStaleReadState(ctx); err != nil || n != 0 {
		t.Fatalf("second RemoveStaleReadState = (%d, %v), want (0, nil)", n, err)
	}
}
