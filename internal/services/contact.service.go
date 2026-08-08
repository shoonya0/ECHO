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
	objects.StatusPending: {
		projection: bson.M{"_id": 1, "updatedAt": 1, "contactInfo.pendingIn": 1, "contactInfo.pendingOut": 1},
		ids:        func(c models.ContactInfoEmbed) []bson.ObjectID { return append(c.PendingIn, c.PendingOut...) },
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

// SendContactRequest sends a contact request from userID to targetUserID.
func SendContactRequest(ctx context.Context, userID, targetUserID bson.ObjectID) error {
	sess, err := objects.DBClient.StartSession()
	if err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}
	defer sess.EndSession(ctx)

	_, err = sess.WithTransaction(ctx, func(sessCtx context.Context) (interface{}, error) {
		userFilter := bson.M{"_id": userID}
		targetFilter := bson.M{"_id": targetUserID}

		projection := bson.M{
			"_id":                      1,
			"contactInfo.blockedChats": 1,
			"contactInfo.pendingOut":   1,
			"contactInfo.pendingIn":    1,
			"contactInfo.favorites":    1,
			"contactInfo.contacts":     1,
		}

		var user, targetUser *models.User
		if err := objects.DB.Collection(string(objects.UserColl)).FindOne(sessCtx, userFilter, options.FindOne().SetProjection(projection)).Decode(&user); err != nil {
			if err == mongo.ErrNoDocuments {
				return nil, fmt.Errorf("requesting user not found")
			}
			return nil, fmt.Errorf("failed to fetch requesting user: %w", err)
		}

		if err := objects.DB.Collection(string(objects.UserColl)).FindOne(sessCtx, targetFilter, options.FindOne().SetProjection(projection)).Decode(&targetUser); err != nil {
			if err == mongo.ErrNoDocuments {
				return nil, fmt.Errorf("target user not found")
			}
			return nil, fmt.Errorf("failed to fetch target user: %w", err)
		}

		checkContactRequest := func(user *models.User, target bson.ObjectID) error {
			switch {
			case contains(user.ContactInfo.PendingOut, target):
				return fmt.Errorf("contact request already sent")
			case contains(user.ContactInfo.PendingIn, target):
				return fmt.Errorf("contact request already sent - accept it instead")
			case contains(user.ContactInfo.Favorites, target):
				return fmt.Errorf("user is already in your favorites")
			case contains(user.ContactInfo.Contacts, target):
				return fmt.Errorf("user is already in your contacts")
			case contains(user.ContactInfo.BlockedChats, target):
				return fmt.Errorf("user is blocked")
			}
			return nil
		}

		if err := checkContactRequest(targetUser, userID); err != nil {
			return nil, err
		}
		if err := checkContactRequest(user, targetUserID); err != nil {
			return nil, err
		}

		now := time.Now()
		userUpdate := bson.M{
			"$push": bson.M{"contactInfo.pendingOut": targetUserID},
			"$set":  bson.M{"contactInfo.updatedAt": now},
		}
		targetUpdate := bson.M{
			"$push": bson.M{"contactInfo.pendingIn": userID},
			"$set":  bson.M{"contactInfo.updatedAt": now},
		}

		if _, err := UpdateOne(sessCtx, objects.DB.Collection(string(objects.UserColl)), userFilter, userUpdate); err != nil {
			return nil, fmt.Errorf("failed to update requesting user contact info: %w", err)
		}
		if _, err := UpdateOne(sessCtx, objects.DB.Collection(string(objects.UserColl)), targetFilter, targetUpdate); err != nil {
			return nil, fmt.Errorf("failed to update target user contact info: %w", err)
		}

		return nil, nil
	})

	if err != nil {
		return fmt.Errorf("transaction failed: %w", err)
	}

	return nil
}

