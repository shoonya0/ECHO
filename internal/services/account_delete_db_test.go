package services

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"
)

type deleteFixture struct {
	a, b, c, d, e          bson.ObjectID // a is deleted
	g1, g2, g3, g4, direct bson.ObjectID
	messageByA, messageByB bson.ObjectID
}

func member(role, status string, joined time.Time) bson.M {
	return bson.M{"role": role, "requestStatus": status, "joinedAt": joined,
		"userInfo": bson.M{"displayName": "someone", "username": "someone", "avatar": "pic.png"}}
}

func insertDeleteFixture(t *testing.T, ctx context.Context) deleteFixture {
	t.Helper()
	f := deleteFixture{
		a: bson.NewObjectID(), b: bson.NewObjectID(), c: bson.NewObjectID(), d: bson.NewObjectID(), e: bson.NewObjectID(),
		g1: bson.NewObjectID(), g2: bson.NewObjectID(), g3: bson.NewObjectID(), g4: bson.NewObjectID(), direct: bson.NewObjectID(),
		messageByA: bson.NewObjectID(), messageByB: bson.NewObjectID(),
	}
	early, late := time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour)
	users := database.DB.Collection(string(constants.UserColl))
	if _, err := users.InsertMany(ctx, []any{
		bson.M{"_id": f.a, "username": "alice", "contactInfo": bson.M{"contacts": bson.A{f.b}}},
		bson.M{"_id": f.b, "username": "bob", "contactInfo": bson.M{
			"contacts": bson.A{f.a, f.c}, "favorites": bson.A{f.a}, "blocked": bson.A{f.a}, "blockedBy": bson.A{f.a},
			"pendingIn": bson.A{f.a}, "pendingOut": bson.A{f.a},
		}},
		bson.M{"_id": f.c, "username": "carol"}, bson.M{"_id": f.d, "username": "dave"}, bson.M{"_id": f.e, "username": "erin"},
	}); err != nil {
		t.Fatalf("insert users: %v", err)
	}
	group := func(id bson.ObjectID, participants bson.M) any {
		return bson.M{"_id": id, "chatType": string(constants.ChatTypeGroup), "ownerId": f.a,
			"participants": participants, "stats": bson.M{"participantCount": len(participants)}}
	}
	chats := database.DB.Collection(string(constants.ChatColl))
	if _, err := chats.InsertMany(ctx, []any{
		group(f.g1, bson.M{
			f.a.Hex(): member("owner", "accepted", early),
			f.c.Hex(): member("admin", "accepted", late),
			f.d.Hex(): member("member", "accepted", early),
		}),
		group(f.g2, bson.M{
			f.a.Hex(): member("owner", "accepted", early),
			f.d.Hex(): member("member", "accepted", late),
			f.e.Hex(): member("member", "pending", early),
		}),
		group(f.g3, bson.M{
			f.a.Hex(): member("owner", "accepted", early),
			f.e.Hex(): member("member", "pending", late),
		}),
		bson.M{"_id": f.g4, "chatType": string(constants.ChatTypeGroup), "ownerId": f.c,
			"participants": bson.M{f.c.Hex(): member("owner", "accepted", early), f.a.Hex(): member("member", "pending", late)},
			"stats":        bson.M{"participantCount": 2}},
		bson.M{"_id": f.direct, "chatType": string(constants.ChatTypeDirect),
			"participants": bson.M{f.a.Hex(): member("member", "", early), f.b.Hex(): member("member", "", early)}},
	}); err != nil {
		t.Fatalf("insert chats: %v", err)
	}
	sender := func(id bson.ObjectID, name string) bson.M {
		return bson.M{"userId": id, "username": name, "displayName": name, "avatar": name + ".png"}
	}
	if _, err := database.DB.Collection(string(constants.MessageColl)).InsertMany(ctx, []any{
		bson.M{"_id": f.messageByA, "chatId": f.direct, "senderId": f.a, "sender": sender(f.a, "alice"), "content": "hi"},
		bson.M{"_id": f.messageByB, "chatId": f.direct, "senderId": f.b, "sender": sender(f.b, "bob"), "content": "hey"},
	}); err != nil {
		t.Fatalf("insert messages: %v", err)
	}
	return f
}

