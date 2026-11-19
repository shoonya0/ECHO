package services

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// storedInviteCodes reads a group's stored invite codes.
func storedInviteCodes(t *testing.T, chatID bson.ObjectID) []models.InviteCodeEmbed {
	t.Helper()
	var chat models.Chat
	if err := database.DB.Collection(string(constants.ChatColl)).FindOne(t.Context(), bson.M{"_id": chatID}).Decode(&chat); err != nil {
		t.Fatalf("read chat: %v", err)
	}
	return chat.InviteCode
}

// #42: a group's inviteCode list only grew. Creating a code now removes the expired
// ones and refuses one past MaxUnexpiredInviteCodes unexpired ones, deleted ones
// included, also when many are created at once. A deleted code stays listed until it
// expires, and a code from before #15 (no stored date) is kept.
func TestCreateInviteKeepsTheListBounded(t *testing.T) {
	ctx := useTestDB(t)
	owner, invitee := bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, owner, invitee)
	chat := newTestGroup(t, ctx, owner) // starts with one unexpired code

	now := time.Now()
	old := []any{
		bson.M{"inviteCode": "deleted", "deleted": true, "expiredDate": now.Add(time.Hour)},
		bson.M{"inviteCode": "expired", "deleted": false, "expiredDate": now.Add(-time.Hour)},
		bson.M{"inviteCode": "legacy", "deleted": false}, // before #15: no stored date
	}
	if _, err := database.DB.Collection(string(constants.ChatColl)).UpdateOne(ctx,
		bson.M{"_id": chat.ChatID}, bson.M{"$push": bson.M{"inviteCode": bson.M{"$each": old}}}); err != nil {
		t.Fatalf("add old codes: %v", err)
	}
	// Invitations sent with two of them. The expired one's can never be used, and
	// a join or decline can't clean it up once its code is gone.
	if _, err := database.DB.Collection(string(constants.UserColl)).UpdateOne(ctx, bson.M{"_id": invitee},
		bson.M{"$set": bson.M{"chatInvitations": bson.A{
			bson.M{"token": "expired", "status": "pending"},
			bson.M{"token": "deleted", "status": "pending"},
		}}}); err != nil {
		t.Fatalf("add invitations: %v", err)
	}

	// Ask for more than fit, all at once. The first code and "deleted" count.
	const tries = MaxUnexpiredInviteCodes + 5
	errs := make([]error, tries)
	var wg sync.WaitGroup
	for i := range tries {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = CreateInvite(ctx, owner, chat.ChatID)
		}()
	}
	wg.Wait()

	created, refused := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			created++
		case errors.Is(err, ErrTooManyInvites):
			refused++
		default:
			t.Fatalf("CreateInvite: %v", err)
		}
	}
	if want := MaxUnexpiredInviteCodes - 2; created != want || refused != tries-want {
		t.Fatalf("created %d and refused %d, want %d and %d", created, refused, want, tries-want)
	}

	codes := storedInviteCodes(t, chat.ChatID)
	names := map[string]bool{}
	for _, c := range codes {
		names[c.InviteCode] = true
	}
	if names["expired"] || !names["deleted"] || !names["legacy"] {
		t.Fatalf("codes %v: want expired removed, deleted and legacy kept", names)
	}
	if len(codes) != MaxUnexpiredInviteCodes+1 { // the unexpired ones and legacy
		t.Fatalf("stored %d codes, want %d", len(codes), MaxUnexpiredInviteCodes+1)
	}
	if got := loadTestUser(t, ctx, invitee).ChatInvitations; len(got) != 1 || got[0].Token != "deleted" {
		t.Fatalf("invitations = %+v, want only the unexpired code's", got)
	}

	// Deleting one doesn't make room: it counts until it expires.
	if err := DeleteInvite(ctx, owner, chat.InviteCode[0].InviteCode); err != nil {
		t.Fatalf("DeleteInvite: %v", err)
	}
	if _, err := CreateInvite(ctx, owner, chat.ChatID); !errors.Is(err, ErrTooManyInvites) {
		t.Fatalf("CreateInvite after a delete: err = %v, want ErrTooManyInvites", err)
	}

	// Once one expires, there is room again, and the expired one is removed.
	if _, err := database.DB.Collection(string(constants.ChatColl)).UpdateOne(ctx,
		bson.M{"_id": chat.ChatID, "inviteCode.inviteCode": "deleted"},
		bson.M{"$set": bson.M{"inviteCode.$.expiredDate": now.Add(-time.Minute)}}); err != nil {
		t.Fatalf("expire a code: %v", err)
	}
	if _, err := CreateInvite(ctx, owner, chat.ChatID); err != nil {
		t.Fatalf("CreateInvite after one expired: %v", err)
	}
	if got := len(storedInviteCodes(t, chat.ChatID)); got != MaxUnexpiredInviteCodes+1 {
		t.Fatalf("stored %d codes, want %d", got, MaxUnexpiredInviteCodes+1)
	}
}