// AcceptOrDeclineContactRequest accepts or declines a pending contact request.
func AcceptOrDeclineContactRequest(ctx context.Context, userID, targetRequestID bson.ObjectID, action string) error {
	sess, err := objects.DBClient.StartSession()
	if err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}
	defer sess.EndSession(ctx)

	_, err = sess.WithTransaction(ctx, func(sessCtx context.Context) (interface{}, error) {
		userFilter := bson.M{"_id": userID, "contactInfo.pendingIn": bson.M{"$in": []bson.ObjectID{targetRequestID}}}
		targetFilter := bson.M{"_id": targetRequestID, "contactInfo.pendingOut": bson.M{"$in": []bson.ObjectID{userID}}}

		count, err := objects.DB.Collection(string(objects.UserColl)).CountDocuments(sessCtx, userFilter)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch user contact: %w", err)
		}
		if count == 0 {
			return nil, fmt.Errorf("contact request not found on user")
		}

		count, err = objects.DB.Collection(string(objects.UserColl)).CountDocuments(sessCtx, targetFilter)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch target contact: %w", err)
		}
		if count == 0 {
			return nil, fmt.Errorf("contact request not found on target")
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

		if _, err := UpdateOne(sessCtx, objects.DB.Collection(string(objects.UserColl)), userFilter, userUpdate); err != nil {
			return nil, fmt.Errorf("failed to update user contact: %w", err)
		}
		if _, err := UpdateOne(sessCtx, objects.DB.Collection(string(objects.UserColl)), targetFilter, targetUpdate); err != nil {
			return nil, fmt.Errorf("failed to update target contact: %w", err)
		}

		return nil, nil
	})

	if err != nil {
		return fmt.Errorf("transaction failed: %w", err)
	}

	return nil
}

// RemoveContact removes a contact relationship between two users.
func RemoveContact(ctx context.Context, userID, targetUserID bson.ObjectID) error {
	sess, err := objects.DBClient.StartSession()
	if err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}
	defer sess.EndSession(ctx)

	_, err = sess.WithTransaction(ctx, func(sessCtx context.Context) (interface{}, error) {
		userFilter := bson.M{"_id": userID, "contactInfo.contacts." + targetUserID.Hex(): bson.M{"$exists": true}}

		var userContact models.ContactRequest
		if err := objects.DB.Collection(string(objects.UserColl)).FindOne(sessCtx, userFilter, options.FindOne().SetProjection(bson.M{
			"_id":                  1,
			"contactInfo.contacts": bson.M{targetUserID.Hex(): 1},
		})).Decode(&userContact); err != nil {
			if err == mongo.ErrNoDocuments {
				return nil, fmt.Errorf("user not found")
			}
			return nil, fmt.Errorf("failed to fetch user contact: %w", err)
		}

		if findIndex(userContact.ContactInfo.Contacts, targetUserID) == -1 {
			return nil, fmt.Errorf("target is not present in user contacts")
		}

		now := time.Now()
		pull := bson.M{
			"contactInfo.blockedChats": targetUserID,
			"contactInfo.favorites":    targetUserID,
			"contactInfo.contacts":     targetUserID,
		}

		if _, err := UpdateOne(sessCtx, objects.DB.Collection(string(objects.UserColl)), userFilter, bson.M{
			"$pull": pull,
			"$set":  bson.M{"contactInfo.updatedAt": now},
		}); err != nil {
			return nil, fmt.Errorf("failed to update user contact: %w", err)
		}

		targetFilter := bson.M{"_id": targetUserID, "contactInfo.contacts." + userID.Hex(): bson.M{"$exists": true}}

		if _, err := UpdateOne(sessCtx, objects.DB.Collection(string(objects.UserColl)), targetFilter, bson.M{
			"$pull": bson.M{
				"contactInfo.blockedChats": userID,
				"contactInfo.favorites":    userID,
				"contactInfo.contacts":     userID,
			},
			"$set": bson.M{"contactInfo.updatedAt": now},
		}); err != nil {
			return nil, fmt.Errorf("failed to update target contact: %w", err)
		}

		return nil, nil
	})

	return err
}