func loadMessage(t *testing.T, ctx context.Context, id bson.ObjectID) models.Message {
	t.Helper()
	var m models.Message
	if err := database.DB.Collection(string(constants.MessageColl)).FindOne(ctx, bson.M{"_id": id}).Decode(&m); err != nil {
		t.Fatalf("load message: %v", err)
	}
	return m
}

func checkDeleted(t *testing.T, ctx context.Context, f deleteFixture) {
	t.Helper()
	if n, err := database.DB.Collection(string(constants.UserColl)).CountDocuments(ctx, bson.M{"_id": f.a}); err != nil || n != 0 {
		t.Fatalf("user document: count %d, err %v; want gone", n, err)
	}

	var bob models.User
	if err := database.DB.Collection(string(constants.UserColl)).FindOne(ctx, bson.M{"_id": f.b}).Decode(&bob); err != nil {
		t.Fatalf("load bob: %v", err)
	}
	ci := bob.ContactInfo
	for name, list := range map[string][]bson.ObjectID{
		"contacts": ci.Contacts, "favorites": ci.Favorites, "blocked": ci.Blocked, "blockedBy": ci.BlockedBy,
		"pendingIn": ci.PendingIn, "pendingOut": ci.PendingOut,
	} {
		if slices.Contains(list, f.a) {
			t.Errorf("bob's %s still holds the deleted user", name)
		}
	}
	if !slices.Contains(ci.Contacts, f.c) {
		t.Errorf("bob's other contact was removed")
	}

	owner := func(chatID bson.ObjectID) (bson.ObjectID, models.Chat) {
		chat := loadChat(t, ctx, chatID)
		if _, ok := chat.Participants[f.a]; ok {
			t.Errorf("group %s still lists the deleted user", chatID.Hex())
		}
		if chat.Stats.ParticipantCount != len(chat.Participants) {
			t.Errorf("group %s: participantCount %d, want %d", chatID.Hex(), chat.Stats.ParticipantCount, len(chat.Participants))
		}
		return chat.OwnerID, chat
	}
	// The earliest-joined admin, even when a plain member joined before them.
	if id, chat := owner(f.g1); id != f.c || chat.Participants[f.c].Role != "owner" || chat.Participants[f.d].Role != "member" {
		t.Errorf("g1: owner %s, roles %+v; want carol as owner", id.Hex(), chat.Participants)
	}
	// No admin: the earliest-joined accepted member, not a pending one.
	if id, chat := owner(f.g2); id != f.d || chat.Participants[f.d].Role != "owner" || chat.Participants[f.e].Role != "member" {
		t.Errorf("g2: owner %s; want dave", id.Hex())
	}
	// Only a pending invitee left: no owner.
	if id, chat := owner(f.g3); !id.IsZero() || chat.Participants[f.e].Role != "member" {
		t.Errorf("g3: owner %s, erin's role %q; want no owner", id.Hex(), chat.Participants[f.e].Role)
	}
	if id, _ := owner(f.g4); id != f.c {
		t.Errorf("g4: owner changed to %s", id.Hex())
	}

	direct := loadChat(t, ctx, f.direct)
	p, ok := direct.Participants[f.a]
	if !ok {
		t.Fatalf("direct chat lost the deleted user's entry; the other side's history needs it")
	}
	if p.UserInfo.DisplayName != DeletedUserName || p.UserInfo.Username != "" || p.UserInfo.Avatar != "" {
		t.Errorf("direct chat userInfo = %+v, want anonymized", p.UserInfo)
	}
	if direct.Participants[f.b].UserInfo.DisplayName != "someone" {
		t.Errorf("the other participant's info changed")
	}

	if s := loadMessage(t, ctx, f.messageByA).Sender; s.DisplayName != DeletedUserName || s.Username != "" || s.Avatar != "" {
		t.Errorf("deleted user's message sender = %+v, want anonymized", s)
	}
	if s := loadMessage(t, ctx, f.messageByB).Sender; s.Username != "bob" {
		t.Errorf("other sender changed: %+v", s)
	}
}

