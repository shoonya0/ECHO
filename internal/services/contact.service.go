package services

import (
	"context"
	"fmt"
	"gin/internal/models"
	"gin/objects"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ---------------------------------------------------------------------------
// Contact-list projection fields per status
// ---------------------------------------------------------------------------

var contactFieldsByStatus = map[objects.ContactStatus]struct {
	projection bson.M
	ids        func(models.ContactInfoEmbed) []bson.ObjectID
}{
	objects.StatusAccepted: {
		projection: bson.M{"_id": 1, "updatedAt": 1, "contactInfo.contacts": 1, "contactInfo.favorites": 1},
		ids:        func(c models.ContactInfoEmbed) []bson.ObjectID { return c.Contacts },
	},
	objects.StatusPendingIn: {
		projection: bson.M{"_id": 1, "updatedAt": 1, "contactInfo.pendingIn": 1},
		ids:        func(c models.ContactInfoEmbed) []bson.ObjectID { return c.PendingIn },
	},
	objects.StatusPendingOut: {
		projection: bson.M{"_id": 1, "updatedAt": 1, "contactInfo.pendingOut": 1},
		ids:        func(c models.ContactInfoEmbed) []bson.ObjectID { return c.PendingOut },
	},
	objects.StatusFavorite: {
		projection: bson.M{"_id": 1, "updatedAt": 1, "contactInfo.favorites": 1},
		ids:        func(c models.ContactInfoEmbed) []bson.ObjectID { return c.Favorites },
	},
	objects.StatusBlocked: {
		projection: bson.M{"_id": 1, "updatedAt": 1, "contactInfo.blockedChats": 1},
		ids:        func(c models.ContactInfoEmbed) []bson.ObjectID { return c.BlockedChats },
	},
	objects.StatusContact: {
		projection: bson.M{
			"_id":                      1,
			"updatedAt":                1,
			"contactInfo.contacts":     1,
			"contactInfo.favorites":    1,
			"contactInfo.pendingIn":    1,
			"contactInfo.pendingOut":   1,
			"contactInfo.blockedChats": 1,
		},
		ids: func(c models.ContactInfoEmbed) []bson.ObjectID {
			return append(append(append(append(
				c.BlockedChats,
				c.PendingOut...),
				c.PendingIn...),
				c.Favorites...),
				c.Contacts...)
		},
	},
}

// userProjection is the standard fields returned when listing contacts.
var userProjection = bson.M{
	"_id":                   1,
	"profile":               1,
	"username":              1,
	"presence.status":       1,
	"presence.isOnline":     1,
	"presence.lastActivity": 1,
	"presence.lastSeen":     1,
}

// ============ CONTACTS & FRIENDS MANAGEMENT ============

// GetContacts returns contacts/groups filtered by status for the given user.
func GetContacts(ctx context.Context, userID bson.ObjectID, contactStatus objects.ContactStatus, limit int) ([]models.ContactInfo, error) {
	cfg, ok := contactFieldsByStatus[contactStatus]
	if !ok {
		return []models.ContactInfo{}, fmt.Errorf("unknown contact status: %s", contactStatus)
	}

	var contact models.ContactRequest
	opts := options.FindOne().SetProjection(cfg.projection)
	err := objects.DB.Collection(string(objects.UserColl)).FindOne(ctx, bson.M{"_id": userID}, opts).Decode(&contact)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return []models.ContactInfo{}, nil
		}
		return []models.ContactInfo{}, err
	}

	contactIDs := cfg.ids(contact.ContactInfo)
	if len(contactIDs) == 0 {
		return []models.ContactInfo{}, nil
	}

	cursor, err := objects.DB.Collection(string(objects.UserColl)).Find(
		ctx,
		bson.M{"_id": bson.M{"$in": contactIDs}},
		options.Find().SetProjection(userProjection),
	)
	if err != nil {
		return []models.ContactInfo{}, err
	}
	defer cursor.Close(ctx)

	results := make([]models.ContactInfo, 0, len(contactIDs))
	for cursor.Next(ctx) {
		var user models.ContactInfo
		if err := cursor.Decode(&user); err != nil {
			return []models.ContactInfo{}, err
		}

		// Normalize presence: offline users show neither activity nor lastSeen.
		if user.Presence.Status != string(objects.UserStatusOnline) {
			user.Presence.IsOnline = false
			if user.Presence.Status != string(objects.UserStatusOffline) {
				user.Presence.LastActivity = time.Time{}
				user.Presence.LastSeen = time.Time{}
			}
		}

		user.IsFavorite = contains(contact.ContactInfo.Favorites, user.ID)

		results = append(results, user)
	}

	return results, nil
}