// BlockUnblockUser blocks or unblocks a user.
func BlockUnblockUser(ctx context.Context, userID, targetUserID bson.ObjectID, action string) error {
	userFilter := bson.M{"_id": userID, "contactInfo.contacts." + targetUserID.Hex(): bson.M{"$exists": true}}

	var userContact models.ContactRequest
	err := objects.DB.Collection(string(objects.UserColl)).FindOne(ctx, userFilter, options.FindOne().SetProjection(bson.M{
		"_id":                      1,
		"contactInfo.blockedChats": bson.M{targetUserID.Hex(): 1},
		"contactInfo.contacts":     bson.M{targetUserID.Hex(): 1},
	})).Decode(&userContact)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("failed to fetch user contact: %w", err)
	}

	now := time.Now()
	var userUpdate bson.M

	switch objects.ContactStatus(action) {
	case objects.StatusBlocked:
		if contains(userContact.ContactInfo.BlockedChats, targetUserID) {
			return fmt.Errorf("user is already blocked")
		}
		userUpdate = bson.M{
			"$push": bson.M{"contactInfo.blockedChats": targetUserID},
			"$set":  bson.M{"contactInfo.updatedAt": now},
		}
	case objects.StatusAccepted:
		if !contains(userContact.ContactInfo.BlockedChats, targetUserID) {
			return fmt.Errorf("user is already unblocked")
		}
		userUpdate = bson.M{
			"$pull": bson.M{"contactInfo.blockedChats": targetUserID},
			"$set":  bson.M{"contactInfo.updatedAt": now},
		}
	default:
		return fmt.Errorf("unknown action: %s", action)
	}

	_, err = UpdateOne(ctx, objects.DB.Collection(string(objects.UserColl)), userFilter, userUpdate)
	if err != nil {
		return fmt.Errorf("failed to update user contact: %w", err)
	}

	return nil
}

// AddToFavorites adds a user to the current user's favorites.
func AddToFavorites(ctx context.Context, userID, targetUserID bson.ObjectID) error {
	userFilter := bson.M{"_id": userID, "contactInfo.contacts." + targetUserID.Hex(): bson.M{"$exists": true}}

	var userContact models.ContactRequest
	if err := objects.DB.Collection(string(objects.UserColl)).FindOne(ctx, userFilter, options.FindOne().SetProjection(bson.M{
		"_id":                      1,
		"contactInfo.favorites":    1,
		"contactInfo.blockedChats": 1,
		"contactInfo.contacts":     1,
	})).Decode(&userContact); err != nil {
		if err == mongo.ErrNoDocuments {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("failed to fetch user contact: %w", err)
	}

	if contains(userContact.ContactInfo.BlockedChats, targetUserID) {
		return fmt.Errorf("user is blocked")
	}
	if contains(userContact.ContactInfo.Favorites, targetUserID) {
		return fmt.Errorf("user is already in favorites")
	}

	now := time.Now()
	_, err := UpdateOne(ctx, objects.DB.Collection(string(objects.UserColl)), userFilter, bson.M{
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
	userFilter := bson.M{"_id": userID, "contactInfo.favorites." + targetUserID.Hex(): bson.M{"$exists": true}}

	var userContact models.ContactRequest
	if err := objects.DB.Collection(string(objects.UserColl)).FindOne(ctx, userFilter, options.FindOne().SetProjection(bson.M{
		"_id":                   1,
		"contactInfo.favorites": 1,
		"contactInfo.contacts":  1,
	})).Decode(&userContact); err != nil {
		if err == mongo.ErrNoDocuments {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("failed to fetch user contact: %w", err)
	}

	if !contains(userContact.ContactInfo.Favorites, targetUserID) {
		return fmt.Errorf("user is already not in favorites")
	}

	now := time.Now()
	_, err := UpdateOne(ctx, objects.DB.Collection(string(objects.UserColl)), userFilter, bson.M{
		"$pull": bson.M{"contactInfo.favorites": targetUserID},
		"$set":  bson.M{"contactInfo.updatedAt": now},
	})
	if err != nil {
		return fmt.Errorf("failed to update user contact: %w", err)
	}

	return nil
}
