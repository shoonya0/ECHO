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

// scrubAccountReferences removes or anonymizes everything that points at userID
// (#35). Every step is idempotent.
func scrubAccountReferences(ctx context.Context, userID bson.ObjectID) error {
	if err := scrubContacts(ctx, userID); err != nil {
		return err
	}
	chatIDs, err := chatsWithParticipant(ctx, userID)
	if err != nil {
		return err
	}
	if err := leaveChatsOnDelete(ctx, userID, chatIDs); err != nil {
		return err
	}
	return anonymizeMessages(ctx, userID)
}

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
// the entry stays, anonymized, so the other person keeps the history. In a group or a
// channel the user is removed, and an owner hands it on (ownershipOnDelete). Each
// update re-checks what it read, and a chat that changed meanwhile is read again.
func leaveChatOnDelete(ctx context.Context, chatID, userID bson.ObjectID) error {
	chats := database.DB.Collection(string(constants.ChatColl))
	key := participantField(userID)
	for range leaveAttempts {
		var chat models.Chat
		err := chats.FindOne(ctx, bson.M{"_id": chatID},
			options.FindOne().SetProjection(bson.M{"chatType": 1, "ownerId": 1, "participants": 1, "inviteCode.inviteCode": 1}),
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
		if chat.ChatType == string(constants.ChatTypeDirect) {
			update = bson.M{"$set": bson.M{key + ".userInfo": models.ContactUserInfo{
				UserID: userID, DisplayName: DeletedUserName,
			}}}
		} else {
			// Pin what the ownership choice was made from, so a concurrent transfer
			// or role change isn't undone (code review of #35). {ownerId: nil} also
			// matches a group that stores none.
			filter[key+".role"] = me.Role
			if chat.OwnerID.IsZero() {
				filter["ownerId"] = nil
			} else {
				filter["ownerId"] = chat.OwnerID
			}
			set := bson.M{"updatedAt": time.Now()}
			unset := bson.M{key: ""}
			ownershipOnDelete(chat, userID, filter, set, unset)
			// The filter requires the user to be present, so this removes one (#16).
			update = bson.M{
				"$set": set, "$unset": unset,
				"$inc":  bson.M{"stats.participantCount": -1},
				"$pull": bson.M{"adminIds": userID},
			}
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

// ownershipOnDelete adds to the update that takes userID out of the group chat what
// ownership needs (#35). A plain member, or a co-owner while another owner stays,
// changes nothing. The primary owner (ownerId), or the last owner, hands the group to
// successorOnDelete's pick, who also becomes ownerId if userID held it or it names
// nobody in the group. With no one to hand it to, ownerId is cleared and every invite
// code is deleted: nobody is left to moderate who joins.
func ownershipOnDelete(chat models.Chat, userID bson.ObjectID, filter, set, unset bson.M) {
	isPrimary := chat.OwnerID == userID
	_, primaryStays := chat.Participants[chat.OwnerID]
	primaryStays = primaryStays && !isPrimary
	otherOwner := false
	for id, p := range chat.Participants {
		if id != userID && p.Role == string(constants.ChatRoleOwner) && p.RequestStatus == string(constants.StatusAccepted) {
			otherOwner = true
		}
	}
	if !isPrimary && (chat.Participants[userID].Role != string(constants.ChatRoleOwner) || otherOwner) {
		return
	}

	if next, ok := successorOnDelete(chat.Participants, userID); ok {
		nextKey := participantField(next)
		filter[nextKey+".role"] = chat.Participants[next].Role
		set[nextKey+".role"] = string(constants.ChatRoleOwner)
		if !primaryStays {
			set["ownerId"] = next
		}
		return
	}
	if isPrimary {
		unset["ownerId"] = ""
	}
	if !primaryStays && len(chat.InviteCode) > 0 {
		filter["inviteCode"] = bson.M{"$type": "array"}
		set["inviteCode.$[].deleted"] = true
	}
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