// ============= Contact Actions =============

// userContactProjection returns standard projection for contact-validation reads.
func userContactProjection() bson.M {
	return bson.M{
		"_id":                      1,
		"contactInfo.blockedChats": 1,
		"contactInfo.pendingOut":   1,
		"contactInfo.pendingIn":    1,
		"contactInfo.favorites":    1,
		"contactInfo.contacts":     1,
	}
}

// fetchUserContact fetches a user with contact-related projection.
func fetchUserContact(ctx context.Context, userID bson.ObjectID) (*models.User, error) {
	var user models.User
	filter := bson.M{"_id": userID}
	opts := options.FindOne().SetProjection(userContactProjection())
	if err := objects.DB.Collection(string(objects.UserColl)).FindOne(ctx, filter, opts).Decode(&user); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("%w: %w", ErrUserNotFound, err)
		}
		return nil, fmt.Errorf("failed to fetch user: %w", err)
	}
	return &user, nil
}

// checkContactRequest validates a contact request between two users.
func checkContactRequest(user *models.User, target bson.ObjectID) error {
	switch {
	case contains(user.ContactInfo.PendingOut, target):
		return fmt.Errorf("%w", ErrRequestAlreadySent)
	case contains(user.ContactInfo.PendingIn, target):
		return fmt.Errorf("%w", ErrRequestAlreadySent)
	case contains(user.ContactInfo.Favorites, target):
		return fmt.Errorf("%w", ErrAlreadyInFavorites)
	case contains(user.ContactInfo.Contacts, target):
		return fmt.Errorf("%w", ErrAlreadyInContacts)
	case contains(user.ContactInfo.BlockedChats, target):
		return fmt.Errorf("%w", ErrBlocked)
	}
	return nil
}

// SendContactRequest sends a contact request from userID to targetUserID.
func SendContactRequest(ctx context.Context, userID, targetUserID bson.ObjectID) error {
	user, err := fetchUserContact(ctx, userID)
	if err != nil {
		return err
	}

	targetUser, err := fetchUserContact(ctx, targetUserID)
	if err != nil {
		return err
	}

	if err := checkContactRequest(targetUser, userID); err != nil {
		return err
	}
	if err := checkContactRequest(user, targetUserID); err != nil {
		return err
	}

	now := time.Now()
	coll := objects.DB.Collection(string(objects.UserColl))

	if _, err := UpdateOne(ctx, coll, bson.M{"_id": userID}, bson.M{
		"$push": bson.M{"contactInfo.pendingOut": targetUserID},
		"$set":  bson.M{"contactInfo.updatedAt": now},
	}); err != nil {
		return fmt.Errorf("failed to update requesting user: %w", err)
	}

	if _, err := UpdateOne(ctx, coll, bson.M{"_id": targetUserID}, bson.M{
		"$push": bson.M{"contactInfo.pendingIn": userID},
		"$set":  bson.M{"contactInfo.updatedAt": now},
	}); err != nil {
		// Best-effort rollback: pull the pendingOut we just pushed
		UpdateOne(ctx, coll, bson.M{"_id": userID}, bson.M{
			"$pull": bson.M{"contactInfo.pendingOut": targetUserID},
		})
		return fmt.Errorf("failed to update target user: %w", err)
	}

	return nil
}

