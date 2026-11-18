package services

import (
	"context"
	"errors"
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

// forgedEntriesMarker is the migrations document that records RemoveForgedChatEntries
// finished a run without errors.
const forgedEntriesMarker = "remove-forged-chat-entries-69"

// RemoveForgedChatEntries removes chats.<id> entries from users whose chat doesn't
// list them in participants, or no longer exists (#69). Before #9 was fixed, a forged
// invite code wrote such an entry without making the user a participant. Pending
// members (#5) are participants, so their entries stay. Every write path adds the
// participant before the entry, so an entry for a join in progress is kept; one that
// a join adds while it is being removed is written back. It runs until one run
// finishes without errors, then never again: the code that wrote these is gone. A
// failure on one user doesn't stop the others; all failures are returned together.
// Returns the number of entries removed, which also shows whether forged joins
// happened.
func RemoveForgedChatEntries(ctx context.Context) (int64, error) {
	migrations := database.DB.Collection(string(constants.MigrationColl))
	done, err := migrations.CountDocuments(ctx, bson.M{"_id": forgedEntriesMarker})
	if err != nil {
		return 0, fmt.Errorf("read the marker: %w", err)
	}
	if done > 0 {
		return 0, nil
	}

	users := database.DB.Collection(string(constants.UserColl))
	chats := database.DB.Collection(string(constants.ChatColl))
	cur, err := users.Find(ctx,
		bson.M{"chats": bson.M{"$type": "object", "$ne": bson.M{}}},
		options.Find().SetProjection(bson.M{"chats": 1}),
	)
	if err != nil {
		return 0, fmt.Errorf("find users with chats: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }() // read-only cursor; a close error changes nothing

	var removed int64
	var errs []error
	for cur.Next(ctx) {
		var u struct {
			ID    bson.ObjectID `bson:"_id"`
			Chats bson.M        `bson:"chats"`
		}
		if err := cur.Decode(&u); err != nil {
			errs = append(errs, fmt.Errorf("decode user: %w", err))
			continue
		}
		listed := make([]bson.ObjectID, 0, len(u.Chats))
		for key := range u.Chats {
			if id, err := bson.ObjectIDFromHex(key); err == nil {
				listed = append(listed, id)
			}
		}
		if len(listed) == 0 {
			continue
		}
		member, err := chatsListing(ctx, chats, listed, u.ID)
		if err != nil {
			errs = append(errs, fmt.Errorf("check chats of %s: %w", u.ID.Hex(), err))
			continue
		}
		unset := bson.M{}
		var stale []bson.ObjectID
		for _, id := range listed {
			if !member[id] {
				unset["chats."+id.Hex()] = ""
				stale = append(stale, id)
			}
		}
		if len(stale) == 0 {
			continue
		}
		if _, err := users.UpdateOne(ctx, bson.M{"_id": u.ID}, bson.M{"$unset": unset}); err != nil {
			errs = append(errs, fmt.Errorf("remove entries of %s: %w", u.ID.Hex(), err))
			continue
		}
		// A join that made the user a participant after the check above kept its
		// entry ($max on one already there), so the unset took it: write it back.
		joined, err := chatsListing(ctx, chats, stale, u.ID)
		if err != nil {
			errs = append(errs, fmt.Errorf("recheck chats of %s: %w", u.ID.Hex(), err))
			continue
		}
		restore := bson.M{}
		for id := range joined {
			restore["chats."+id.Hex()] = 0
		}
		if len(restore) > 0 {
			if _, err := users.UpdateOne(ctx, bson.M{"_id": u.ID}, bson.M{"$max": restore}); err != nil {
				errs = append(errs, fmt.Errorf("restore entries of %s: %w", u.ID.Hex(), err))
			}
		}
		removed += int64(len(stale) - len(joined))
	}
	if err := cur.Err(); err != nil {
		errs = append(errs, fmt.Errorf("iterate users: %w", err))
	}
	if len(errs) > 0 {
		return removed, errors.Join(errs...)
	}
	if _, err := migrations.UpdateOne(ctx,
		bson.M{"_id": forgedEntriesMarker},
		bson.M{"$setOnInsert": bson.M{"finishedAt": time.Now(), "removed": removed}},
		options.UpdateOne().SetUpsert(true),
	); err != nil {
		return removed, fmt.Errorf("write the marker: %w", err)
	}
	return removed, nil
}

// chatsListing returns which of ids are chats that list userID in participants.
func chatsListing(ctx context.Context, chats *mongo.Collection, ids []bson.ObjectID, userID bson.ObjectID) (map[bson.ObjectID]bool, error) {
	cur, err := chats.Find(ctx,
		bson.M{"_id": bson.M{"$in": ids}, participantField(userID): bson.M{"$exists": true}},
		options.Find().SetProjection(bson.M{"_id": 1}),
	)
	if err != nil {
		return nil, err
	}
	var found []struct {
		ID bson.ObjectID `bson:"_id"`
	}
	if err := cur.All(ctx, &found); err != nil {
		return nil, err
	}
	member := make(map[bson.ObjectID]bool, len(found))
	for _, c := range found {
		member[c.ID] = true
	}
	return member, nil
}

// legacyPendingMarker is the migrations document that stores the cutoff of
// AcceptLegacyPendingMembers: the start of the first server that ran it.
const legacyPendingMarker = "accept-legacy-pending-5"

// AcceptLegacyPendingMembers accepts group members made pending before #5, when nobody
// could accept and pending members had full access; the choice was to keep that access.
// "Before" is a cutoff stored on the first run: startedAt of the first server running
// this code, taken before it served requests, so every invite this code made is later.
// A later start keeps the first cutoff. Each legacy member gets the group's chats entry,
// then is accepted, so a run stopped in between is finished by the next one. A pending
// member from after the cutoff is never accepted here; one whose chats entry is missing
// (a write that failed or didn't finish) gets it, so the invite shows in their list.
// Members whose account is gone are left as they are (#35). A failure on one member
// doesn't stop the others; all failures are returned together. Idempotent. Returns
// the number of members accepted.
func AcceptLegacyPendingMembers(ctx context.Context, startedAt time.Time) (int64, error) {
	chats := database.DB.Collection(string(constants.ChatColl))
	users := database.DB.Collection(string(constants.UserColl))

	var marker struct {
		Cutoff time.Time `bson:"cutoff"`
	}
	err := database.DB.Collection(string(constants.MigrationColl)).FindOneAndUpdate(ctx,
		bson.M{"_id": legacyPendingMarker},
		bson.M{"$setOnInsert": bson.M{"cutoff": startedAt}},
		options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After),
	).Decode(&marker)
	if err != nil {
		return 0, fmt.Errorf("read the cutoff: %w", err)
	}

	// Each group's pending members with when they became pending, as strings and
	// dates, so one malformed entry can't fail the decode of the rest.
	cur, err := chats.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"chatType": string(constants.ChatTypeGroup), "participants": bson.M{"$type": "object"}}}},
		{{Key: "$project", Value: bson.M{"pending": bson.M{"$map": bson.M{
			"input": bson.M{"$filter": bson.M{
				"input": bson.M{"$objectToArray": "$participants"},
				"cond":  bson.M{"$eq": bson.A{"$$this.v.requestStatus", string(constants.StatusPending)}},
			}},
			"in": bson.M{"id": "$$this.k", "createdAt": "$$this.v.createdAt"},
		}}}}},
		{{Key: "$match", Value: bson.M{"pending.0": bson.M{"$exists": true}}}},
	})
	if err != nil {
		return 0, fmt.Errorf("find pending members: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }() // read-only cursor; a close error changes nothing

	var accepted int64
	var errs []error
	for cur.Next(ctx) {
		var g struct {
			ID      bson.ObjectID `bson:"_id"`
			Pending []struct {
				ID        string    `bson:"id"`
				CreatedAt time.Time `bson:"createdAt"` // zero when missing: made before #5
			} `bson:"pending"`
		}
		if err := cur.Decode(&g); err != nil {
			errs = append(errs, fmt.Errorf("decode group: %w", err))
			continue
		}
		entry := "chats." + g.ID.Hex()
		var legacy, recent []bson.ObjectID
		for _, p := range g.Pending {
			id, err := bson.ObjectIDFromHex(p.ID)
			if err != nil {
				continue // not a user ID; nothing to accept
			}
			if p.CreatedAt.Before(marker.Cutoff) {
				legacy = append(legacy, id)
			} else {
				recent = append(recent, id)
			}
		}

		if len(recent) > 0 {
			if _, err := users.UpdateMany(ctx,
				bson.M{"_id": bson.M{"$in": recent}, entry: bson.M{"$exists": false}},
				bson.M{"$set": bson.M{entry: 0}},
			); err != nil {
				errs = append(errs, fmt.Errorf("add %s to invitees' chats: %w", g.ID.Hex(), err))
			}
		}
		for _, id := range legacy {
			res, err := users.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$max": bson.M{entry: 0}})
			if err != nil {
				errs = append(errs, fmt.Errorf("add %s to the chats of %s: %w", g.ID.Hex(), id.Hex(), err))
				continue
			}
			if res.MatchedCount == 0 {
				continue // the account is gone (#35)
			}
			res, err = chats.UpdateOne(ctx,
				bson.M{"_id": g.ID, participantField(id) + ".requestStatus": string(constants.StatusPending)},
				bson.M{"$set": acceptedFields(id, time.Now())},
			)
			if err != nil {
				errs = append(errs, fmt.Errorf("accept %s in %s: %w", id.Hex(), g.ID.Hex(), err))
				continue
			}
			accepted += res.ModifiedCount // 0: answered meanwhile
		}
	}
	if err := cur.Err(); err != nil {
		errs = append(errs, fmt.Errorf("iterate groups: %w", err))
	}
	return accepted, errors.Join(errs...)
}
