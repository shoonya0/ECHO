package services

import (
	"context"
	"fmt"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"

	"go.mongodb.org/mongo-driver/v2/bson"
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
