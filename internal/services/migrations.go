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

// BackfillDirectKeys keeps one keyed direct chat per pair (#37), and runs on every start
// before the indexes are built.
//
// First, it takes the key off the newer of two chats with the same key: they can only
// exist if directKey_1 was missing when they were made (a fresh deploy creates it after
// the server starts serving), and they would stop it from ever being built. Then it keys
// direct chats that have none: chats from before #37, and any an older server copy
// creates during a deploy. Both passes go oldest first, so a pair keeps its first chat.
// A pair's later chats stay unkeyed and are returned in duplicates: merging two chats'
// messages is a person's call. Keys in use are tracked in memory, so this needs neither
// the index nor a query per chat. A chat without exactly two participants is skipped.
// Returns the number of chats keyed and the IDs of the duplicates.
func BackfillDirectKeys(ctx context.Context) (keyed int64, duplicates []bson.ObjectID, err error) {
	chats := database.DB.Collection(string(constants.ChatColl))
	oldestFirst := bson.D{{Key: "_id", Value: 1}}

	used := make(map[string]bool)
	cur, err := chats.Find(ctx, bson.M{"directKey": bson.M{"$type": "string"}},
		options.Find().SetProjection(bson.M{"directKey": 1}).SetSort(oldestFirst))
	if err != nil {
		return 0, nil, fmt.Errorf("find keyed chats: %w", err)
	}
	var clashing []bson.ObjectID
	for cur.Next(ctx) {
		var c struct {
			ID        bson.ObjectID `bson:"_id"`
			DirectKey string        `bson:"directKey"`
		}
		if err := cur.Decode(&c); err != nil {
			_ = cur.Close(ctx)
			return 0, nil, fmt.Errorf("decode keyed chat: %w", err)
		}
		if used[c.DirectKey] {
			clashing = append(clashing, c.ID)
		}
		used[c.DirectKey] = true
	}
	err = cur.Err()
	_ = cur.Close(ctx) // read-only cursor; a close error changes nothing
	if err != nil {
		return 0, nil, fmt.Errorf("iterate keyed chats: %w", err)
	}
	if len(clashing) > 0 {
		if _, err := chats.UpdateMany(ctx, bson.M{"_id": bson.M{"$in": clashing}},
			bson.M{"$unset": bson.M{"directKey": ""}}); err != nil {
			return 0, nil, fmt.Errorf("unkey duplicate direct chats: %w", err)
		}
	}

	cur, err = chats.Find(ctx,
		bson.M{"chatType": "direct", "directKey": bson.M{"$exists": false}},
		options.Find().SetProjection(bson.M{"participants": 1}).SetSort(oldestFirst),
	)
	if err != nil {
		return 0, nil, fmt.Errorf("find direct chats: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }() // read-only cursor; a close error changes nothing

	for cur.Next(ctx) {
		var c struct {
			ID           bson.ObjectID       `bson:"_id"`
			Participants map[string]bson.Raw `bson:"participants"`
		}
		if err := cur.Decode(&c); err != nil {
			return keyed, duplicates, fmt.Errorf("decode chat: %w", err)
		}
		pair := make([]bson.ObjectID, 0, 2)
		for key := range c.Participants {
			if id, err := bson.ObjectIDFromHex(key); err == nil {
				pair = append(pair, id)
			}
		}
		if len(pair) != 2 {
			continue
		}
		key := directChatKey(pair[0], pair[1])
		if used[key] {
			duplicates = append(duplicates, c.ID)
			continue
		}
		res, err := chats.UpdateOne(ctx,
			bson.M{"_id": c.ID, "directKey": bson.M{"$exists": false}},
			bson.M{"$set": bson.M{"directKey": key}},
		)
		if mongo.IsDuplicateKeyError(err) { // a new chat of the pair took it meanwhile
			duplicates = append(duplicates, c.ID)
			continue
		}
		if err != nil {
			return keyed, duplicates, fmt.Errorf("key chat %s: %w", c.ID.Hex(), err)
		}
		used[key] = true
		keyed += res.ModifiedCount
	}
	if err := cur.Err(); err != nil {
		return keyed, duplicates, fmt.Errorf("iterate direct chats: %w", err)
	}
	return keyed, duplicates, nil
}

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

// staleEntriesBatch is how many users RemoveStaleChatEntries checks per chats query.
var staleEntriesBatch = 200 // a variable so tests can make batches small

// RemoveStaleChatEntries removes chats.<id> entries from users whose chat doesn't list
// them in participants, or no longer exists. Forged invite joins left these before #9
// was fixed (#69); a cleanup write that fails after a decline, a removal or an undone
// direct chat still can, so it runs on every start. Pending members (#5) are
// participants, so their entries stay. Every write path adds the participant before the
// entry, so a join in progress keeps its entry. A join that lands between the check and
// the removal is written back, with an unread count of 0; a removal in that window can
// leave an entry, which the next start removes. Closing those windows needs
// transactions (#37). Users are checked in batches, one chats query per batch. A failure
// in one batch or user doesn't stop the others; all failures are returned together.
// Returns the number of entries removed.
func RemoveStaleChatEntries(ctx context.Context) (int64, error) {
	cur, err := database.DB.Collection(string(constants.UserColl)).Find(ctx,
		bson.M{"chats": bson.M{"$type": "object", "$ne": bson.M{}}},
		options.Find().SetProjection(bson.M{"chats": 1}).SetBatchSize(int32(staleEntriesBatch)),
	)
	if err != nil {
		return 0, fmt.Errorf("find users with chats: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }() // read-only cursor; a close error changes nothing

	var removed int64
	var errs []error
	batch := make([]userChatEntries, 0, staleEntriesBatch)
	flush := func() {
		n, err := removeStaleEntriesOf(ctx, batch)
		removed += n
		if err != nil {
			errs = append(errs, err)
		}
		batch = batch[:0]
	}
	for cur.Next(ctx) {
		var u struct {
			ID    bson.ObjectID `bson:"_id"`
			Chats bson.M        `bson:"chats"`
		}
		if err := cur.Decode(&u); err != nil {
			errs = append(errs, fmt.Errorf("decode user: %w", err))
			continue
		}
		e := userChatEntries{user: u.ID}
		for key := range u.Chats {
			if id, err := bson.ObjectIDFromHex(key); err == nil {
				e.chats = append(e.chats, id)
			}
		}
		if len(e.chats) == 0 {
			continue
		}
		if batch = append(batch, e); len(batch) == staleEntriesBatch {
			flush()
		}
	}
	if err := cur.Err(); err != nil {
		errs = append(errs, fmt.Errorf("iterate users: %w", err))
	}
	if len(batch) > 0 {
		flush()
	}
	return removed, errors.Join(errs...)
}

// userChatEntries is a user and the chat IDs in their chats list.
type userChatEntries struct {
	user  bson.ObjectID
	chats []bson.ObjectID
}

// removeStaleEntriesOf removes, for each user in batch, the entries whose chat doesn't
// list them, and writes back those whose user became a participant meanwhile.
func removeStaleEntriesOf(ctx context.Context, batch []userChatEntries) (int64, error) {
	var all []bson.ObjectID
	for _, e := range batch {
		all = append(all, e.chats...)
	}
	members, err := chatParticipantKeys(ctx, all)
	if err != nil {
		return 0, err
	}

	users := database.DB.Collection(string(constants.UserColl))
	var removed int64
	var errs []error
	stale := map[bson.ObjectID][]bson.ObjectID{}
	var staleChats []bson.ObjectID
	for _, e := range batch {
		unset := bson.M{}
		var ids []bson.ObjectID
		for _, id := range e.chats {
			if !members[id][e.user.Hex()] {
				unset["chats."+id.Hex()] = ""
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			continue
		}
		res, err := users.UpdateOne(ctx, bson.M{"_id": e.user}, bson.M{"$unset": unset})
		if err != nil {
			errs = append(errs, fmt.Errorf("remove entries of %s: %w", e.user.Hex(), err))
			continue
		}
		if res.MatchedCount == 0 {
			continue // the account is gone (#35)
		}
		removed += int64(len(ids))
		stale[e.user] = ids
		staleChats = append(staleChats, ids...)
	}
	if len(stale) == 0 {
		return removed, errors.Join(errs...)
	}

	// A join that made the user a participant after the check above kept its entry
	// ($max on one already there), so the unset took it: write it back.
	joined, err := chatParticipantKeys(ctx, staleChats)
	if err != nil {
		return removed, errors.Join(append(errs, fmt.Errorf("recheck: %w", err))...)
	}
	for user, ids := range stale {
		restore := bson.M{}
		for _, id := range ids {
			if joined[id][user.Hex()] {
				restore["chats."+id.Hex()] = 0
			}
		}
		if len(restore) == 0 {
			continue
		}
		res, err := users.UpdateOne(ctx, bson.M{"_id": user}, bson.M{"$max": restore})
		if err != nil {
			errs = append(errs, fmt.Errorf("restore entries of %s: %w", user.Hex(), err))
			continue
		}
		if res.MatchedCount > 0 {
			removed -= int64(len(restore))
		}
	}
	return removed, errors.Join(errs...)
}

// chatParticipantKeys returns, for each of chatIDs that exists, the hex IDs of its
// participants. A chat whose participants isn't a document has none.
func chatParticipantKeys(ctx context.Context, chatIDs []bson.ObjectID) (map[bson.ObjectID]map[string]bool, error) {
	out := make(map[bson.ObjectID]map[string]bool, len(chatIDs))
	if len(chatIDs) == 0 {
		return out, nil
	}
	cur, err := database.DB.Collection(string(constants.ChatColl)).Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"_id": bson.M{"$in": chatIDs}}}},
		{{Key: "$project", Value: bson.M{"members": bson.M{"$cond": bson.A{
			bson.M{"$eq": bson.A{bson.M{"$type": "$participants"}, "object"}},
			bson.M{"$map": bson.M{"input": bson.M{"$objectToArray": "$participants"}, "in": "$$this.k"}},
			bson.A{},
		}}}}},
	})
	if err != nil {
		return nil, fmt.Errorf("find participants: %w", err)
	}
	var found []struct {
		ID      bson.ObjectID `bson:"_id"`
		Members []string      `bson:"members"`
	}
	if err := cur.All(ctx, &found); err != nil {
		return nil, fmt.Errorf("decode participants: %w", err)
	}
	for _, c := range found {
		set := make(map[string]bool, len(c.Members))
		for _, m := range c.Members {
			set[m] = true
		}
		out[c.ID] = set
	}
	return out, nil
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

// showLastSeenMarker is the migrations document that records that
// EnableShowLastSeen ran.
const showLastSeenMarker = "show-last-seen-default-32"

// EnableShowLastSeen turns settings.privacy.showLastSeen on for every account,
// once. Accounts were created with it off while nothing read it, so nobody
// chose it; applying it as stored would have hidden everyone's lastSeen (#32).
// The run that creates the marker does the update; every later one does
// nothing, so a user's later choice is never undone. An update that fails
// after the marker is claimed isn't retried: lastSeen stays hidden for the
// accounts it missed, the safe side. Returns the number of accounts changed.
func EnableShowLastSeen(ctx context.Context) (int64, error) {
	claim, err := database.DB.Collection(string(constants.MigrationColl)).UpdateOne(ctx,
		bson.M{"_id": showLastSeenMarker},
		bson.M{"$setOnInsert": bson.M{"startedAt": time.Now()}}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		return 0, fmt.Errorf("claim the marker: %w", err)
	}
	if claim.UpsertedCount == 0 {
		return 0, nil // ran before, or another server is running it
	}
	res, err := database.DB.Collection(string(constants.UserColl)).UpdateMany(ctx,
		bson.M{
			"settings.privacy.showLastSeen": bson.M{"$ne": true},
			// A null settings or privacy can't take the field; leave those.
			"settings":         bson.M{"$not": bson.M{"$type": "null"}},
			"settings.privacy": bson.M{"$not": bson.M{"$type": "null"}},
		},
		bson.M{"$set": bson.M{"settings.privacy.showLastSeen": true}},
	)
	if err != nil {
		return 0, fmt.Errorf("turn on showLastSeen: %w", err)
	}
	return res.ModifiedCount, nil
}

// seqBackfillBatch is how many messages of a chat one backfill transaction numbers.
const seqBackfillBatch = 500

// messageSeqsMarker is the migrations document BackfillMessageSeqs sets once a final
// run found nothing left, so later starts skip the scan.
const messageSeqsMarker = "message-seqs-43"

// BackfillMessageSeqs numbers the messages that have no seq (#43): every message from
// before #43, and any an older server copy saves during the deploy of #43. A chat's
// old messages get the numbers seedChatSeqs reserved for them, oldest first; messages
// saved after the seeding by an older copy get numbers after the newest. Each batch
// is a transaction that writes the chat document, so it can't clash with a send or
// another server's backfill: one reruns. A failing chat doesn't stop the others; all
// failures are returned together. Messages of a chat that no longer exists are left.
//
// Finding the chats reads every message (no index can serve "seq is missing"), so once
// a run with final set succeeds, it records messageSeqsMarker and later calls return
// at once: after the #43 deploy no server saves a message without a seq. Returns the
// number of messages numbered.
func BackfillMessageSeqs(ctx context.Context, final bool) (int64, error) {
	markers := database.DB.Collection(string(constants.MigrationColl))
	err := markers.FindOne(ctx, bson.M{"_id": messageSeqsMarker}).Err()
	if err == nil {
		return 0, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return 0, fmt.Errorf("read the message seqs marker: %w", err)
	}

	var chatIDs []bson.ObjectID
	if err := database.DB.Collection(string(constants.MessageColl)).
		Distinct(ctx, "chatId", bson.M{"seq": bson.M{"$exists": false}}).Decode(&chatIDs); err != nil {
		return 0, fmt.Errorf("find chats with unnumbered messages: %w", err)
	}

	var numbered int64
	var errs []error
	for _, chatID := range chatIDs {
		for {
			n, err := backfillChatSeqs(ctx, chatID)
			numbered += n
			if errors.Is(err, ErrChatNotFound) {
				break
			}
			if err != nil {
				errs = append(errs, err)
				break
			}
			if n < seqBackfillBatch {
				break
			}
		}
	}
	if err := errors.Join(errs...); err != nil {
		return numbered, err
	}
	if final {
		if _, err := markers.UpdateOne(ctx, bson.M{"_id": messageSeqsMarker},
			bson.M{"$set": bson.M{"doneAt": time.Now()}}, options.UpdateOne().SetUpsert(true)); err != nil {
			return numbered, fmt.Errorf("record the message seqs marker: %w", err)
		}
	}
	return numbered, nil
}

// backfillChatSeqs numbers up to seqBackfillBatch of a chat's oldest unnumbered
// messages in one transaction and returns how many it numbered.
func backfillChatSeqs(ctx context.Context, chatID bson.ObjectID) (int64, error) {
	var numbered int64
	err := database.WithTransaction(ctx, func(ctx context.Context) error {
		numbered = 0
		if err := seedChatSeqs(ctx, chatID); err != nil {
			return err
		}
		messages := database.DB.Collection(string(constants.MessageColl))
		cur, err := messages.Find(ctx,
			bson.M{"chatId": chatID, "seq": bson.M{"$exists": false}},
			options.Find().SetProjection(bson.M{"_id": 1}).
				SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(seqBackfillBatch),
		)
		if err != nil {
			return fmt.Errorf("find unnumbered messages of chat %s: %w", chatID.Hex(), err)
		}
		var ids []struct {
			ID bson.ObjectID `bson:"_id"`
		}
		if err := cur.All(ctx, &ids); err != nil {
			return fmt.Errorf("read unnumbered messages of chat %s: %w", chatID.Hex(), err)
		}
		if len(ids) == 0 {
			return nil
		}

		var chat struct {
			Stats struct {
				OldSeqNext int64 `bson:"oldSeqNext"`
				OldSeqEnd  int64 `bson:"oldSeqEnd"`
			} `bson:"stats"`
		}
		if err := database.DB.Collection(string(constants.ChatColl)).FindOne(ctx, bson.M{"_id": chatID},
			options.FindOne().SetProjection(bson.M{"stats.oldSeqNext": 1, "stats.oldSeqEnd": 1}),
		).Decode(&chat); err != nil {
			return fmt.Errorf("read reserved seqs of chat %s: %w", chatID.Hex(), err)
		}

		// The reserved range first, then numbers after the newest for the rest.
		m := int64(len(ids))
		reserved := max(0, min(m, chat.Stats.OldSeqEnd-chat.Stats.OldSeqNext+1))
		update := bson.M{}
		if reserved > 0 {
			update["$inc"] = bson.M{"stats.oldSeqNext": reserved}
		}
		last, err := reserveChatSeqs(ctx, chatID, m-reserved, update)
		if err != nil {
			return err
		}
		seqs := make([]int64, 0, m)
		for i := range reserved {
			seqs = append(seqs, chat.Stats.OldSeqNext+i)
		}
		for i := last - (m - reserved) + 1; i <= last; i++ {
			seqs = append(seqs, i)
		}

		writes := make([]mongo.WriteModel, len(ids))
		for i, msg := range ids {
			writes[i] = mongo.NewUpdateOneModel().
				SetFilter(bson.M{"_id": msg.ID, "seq": bson.M{"$exists": false}}).
				SetUpdate(bson.M{"$set": bson.M{"seq": seqs[i]}})
		}
		res, err := messages.BulkWrite(ctx, writes)
		if err != nil {
			return fmt.Errorf("number messages of chat %s: %w", chatID.Hex(), err)
		}
		// Every message was read unnumbered in this transaction, so a miss means
		// the snapshot is wrong: rather undo than leave a gap.
		if res.MatchedCount != m {
			return fmt.Errorf("numbered %d of %d messages of chat %s", res.MatchedCount, m, chatID.Hex())
		}
		numbered = m
		return nil
	})
	return numbered, err
}

// staleReadStateMarker is the migrations document RemoveStaleReadState sets once it
// has run, so later starts don't rewrite every chat.
const staleReadStateMarker = "stale-read-state-42"

// RemoveStaleReadState takes the readReceipts and readSeqs entries of users who
// aren't participants off every chat, once (#42). Until #42, removing, declining
// and deleting an account left them, so the maps only grew; now those writes clear
// them, and mark_read writes only for participants. A chat without either map
// isn't given one. Two servers running it at once do the same harmless work. The
// marker is set after a successful run, so a failed one is retried at the next
// start. Returns the number of chats changed.
func RemoveStaleReadState(ctx context.Context) (int64, error) {
	markers := database.DB.Collection(string(constants.MigrationColl))
	err := markers.FindOne(ctx, bson.M{"_id": staleReadStateMarker}).Err()
	if err == nil {
		return 0, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return 0, fmt.Errorf("read the marker: %w", err)
	}

	members := bson.M{"$map": bson.M{
		"input": bson.M{"$objectToArray": bson.M{"$ifNull": bson.A{"$participants", bson.M{}}}},
		"in":    "$$this.k",
	}}
	// keepMembers is field with only the participants' keys while it is a map, and
	// the field unchanged otherwise (missing stays missing).
	keepMembers := func(field string) bson.M {
		return bson.M{"$cond": bson.A{
			bson.M{"$eq": bson.A{bson.M{"$type": "$" + field}, "object"}},
			bson.M{"$arrayToObject": bson.M{"$filter": bson.M{
				"input": bson.M{"$objectToArray": "$" + field},
				"cond":  bson.M{"$in": bson.A{"$$this.k", members}},
			}}},
			bson.M{"$cond": bson.A{
				bson.M{"$eq": bson.A{bson.M{"$type": "$" + field}, "missing"}},
				"$$REMOVE",
				"$" + field,
			}},
		}}
	}
	res, err := database.DB.Collection(string(constants.ChatColl)).UpdateMany(ctx,
		bson.M{"$or": bson.A{
			bson.M{"readReceipts": bson.M{"$type": "object"}},
			bson.M{"readSeqs": bson.M{"$type": "object"}},
		}},
		mongo.Pipeline{{{Key: "$set", Value: bson.M{
			"readReceipts": keepMembers("readReceipts"),
			"readSeqs":     keepMembers("readSeqs"),
		}}}},
	)
	if err != nil {
		return 0, fmt.Errorf("remove stale read state: %w", err)
	}
	if _, err := markers.UpdateOne(ctx, bson.M{"_id": staleReadStateMarker},
		bson.M{"$setOnInsert": bson.M{"doneAt": time.Now(), "chats": res.ModifiedCount}},
		options.UpdateOne().SetUpsert(true)); err != nil {
		return res.ModifiedCount, fmt.Errorf("record the marker: %w", err)
	}
	return res.ModifiedCount, nil
}
