package controller

import (
	"context"
	"testing"

	"github.com/shoonya0/ECHO/internal/logger"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/testutil"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// A chat is already saved when its reply is built, so a failed name read keeps
// the stored copies instead of failing the request: a 500 would make the client
// retry and create the group again (code review of #36).
func TestFillParticipantNamesKeepsStoredCopiesOnError(t *testing.T) {
	ctx := testutil.UseTestDB(t)
	id := bson.NewObjectID()
	stored := models.ContactUserInfo{UserID: id, Username: "stored", DisplayName: "Stored"}
	chat := &models.Chat{Participants: map[bson.ObjectID]models.ParticipantEmbed{id: {UserInfo: stored}}}

	failing, cancel := context.WithCancel(ctx)
	cancel() // every read fails
	fillParticipantNames(failing, *logger.WithContext(ctx), chat)

	if got := chat.Participants[id].UserInfo; got != stored {
		t.Fatalf("userInfo %+v, want the stored copy %+v", got, stored)
	}
}
