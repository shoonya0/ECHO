package services

import (
	"context"
	"fmt"
	"time"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// BackfillGroupOwnerRoles gives each group's owner (chat.ownerId) the "owner" participant
// role. Groups created before #18 stored every participant, the creator included, as
// "member", which left nobody allowed to add or remove members. Idempotent: it only
// updates groups whose owner is a participant without the owner role. Returns the number
// of groups updated.
func BackfillGroupOwnerRoles(ctx context.Context) (int64, error) {
	chats := database.DB.Collection(string(constants.ChatColl))
	cur, err := chats.Find(ctx,
		bson.M{"chatType": string(constants.ChatTypeGroup), "ownerId": bson.M{"$exists": true}},
		options.Find().SetProjection(bson.M{"ownerId": 1}),
	)
	if err != nil {
		return 0, fmt.Errorf("find groups: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }() // read-only cursor; a close error changes nothing

	var updated int64
	for cur.Next(ctx) {
		var g struct {
			ID      bson.ObjectID `bson:"_id"`
			OwnerID bson.ObjectID `bson:"ownerId"`
		}
		if err := cur.Decode(&g); err != nil {
			return updated, fmt.Errorf("decode group: %w", err)
		}
		if g.OwnerID.IsZero() {
			continue
		}
		key := "participants." + g.OwnerID.Hex()
		res, err := chats.UpdateOne(ctx,
			bson.M{
				"_id":         g.ID,
				key:           bson.M{"$exists": true},
				key + ".role": bson.M{"$ne": string(constants.ChatRoleOwner)},
			},
			bson.M{"$set": bson.M{key + ".role": string(constants.ChatRoleOwner)}},
		)
		if err != nil {
			return updated, fmt.Errorf("update group %s: %w", g.ID.Hex(), err)
		}
		updated += res.ModifiedCount
	}
	if err := cur.Err(); err != nil {
		return updated, fmt.Errorf("iterate groups: %w", err)
	}
	return updated, nil
}

// MigrateLegacyBlocks moves blocks out of the old contactInfo.blockedChats field (#26).
// Before the fix a block was written to both users, so the field can't tell who blocked
// whom. Each pair is kept as a mutual block: both users get the other in blocked and
// blockedBy, so nobody is unblocked without choosing to be, and either can lift their own
// half. Idempotent: a user is done once blockedChats is removed, and that happens last,
// so an interrupted run is finished by the next one. Returns the number of users migrated.
func MigrateLegacyBlocks(ctx context.Context) (int64, error) {
	users := database.DB.Collection(string(constants.UserColl))
	cur, err := users.Find(ctx,
		bson.M{"contactInfo.blockedChats": bson.M{"$exists": true}},
		options.Find().SetProjection(bson.M{"contactInfo.blockedChats": 1}),
	)
	if err != nil {
		return 0, fmt.Errorf("find legacy blocks: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }() // read-only cursor; a close error changes nothing

	var migrated int64
	for cur.Next(ctx) {
		var u struct {
			ID          bson.ObjectID `bson:"_id"`
			ContactInfo struct {
				BlockedChats []bson.ObjectID `bson:"blockedChats"`
			} `bson:"contactInfo"`
		}
		if err := cur.Decode(&u); err != nil {
			return migrated, fmt.Errorf("decode user: %w", err)
		}

		// The other side first, so the block survives if this user's update fails.
		// A missing other user (deleted account) matches nothing, which is fine.
		for _, other := range u.ContactInfo.BlockedChats {
			if _, err := users.UpdateOne(ctx, bson.M{"_id": other}, bson.M{"$addToSet": bson.M{
				"contactInfo.blocked":   u.ID,
				"contactInfo.blockedBy": u.ID,
			}}); err != nil {
				return migrated, fmt.Errorf("migrate block %s -> %s: %w", other.Hex(), u.ID.Hex(), err)
			}
		}

		ids := u.ContactInfo.BlockedChats
		if ids == nil {
			ids = []bson.ObjectID{}
		}
		res, err := users.UpdateOne(ctx, bson.M{"_id": u.ID}, bson.M{
			"$addToSet": bson.M{
				"contactInfo.blocked":   bson.M{"$each": ids},
				"contactInfo.blockedBy": bson.M{"$each": ids},
			},
			"$unset": bson.M{"contactInfo.blockedChats": ""},
		})
		if err != nil {
			return migrated, fmt.Errorf("migrate blocks of %s: %w", u.ID.Hex(), err)
		}
		migrated += res.ModifiedCount
	}
	if err := cur.Err(); err != nil {
		return migrated, fmt.Errorf("iterate users: %w", err)
	}
	return migrated, nil
}

// BackfillParticipantCounts sets stats.participantCount on every group to the number of
// entries in participants. Before #16 adds and joins never raised the count and removals
// lowered it, so it drifted and could go negative. Idempotent: it only updates groups
// whose count differs. A group whose participants isn't a document is skipped, so one
// malformed group can't fail the update for all the others. Returns the number of
// groups updated.
func BackfillParticipantCounts(ctx context.Context) (int64, error) {
	size := bson.M{"$size": bson.M{"$objectToArray": "$participants"}}
	res, err := database.DB.Collection(string(constants.ChatColl)).UpdateMany(ctx,
		bson.M{
			"chatType":     string(constants.ChatTypeGroup),
			"participants": bson.M{"$type": "object"},
			"$expr":        bson.M{"$ne": bson.A{"$stats.participantCount", size}},
		},
		mongo.Pipeline{{{Key: "$set", Value: bson.M{"stats.participantCount": size}}}},
	)
	if err != nil {
		return 0, fmt.Errorf("backfill participant counts: %w", err)
	}
	return res.ModifiedCount, nil
}

// NormalizeEmails stores every email trimmed and in lower case, the form signup and login
// use since #36. Signup and login already match emails ignoring case, so this is a cleanup,
// not something logins wait for. A user whose normalized email is already another account's
// is left as it is and returned in conflicts: merging two accounts is a person's call. That
// is checked here, not left to the unique email index, which may be missing. The
// comparison is done in Go, with the same function signup and login use, so it agrees with
// them on every character. Idempotent. Returns the number of users updated and the IDs of
// the conflicting ones.
func NormalizeEmails(ctx context.Context) (updated int64, conflicts []bson.ObjectID, err error) {
	users := database.DB.Collection(string(constants.UserColl))
	cur, err := users.Find(ctx,
		bson.M{"email": bson.M{"$type": "string"}},
		options.Find().SetProjection(bson.M{"email": 1}),
	)
	if err != nil {
		return 0, nil, fmt.Errorf("find emails: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }() // read-only cursor; a close error changes nothing

	for cur.Next(ctx) {
		var u struct {
			ID    bson.ObjectID `bson:"_id"`
			Email string        `bson:"email"`
		}
		if err := cur.Decode(&u); err != nil {
			return updated, conflicts, fmt.Errorf("decode user: %w", err)
		}
		normalized := utils.NormalizeEmail(u.Email)
		if normalized == u.Email {
			continue
		}
		taken, err := users.CountDocuments(ctx, bson.M{"email": normalized, "_id": bson.M{"$ne": u.ID}})
		if err != nil {
			return updated, conflicts, fmt.Errorf("check email of %s: %w", u.ID.Hex(), err)
		}
		if taken > 0 {
			conflicts = append(conflicts, u.ID)
			continue
		}
		// Filtered on the old value, so a change in between isn't overwritten.
		res, err := users.UpdateOne(ctx,
			bson.M{"_id": u.ID, "email": u.Email},
			bson.M{"$set": bson.M{"email": normalized}},
		)
		if mongo.IsDuplicateKeyError(err) { // a signup took it since the check
			conflicts = append(conflicts, u.ID)
			continue
		}
		if err != nil {
			return updated, conflicts, fmt.Errorf("normalize email of %s: %w", u.ID.Hex(), err)
		}
		updated += res.ModifiedCount
	}
	if err := cur.Err(); err != nil {
		return updated, conflicts, fmt.Errorf("iterate users: %w", err)
	}
	return updated, conflicts, nil
}

// AcceptLegacyPendingMembers accepts group members made pending before #5, when nobody
// could accept and pending members had full access; the choice was to keep that access.
// They are told apart from invitees since #5 by having no chats entry for the group:
// every invite since then writes one. The member is accepted first and the entry
// written after. A run stopped in between leaves an accepted member without an entry
// (no live messages until a join by code writes it), rather than a legacy member who
// looks like a new invitee. A member whose account is gone is left as it is (#35). Idempotent: an
// accepted member, or one with an entry, is skipped. Returns the number accepted.
func AcceptLegacyPendingMembers(ctx context.Context) (int64, error) {
	chats := database.DB.Collection(string(constants.ChatColl))
	users := database.DB.Collection(string(constants.UserColl))

	// Each group's pending member IDs, as strings, so one malformed entry can't
	// fail the decode of the rest.
	cur, err := chats.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"chatType": string(constants.ChatTypeGroup), "participants": bson.M{"$type": "object"}}}},
		{{Key: "$project", Value: bson.M{"pending": bson.M{"$map": bson.M{
			"input": bson.M{"$filter": bson.M{
				"input": bson.M{"$objectToArray": "$participants"},
				"cond":  bson.M{"$eq": bson.A{"$$this.v.requestStatus", string(constants.StatusPending)}},
			}},
			"in": "$$this.k",
		}}}}},
		{{Key: "$match", Value: bson.M{"pending.0": bson.M{"$exists": true}}}},
	})
	if err != nil {
		return 0, fmt.Errorf("find pending members: %w", err)
	}
	var groups []struct {
		ID      bson.ObjectID `bson:"_id"`
		Pending []string      `bson:"pending"`
	}
	if err := cur.All(ctx, &groups); err != nil {
		return 0, fmt.Errorf("decode pending members: %w", err)
	}

	var accepted int64
	for _, g := range groups {
		entry := "chats." + g.ID.Hex()
		for _, hex := range g.Pending {
			userID, err := bson.ObjectIDFromHex(hex)
			if err != nil {
				continue // not a user ID; nothing to accept
			}
			n, err := users.CountDocuments(ctx, bson.M{"_id": userID, entry: bson.M{"$exists": false}})
			if err != nil {
				return accepted, fmt.Errorf("check user %s: %w", hex, err)
			}
			if n == 0 {
				continue // invited since #5, or the account is gone
			}
			now := time.Now()
			key := "participants." + hex
			res, err := chats.UpdateOne(ctx,
				bson.M{"_id": g.ID, key + ".requestStatus": string(constants.StatusPending)},
				bson.M{"$set": bson.M{
					key + ".requestStatus": string(constants.StatusAccepted),
					key + ".joinedAt":      now,
					key + ".updatedAt":     now,
				}},
			)
			if err != nil {
				return accepted, fmt.Errorf("accept %s in %s: %w", hex, g.ID.Hex(), err)
			}
			if res.ModifiedCount == 0 {
				continue // answered meanwhile
			}
			accepted++
			if _, err := users.UpdateOne(ctx, bson.M{"_id": userID}, bson.M{"$max": bson.M{entry: 0}}); err != nil {
				return accepted, fmt.Errorf("add %s to the chats of %s: %w", g.ID.Hex(), hex, err)
			}
		}
	}
	return accepted, nil
}