// AcceptOrDeclineContactRequest accepts or declines a pending contact request.
func AcceptOrDeclineContactRequest(ctx context.Context, userID, targetRequestID bson.ObjectID, action string) error {
	userFilter := bson.M{"_id": userID, "contactInfo.pendingIn": bson.M{"$in": []bson.ObjectID{targetRequestID}}}
	targetFilter := bson.M{"_id": targetRequestID, "contactInfo.pendingOut": bson.M{"$in": []bson.ObjectID{userID}}}

	coll := objects.DB.Collection(string(objects.UserColl))

	count, err := coll.CountDocuments(ctx, userFilter)
	if err != nil {
		return fmt.Errorf("failed to fetch user contact: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("%w", ErrContactRequestNotFound)
	}

	count, err = coll.CountDocuments(ctx, targetFilter)
	if err != nil {
		return fmt.Errorf("failed to fetch target contact: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("%w", ErrContactRequestNotFound)
	}

	now := time.Now()
	var userUpdate, targetUpdate bson.M

	switch objects.ContactStatus(action) {
	case objects.StatusAccepted:
		userUpdate = bson.M{
			"$pull": bson.M{"contactInfo.pendingIn": targetRequestID},
			"$push": bson.M{"contactInfo.contacts": targetRequestID},
			"$set":  bson.M{"contactInfo.updatedAt": now},
		}
		targetUpdate = bson.M{
			"$pull": bson.M{"contactInfo.pendingOut": userID},
			"$push": bson.M{"contactInfo.contacts": userID},
			"$set":  bson.M{"contactInfo.updatedAt": now},
		}
	case objects.StatusDeclined:
		userUpdate = bson.M{
			"$pull": bson.M{"contactInfo.pendingIn": targetRequestID},
		}
		targetUpdate = bson.M{
			"$pull": bson.M{"contactInfo.pendingOut": userID},
		}
	}

	if _, err := UpdateOne(ctx, coll, userFilter, userUpdate); err != nil {
		return fmt.Errorf("failed to update user contact: %w", err)
	}
	if _, err := UpdateOne(ctx, coll, targetFilter, targetUpdate); err != nil {
		return fmt.Errorf("failed to update target contact: %w", err)
	}

	return nil
}

// RemoveContact removes a contact relationship between two users.
func RemoveContact(ctx context.Context, userID, targetUserID bson.ObjectID) error {
	user, err := fetchUserContact(ctx, userID)
	if err != nil {
		return err
	}

	if !contains(user.ContactInfo.Contacts, targetUserID) {
		return fmt.Errorf("%w", ErrNotInContacts)
	}

	now := time.Now()
	coll := objects.DB.Collection(string(objects.UserColl))

	if _, err := UpdateOne(ctx, coll, bson.M{"_id": userID}, bson.M{
		"$pull": bson.M{
			"contactInfo.blockedChats": targetUserID,
			"contactInfo.favorites":    targetUserID,
			"contactInfo.contacts":     targetUserID,
		},
		"$set": bson.M{"contactInfo.updatedAt": now},
	}); err != nil {
		return fmt.Errorf("failed to update user contact: %w", err)
	}

	if _, err := UpdateOne(ctx, coll, bson.M{"_id": targetUserID}, bson.M{
		"$pull": bson.M{
			"contactInfo.blockedChats": userID,
			"contactInfo.favorites":    userID,
			"contactInfo.contacts":     userID,
		},
		"$set": bson.M{"contactInfo.updatedAt": now},
	}); err != nil {
		return fmt.Errorf("failed to update target contact: %w", err)
	}

	return nil
}

// BlockUnblockUser blocks or unblocks a user. Blocking removes the target
// from the caller's contacts/favourites/pending lists on both sides.
func BlockUnblockUser(ctx context.Context, userID, targetUserID bson.ObjectID, action string) error {
	if userID == targetUserID {
		return fmt.Errorf("%w", ErrSelfBlock)
	}

	// Verify the target exists.
	if _, err := fetchUserContact(ctx, targetUserID); err != nil {
		return err
	}

	now := time.Now()
	coll := objects.DB.Collection(string(objects.UserColl))

	switch objects.ContactStatus(action) {
	case objects.StatusBlocked:
		// Push into blockedChats and pull the target from every positive list
		// on both sides so the block completely severs the relationship.
		pullAll := bson.M{
			"contactInfo.contacts":   targetUserID,
			"contactInfo.favorites":  targetUserID,
			"contactInfo.pendingIn":  targetUserID,
			"contactInfo.pendingOut": targetUserID,
		}
		pullAllReverse := bson.M{
			"contactInfo.contacts":   userID,
			"contactInfo.favorites":  userID,
			"contactInfo.pendingIn":  userID,
			"contactInfo.pendingOut": userID,
		}

		if _, err := UpdateOne(ctx, coll, bson.M{"_id": userID}, bson.M{
			"$push": bson.M{"contactInfo.blockedChats": targetUserID},
			"$pull": pullAll,
			"$set":  bson.M{"contactInfo.updatedAt": now},
		}); err != nil {
			return fmt.Errorf("failed to block user: %w", err)
		}

		if _, err := UpdateOne(ctx, coll, bson.M{"_id": targetUserID}, bson.M{
			"$push": bson.M{"contactInfo.blockedChats": userID},
			"$pull": pullAllReverse,
			"$set":  bson.M{"contactInfo.updatedAt": now},
		}); err != nil {
			return fmt.Errorf("failed to update target user: %w", err)
		}

	case objects.StatusUnblocked:
		if _, err := UpdateOne(ctx, coll, bson.M{"_id": userID}, bson.M{
			"$pull": bson.M{"contactInfo.blockedChats": targetUserID},
			"$set":  bson.M{"contactInfo.updatedAt": now},
		}); err != nil {
			return fmt.Errorf("failed to unblock user: %w", err)
		}

		if _, err := UpdateOne(ctx, coll, bson.M{"_id": targetUserID}, bson.M{
			"$pull": bson.M{"contactInfo.blockedChats": userID},
			"$set":  bson.M{"contactInfo.updatedAt": now},
		}); err != nil {
			return fmt.Errorf("failed to update target user: %w", err)
		}

	default:
		return fmt.Errorf("unknown action: %s (use 'blocked' or 'unblocked')", action)
	}

	return nil
}

// AddToFavorites adds a user to the current user's favorites.
func AddToFavorites(ctx context.Context, userID, targetUserID bson.ObjectID) error {
	user, err := fetchUserContact(ctx, userID)
	if err != nil {
		return err
	}

	if !contains(user.ContactInfo.Contacts, targetUserID) {
		return fmt.Errorf("%w", ErrNotInContacts)
	}
	if contains(user.ContactInfo.BlockedChats, targetUserID) {
		return fmt.Errorf("%w", ErrBlocked)
	}
	if contains(user.ContactInfo.Favorites, targetUserID) {
		return fmt.Errorf("%w", ErrAlreadyInFavorites)
	}

	now := time.Now()
	_, err = UpdateOne(ctx, objects.DB.Collection(string(objects.UserColl)), bson.M{"_id": userID}, bson.M{
		"$push": bson.M{"contactInfo.favorites": targetUserID},
		"$set":  bson.M{"contactInfo.updatedAt": now},
	})
	if err != nil {
		return fmt.Errorf("failed to update user contact: %w", err)
	}

	return nil
}

// RemoveFromFavorites removes a user from the current user's favorites.
func RemoveFromFavorites(ctx context.Context, userID, targetUserID bson.ObjectID) error {
	user, err := fetchUserContact(ctx, userID)
	if err != nil {
		return err
	}

	if !contains(user.ContactInfo.Favorites, targetUserID) {
		return fmt.Errorf("%w", ErrNotInFavorites)
	}

	now := time.Now()
	_, err = UpdateOne(ctx, objects.DB.Collection(string(objects.UserColl)), bson.M{"_id": userID}, bson.M{
		"$pull": bson.M{"contactInfo.favorites": targetUserID},
		"$set":  bson.M{"contactInfo.updatedAt": now},
	})
	if err != nil {
		return fmt.Errorf("failed to update user contact: %w", err)
	}

	return nil
}
