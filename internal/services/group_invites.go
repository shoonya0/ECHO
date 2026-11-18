package services

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/logger"
)

// giveInviteesChatEntry adds the group chatID to each invitee's chats list, which is
// where their pending invite is found (#5). $max keeps an entry that is already
// there. The invitees are already pending members, so a failure is only logged:
// accepting by group ID still works, and the hub skips pending chats either way.
func giveInviteesChatEntry(ctx context.Context, chatID bson.ObjectID, invitees []bson.ObjectID) {
	if len(invitees) == 0 {
		return
	}
	if _, err := database.DB.Collection(string(constants.UserColl)).UpdateMany(ctx,
		bson.M{"_id": bson.M{"$in": invitees}},
		bson.M{"$max": bson.M{"chats." + chatID.Hex(): 0}},
	); err != nil {
		logger.WithContext(ctx).WithError(err).WithField("chat", chatID.Hex()).
			Warn("Invitees may not see the group among their pending invites")
	}
}
