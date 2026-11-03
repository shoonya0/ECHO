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