// Deleting an account removes or anonymizes everything that points at it (#35):
// contact lists, group memberships (handing ownership on), and the name and
// avatar copied into direct chats and messages. The history stays.
func TestDeleteProfileScrubsReferences(t *testing.T) {
	ctx := useTestDB(t)
	f := insertDeleteFixture(t, ctx)

	if err := DeleteProfile(ctx, f.a.Hex()); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}
	checkDeleted(t, ctx, f)

	if err := DeleteProfile(ctx, f.a.Hex()); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("second delete: err %v, want ErrUserNotFound", err)
	}
}

// The account is deleted last, so a delete that stopped partway can be run
// again and ends in the same state (#35).
func TestDeleteProfileFinishesAPartialRun(t *testing.T) {
	ctx := useTestDB(t)
	f := insertDeleteFixture(t, ctx)

	// A first run that only got through the contact lists and one group.
	if err := scrubContacts(ctx, f.a); err != nil {
		t.Fatalf("scrubContacts: %v", err)
	}
	if err := leaveChatsOnDelete(ctx, f.a, []bson.ObjectID{f.g1}); err != nil {
		t.Fatalf("leave g1: %v", err)
	}

	if err := DeleteProfile(ctx, f.a.Hex()); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}
	checkDeleted(t, ctx, f)
}

// Review of #35: deleting a co-owner leaves the primary owner (ownerId) alone;
// a channel's owner hands it on like a group's, not kept like a direct chat's;
// and a group left with no owner stops admitting people by its invite codes.
func TestDeleteProfileOwnershipEdgeCases(t *testing.T) {
	ctx := useTestDB(t)
	a, x, y, m, e := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, a, x, y, m, e)
	early, late := time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour)
	coOwned, channel, orphan := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	if _, err := database.DB.Collection(string(constants.ChatColl)).InsertMany(ctx, []any{
		bson.M{"_id": coOwned, "chatType": "group", "ownerId": x, "adminIds": bson.A{a, x}, "stats": bson.M{"participantCount": 3},
			"participants": bson.M{
				a.Hex(): member("owner", "accepted", early), x.Hex(): member("owner", "accepted", late),
				y.Hex(): member("owner", "accepted", early),
			}},
		bson.M{"_id": channel, "chatType": "channel", "ownerId": a, "stats": bson.M{"participantCount": 2},
			"participants": bson.M{a.Hex(): member("owner", "accepted", early), m.Hex(): member("member", "accepted", late)}},
		bson.M{"_id": orphan, "chatType": "group", "ownerId": a, "stats": bson.M{"participantCount": 2},
			"participants": bson.M{a.Hex(): member("owner", "accepted", early), e.Hex(): member("member", "pending", late)},
			"inviteCode":   bson.A{mintInvite(t, time.Now().Add(time.Hour))}},
	}); err != nil {
		t.Fatalf("insert chats: %v", err)
	}

	if err := DeleteProfile(ctx, a.Hex()); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}

	co := loadChat(t, ctx, coOwned)
	if co.OwnerID != x || co.Participants[y].Role != "owner" || co.Participants[x].Role != "owner" {
		t.Errorf("co-owned: ownerId %s; want x kept and both owners untouched", co.OwnerID.Hex())
	}
	if slices.Contains(co.AdminIDs, a) {
		t.Errorf("co-owned: adminIds still holds the deleted user")
	}
	ch := loadChat(t, ctx, channel)
	if _, ok := ch.Participants[a]; ok || ch.OwnerID != m || ch.Participants[m].Role != "owner" {
		t.Errorf("channel: deleted user kept or not handed on (ownerId %s)", ch.OwnerID.Hex())
	}
	or := loadChat(t, ctx, orphan)
	if !or.OwnerID.IsZero() || len(or.InviteCode) != 1 || !or.InviteCode[0].Deleted {
		t.Errorf("ownerless group: ownerId %s, codes %+v; want no owner and every code deleted", or.OwnerID.Hex(), or.InviteCode)
	}
}
