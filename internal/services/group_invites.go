package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/logger"
	"github.com/shoonya0/ECHO/internal/models"
)

// PendingGroupInvite is one group a user is invited to and hasn't answered (#5).
type PendingGroupInvite struct {
	ChatID           bson.ObjectID `json:"chatId"`
	Name             string        `json:"name"`
	Description      string        `json:"description"`
	ParticipantCount int           `json:"participantCount"`
	InvitedBy        bson.ObjectID `json:"invitedBy"`
	InvitedAt        time.Time     `json:"invitedAt"`
}

// giveChatEntry adds the group chatID to each user's chats list, which is where
// an invitee's pending invite is found (#5) and the hub finds a member's chats.
// $max keeps an entry that is already there. It runs inside the group write's
// transaction (#37), so its error aborts the whole write, and it returns how many
// of userIDs exist.
func giveChatEntry(ctx context.Context, chatID bson.ObjectID, userIDs []bson.ObjectID) (int64, error) {
	if len(userIDs) == 0 {
		return 0, nil
	}
	res, err := database.DB.Collection(string(constants.UserColl)).UpdateMany(ctx,
		bson.M{"_id": bson.M{"$in": userIDs}},
		bson.M{"$max": bson.M{"chats." + chatID.Hex(): 0}},
	)
	if err != nil {
		return 0, fmt.Errorf("failed to add the group to its members' chats: %w", err)
	}
	return res.MatchedCount, nil
}

// acceptedFields is the $set that makes userID an accepted member, used by every
// path that accepts a pending member (#5, #103).
func acceptedFields(userID bson.ObjectID, now time.Time) bson.M {
	key := participantField(userID)
	return bson.M{
		key + ".requestStatus": string(constants.StatusAccepted),
		key + ".joinedAt":      now,
		key + ".updatedAt":     now,
	}
}

// pendingInviteFilter matches the group chatID while userID is a pending member.
func pendingInviteFilter(chatID, userID bson.ObjectID) bson.M {
	return bson.M{
		"_id":      chatID,
		"chatType": string(constants.ChatTypeGroup),
		participantField(userID) + ".requestStatus": string(constants.StatusPending),
	}
}

// notPendingError explains an accept or decline that matched no pending invite:
// ErrAlreadyParticipant if userID is a member of the group who isn't pending, and
// otherwise ErrChatNotFound, whether or not the group exists, so group IDs can't be
// probed (#59).
func notPendingError(ctx context.Context, chatID, userID bson.ObjectID) error {
	n, err := database.DB.Collection(string(constants.ChatColl)).CountDocuments(ctx, bson.M{
		"_id":                    chatID,
		"chatType":               string(constants.ChatTypeGroup),
		participantField(userID): bson.M{"$exists": true},
	})
	if err != nil {
		return fmt.Errorf("failed to check the invite: %w", err)
	}
	if n > 0 {
		return fmt.Errorf("%w", ErrAlreadyParticipant)
	}
	return fmt.Errorf("%w", ErrChatNotFound)
}

// AcceptGroupInvite makes userID, a pending member of the group chatID, an accepted
// one (#5). Calling it again as an accepted member returns ErrAlreadyParticipant
// after making sure the join is complete. The member already counts toward the size limit and participantCount,
// so neither changes. Afterwards the group is on their chats list and their open
// connections follow it (#17).
func AcceptGroupInvite(ctx context.Context, userID, chatID bson.ObjectID) error {
	res, err := database.DB.Collection(string(constants.ChatColl)).UpdateOne(ctx,
		pendingInviteFilter(chatID, userID),
		bson.M{"$set": acceptedFields(userID, time.Now())},
	)
	if err != nil {
		return fmt.Errorf("failed to accept the invite: %w", err)
	}
	if res.MatchedCount == 0 {
		err := notPendingError(ctx, chatID, userID)
		if errors.Is(err, ErrAlreadyParticipant) {
			// A retry of an accept that stopped after the status write (the client
			// got a 500) finishes it; the answer stays 409.
			if ferr := finishGroupJoin(ctx, chatID, userID); ferr != nil {
				return ferr
			}
		}
		return err
	}
	return finishGroupJoin(ctx, chatID, userID)
}

