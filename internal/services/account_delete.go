package services

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"
)

// DeletedUserName is shown instead of a deleted account's name, in the messages it
// sent and in the direct chats it was in (#35).
const DeletedUserName = "Deleted user"

// leaveAttempts bounds how often a deleted user's exit from one chat is retried
// when the chat changes between its read and its update.
const leaveAttempts = 3

// scrubContacts takes userID out of every other user's contact, favorite, request
// and block lists (#35).
func scrubContacts(ctx context.Context, userID bson.ObjectID) error {
	pull := relationshipLists(userID)
	pull["contactInfo.blocked"] = userID
	pull["contactInfo.blockedBy"] = userID
	or := make(bson.A, 0, len(pull))
	for field := range pull {
		or = append(or, bson.M{field: userID})
	}
	if _, err := database.DB.Collection(string(constants.UserColl)).UpdateMany(ctx,
		bson.M{"$or": or}, bson.M{"$pull": pull},
	); err != nil {
		return fmt.Errorf("failed to remove the user from contact lists: %w", err)
	}
	return nil
}

// chatsWithParticipant returns the chats that list userID in participants. It scans
// on the dynamic participants.<id> key, which can't be indexed; it runs only when an
// account is deleted, and it also finds chats missing from the user's chats list.
func chatsWithParticipant(ctx context.Context, userID bson.ObjectID) ([]bson.ObjectID, error) {
	cur, err := database.DB.Collection(string(constants.ChatColl)).Find(ctx,
		bson.M{participantField(userID): bson.M{"$exists": true}},
		options.Find().SetProjection(bson.M{"_id": 1}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find the user's chats: %w", err)
	}
	var found []struct {
		ID bson.ObjectID `bson:"_id"`
	}
	if err := cur.All(ctx, &found); err != nil {
		return nil, fmt.Errorf("failed to decode the user's chats: %w", err)
	}
	ids := make([]bson.ObjectID, len(found))
	for i, c := range found {
		ids[i] = c.ID
	}
	return ids, nil
}

// leaveChatsOnDelete takes userID out of each chat as their account is deleted. A
// failure in one chat doesn't stop the others; all failures are returned together.
func leaveChatsOnDelete(ctx context.Context, userID bson.ObjectID, chatIDs []bson.ObjectID) error {
	var errs []error
	for _, chatID := range chatIDs {
		if err := leaveChatOnDelete(ctx, chatID, userID); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// leaveChatOnDelete takes the deleted userID out of one chat (#35). In a direct chat
// the entry stays, anonymized, so the other person keeps the history. In a group the
// user is removed, and an owner hands the group on (successorOnDelete). Each update
// re-checks what it read, and a chat that changed meanwhile is read again.
func leaveChatOnDelete(ctx context.Context, chatID, userID bson.ObjectID) error {
	chats := database.DB.Collection(string(constants.ChatColl))
	key := participantField(userID)
	for range leaveAttempts {
		var chat models.Chat
		err := chats.FindOne(ctx, bson.M{"_id": chatID},
			options.FindOne().SetProjection(bson.M{"chatType": 1, "ownerId": 1, "participants": 1}),
		).Decode(&chat)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil // the chat is gone
		}
		if err != nil {
			return fmt.Errorf("failed to read chat %s: %w", chatID.Hex(), err)
		}
		me, ok := chat.Participants[userID]
		if !ok {
			return nil // already out
		}

		filter := bson.M{"_id": chatID, key: bson.M{"$exists": true}}
		var update bson.M
		if chat.ChatType != string(constants.ChatTypeGroup) {
			update = bson.M{"$set": bson.M{key + ".userInfo": models.ContactUserInfo{
				UserID: userID, DisplayName: DeletedUserName,
			}}}
		} else {
			set := bson.M{"updatedAt": time.Now()}
			unset := bson.M{key: ""}
			if me.Role == string(constants.ChatRoleOwner) || chat.OwnerID == userID {
				next, ok := successorOnDelete(chat.Participants, userID)
				if ok {
					nextKey := participantField(next)
					filter[nextKey+".role"] = chat.Participants[next].Role
					set[nextKey+".role"] = string(constants.ChatRoleOwner)
					set["ownerId"] = next
				} else if chat.OwnerID == userID {
					unset["ownerId"] = ""
				}
			}
			// The filter requires the user to be present, so this removes one (#16).
			update = bson.M{"$set": set, "$unset": unset, "$inc": bson.M{"stats.participantCount": -1}}
		}

		res, err := chats.UpdateOne(ctx, filter, update)
		if err != nil {
			return fmt.Errorf("failed to leave chat %s: %w", chatID.Hex(), err)
		}
		if res.MatchedCount == 1 {
			return nil
		}
	}
	return fmt.Errorf("leave chat %s: it kept changing", chatID.Hex())
}

// successorOnDelete picks who owns a group after its owner userID is deleted (#35):
// another owner if there is one, else the earliest-joined admin, else the
// earliest-joined member. Only accepted members count: a pending invitee hasn't
// agreed to be in the group. Ties go to the lower ID, so the choice is stable.
func successorOnDelete(participants map[bson.ObjectID]models.ParticipantEmbed, userID bson.ObjectID) (bson.ObjectID, bool) {
	rank := map[string]int{
		string(constants.ChatRoleOwner):  0,
		string(constants.ChatRoleAdmin):  1,
		string(constants.ChatRoleMember): 2,
	}
	var ids []bson.ObjectID
	for id, p := range participants {
		if _, known := rank[p.Role]; id != userID && known && p.RequestStatus == string(constants.StatusAccepted) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return bson.ObjectID{}, false
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := participants[ids[i]], participants[ids[j]]
		if rank[a.Role] != rank[b.Role] {
			return rank[a.Role] < rank[b.Role]
		}
		if !a.JoinedAt.Equal(b.JoinedAt) {
			return a.JoinedAt.Before(b.JoinedAt)
		}
		return ids[i].Hex() < ids[j].Hex()
	})
	return ids[0], true
}

// anonymizeMessages replaces the name and avatar copied into userID's messages
// (#35). senderId has no index; this runs only when an account is deleted.
func anonymizeMessages(ctx context.Context, userID bson.ObjectID) error {
	if _, err := database.DB.Collection(string(constants.MessageColl)).UpdateMany(ctx,
		bson.M{"senderId": userID},
		bson.M{"$set": bson.M{
			"sender.displayName": DeletedUserName,
			"sender.username":    "",
			"sender.avatar":      "",
		}},
	); err != nil {
		return fmt.Errorf("failed to anonymize the user's messages: %w", err)
	}
	return nil
}