// DeclineGroupInvite takes userID, a pending member of the group chatID, out of the
// group (#5). They can be added again later. Their chats entry and invitations to the
// group go too; if that write fails it is only logged, since the hub and the pending
// list ignore an entry for a group the user isn't in.
func DeclineGroupInvite(ctx context.Context, userID, chatID bson.ObjectID) error {
	res, err := database.DB.Collection(string(constants.ChatColl)).UpdateOne(ctx,
		pendingInviteFilter(chatID, userID),
		bson.M{
			"$unset": bson.M{participantField(userID): ""},
			// The filter requires userID to be present, so this removes one (#16).
			"$inc": bson.M{"stats.participantCount": -1},
			"$set": bson.M{"updatedAt": time.Now()},
		},
	)
	if err != nil {
		return fmt.Errorf("failed to decline the invite: %w", err)
	}
	if res.MatchedCount == 0 {
		return notPendingError(ctx, chatID, userID)
	}

	log := logger.WithContext(ctx).WithField("chat", chatID.Hex())
	// A pending member may follow the chat live with join_chat (#48) until #5's
	// second step enforces pending, so take their connections out of its room.
	if err := RemoveMembersFromLiveChat(chatID.Hex(), []string{userID.Hex()}); err != nil {
		log.WithError(err).Debug("Could not take a declined invitee out of the live room")
	}
	codes, err := groupInviteCodes(ctx, chatID)
	if err != nil {
		log.WithError(err).Warn("Declined invitee keeps their invitations to the group")
		codes = nil
	}
	update := bson.M{"$unset": bson.M{"chats." + chatID.Hex(): ""}}
	if len(codes) > 0 {
		update["$pull"] = bson.M{"chatInvitations": bson.M{"token": bson.M{"$in": codes}}}
	}
	userRes, err := database.DB.Collection(string(constants.UserColl)).UpdateOne(ctx, bson.M{"_id": userID}, update)
	switch {
	case err != nil:
		log.WithError(err).Warn("Declined invitee keeps the group on their chats list")
	case userRes.MatchedCount == 0:
		log.Debug("Declined invitee has no user document") // the account is gone (#35)
	}
	return nil
}

// PendingGroupInvites lists the groups userID is a pending member of, newest group
// first (#5). Invites are found through the user's chats list, so the chats are read
// by _id. Members made pending before #5 have no entry until the startup backfill
// accepts them.
func PendingGroupInvites(ctx context.Context, userID bson.ObjectID) ([]PendingGroupInvite, error) {
	user, err := FindByID[models.User](ctx, database.DB.Collection(string(constants.UserColl)),
		bson.M{"_id": userID}, bson.M{"chats": 1})
	if err != nil {
		return nil, fmt.Errorf("failed to read the user's chats: %w", err)
	}
	invites := []PendingGroupInvite{}
	if len(user.Chats) == 0 {
		return invites, nil
	}
	ids := make([]bson.ObjectID, 0, len(user.Chats))
	for id := range user.Chats {
		ids = append(ids, id)
	}

	key := participantField(userID)
	cur, err := database.DB.Collection(string(constants.ChatColl)).Find(ctx,
		bson.M{
			"_id":                  bson.M{"$in": ids},
			"chatType":             string(constants.ChatTypeGroup),
			key + ".requestStatus": string(constants.StatusPending),
		},
		options.Find().
			SetProjection(bson.M{"name": 1, "description": 1, "stats.participantCount": 1, key: 1}).
			SetSort(bson.D{{Key: "_id", Value: -1}}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find pending invites: %w", err)
	}
	var chats []models.Chat
	if err := cur.All(ctx, &chats); err != nil {
		return nil, fmt.Errorf("failed to decode pending invites: %w", err)
	}
	for _, c := range chats {
		p := c.Participants[userID]
		invites = append(invites, PendingGroupInvite{
			ChatID:           c.ChatID,
			Name:             c.Name,
			Description:      c.Description,
			ParticipantCount: c.Stats.ParticipantCount,
			InvitedBy:        p.RequestedBy,
			InvitedAt:        p.CreatedAt,
		})
	}
	return invites, nil
}
