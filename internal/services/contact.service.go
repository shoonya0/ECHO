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

// ============ CONTACTS & FRIENDS MANAGEMENT ============
func GetContacts(ctx context.Context, userID bson.ObjectID, contactStatus objects.ContactStatus, limit int) ([]models.ContactInfo, error) {
	contactRequests := []models.ContactInfo{}

	contactProjection := bson.M{
		"_id":       1,
		"updatedAt": 1,
	}

	switch contactStatus {
	case objects.StatusAccepted:
		contactProjection["contactInfo.contacts"] = 1
		contactProjection["contactInfo.favorites"] = 1
	case objects.StatusPending:
		contactProjection["contactInfo.pendingIn"] = 1
		contactProjection["contactInfo.pendingOut"] = 1
	case objects.StatusFavorite:
		contactProjection["contactInfo.favorites"] = 1
	case objects.StatusBlocked:
		contactProjection["contactInfo.blockedChats"] = 1
	case objects.StatusContact:
		contactProjection["contactInfo.contacts"] = 1
		contactProjection["contactInfo.favorites"] = 1
		contactProjection["contactInfo.pendingIn"] = 1
		contactProjection["contactInfo.pendingOut"] = 1
		contactProjection["contactInfo.blockedChats"] = 1
	}

	contact := models.ContactRequest{}
	err := objects.DB.Collection(string(objects.UserColl)).FindOne(context.Background(), bson.M{"_id": userID}, options.FindOne().SetProjection(contactProjection)).Decode(&contact)
	if err != nil {
		return []models.ContactInfo{}, err
	}

	contactIds := contact.ContactInfo
	contacts := make([]bson.ObjectID, 0)

	switch contactStatus {
	case objects.StatusAccepted:
		contacts = append(contacts, contactIds.Contacts...)
	case objects.StatusPending:
		contacts = append(contacts, contactIds.PendingIn...)
		contacts = append(contacts, contactIds.PendingOut...)
	case objects.StatusFavorite:
		contacts = append(contacts, contactIds.Favorites...)
	case objects.StatusBlocked:
		contacts = append(contacts, contactIds.BlockedChats...)
	case objects.StatusContact:
		contacts = append(contacts, contactIds.BlockedChats...)
		contacts = append(contacts, contactIds.PendingOut...)
		contacts = append(contacts, contactIds.PendingIn...)
		contacts = append(contacts, contactIds.Favorites...)
		contacts = append(contacts, contactIds.Contacts...)
	}

	usersProjection := bson.M{
		"_id":                   1,
		"profile":               1,
		"username":              1,
		"presence.status":       1,
		"presence.isOnline":     1,
		"presence.lastActivity": 1,
		"presence.lastSeen":     1,
	}

	cursor, err := objects.DB.Collection(string(objects.UserColl)).Find(context.Background(), bson.M{"_id": bson.M{"$in": contacts}}, options.Find().SetProjection(usersProjection))
	if err != nil {
		return []models.ContactInfo{}, err
	}

	for cursor.Next(context.Background()) {
		var user models.ContactInfo
		if err := cursor.Decode(&user); err != nil {
			return []models.ContactInfo{}, err
		}

		if user.Presence.Status != string(objects.UserStatusOnline) {
			user.Presence.IsOnline = false
			if user.Presence.Status != string(objects.UserStatusOffline) {
				user.Presence.LastActivity = time.Time{}
				user.Presence.LastSeen = time.Time{}
			}
		}

		if contains(contactIds.Favorites, user.ID) {
			user.IsFavorite = true
		} else {
			user.IsFavorite = false
		}

		contactRequests = append(contactRequests, user)
	}

	return contactRequests, nil
}

// ============= Contact Actions =============
func SendContactRequest(ctx context.Context, userID bson.ObjectID, targetUserID bson.ObjectID) error {
	sess, err := objects.DBClient.StartSession()
	if err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}
	defer sess.EndSession(ctx)

	_, err = sess.WithTransaction(ctx, func(sessCtx context.Context) (interface{}, error) {
		userFilter := bson.M{"_id": userID}
		targetFilter := bson.M{"_id": targetUserID}

		Projection := bson.M{
			"_id":                      1,
			"contactInfo.blockedChats": 1,
			"contactInfo.pendingOut":   1,
			"contactInfo.pendingIn":    1,
			"contactInfo.favorites":    1,
			"contactInfo.contacts":     1,
		}

		var user, targetUser *models.User
		err = objects.DB.Collection(string(objects.UserColl)).FindOne(sessCtx, userFilter, options.FindOne().SetProjection(Projection)).Decode(&user)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				return nil, fmt.Errorf("requesting user not found")
			}
			return nil, fmt.Errorf("failed to fetch requesting user: %w", err)
		}

		err = objects.DB.Collection(string(objects.UserColl)).FindOne(sessCtx, targetFilter, options.FindOne().SetProjection(Projection)).Decode(&targetUser)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				return nil, fmt.Errorf("target user not found")
			}
			return nil, fmt.Errorf("failed to fetch target user: %w", err)
		}

		checkContactRequest := func(user *models.User, targetUserID bson.ObjectID) error {
			if contains(user.ContactInfo.PendingOut, targetUserID) {
				return fmt.Errorf("contact request already sent")
			}

			if contains(user.ContactInfo.PendingIn, targetUserID) {
				return fmt.Errorf("contact request already sent - accept it instead")
			}

			if contains(user.ContactInfo.Favorites, targetUserID) {
				return fmt.Errorf("user is already in your favorites")
			}

			if contains(user.ContactInfo.Contacts, targetUserID) {
				return fmt.Errorf("user is already in your contacts")
			}

			if contains(user.ContactInfo.BlockedChats, targetUserID) {
				return fmt.Errorf("user is blocked")
			}

			return nil
		}

		if err := checkContactRequest(user, targetUserID); err != nil {
			return nil, err
		}

		if err := checkContactRequest(targetUser, userID); err != nil {
			return nil, err
		}

		now := time.Now()

		userUpdateProjection := bson.M{
			"$push": bson.M{
				"contactInfo.pendingOut": targetUserID,
			},
			"$set": bson.M{
				"contactInfo.updatedAt": now,
			},
		}

		_, err = UpdateOne(sessCtx, objects.DB.Collection(string(objects.UserColl)), userFilter, userUpdateProjection)
		if err != nil {
			return models.GetUserProfileResponse{}, fmt.Errorf("failed to update requesting user contact info: %w", err)
		}

		targetUpdateProjection := bson.M{
			"$push": bson.M{
				"contactInfo.pendingIn": userID,
			},
			"$set": bson.M{
				"contactInfo.updatedAt": now,
			},
		}

		_, err = UpdateOne(sessCtx, objects.DB.Collection(string(objects.UserColl)), targetFilter, targetUpdateProjection)
		if err != nil {
			return models.GetUserProfileResponse{}, fmt.Errorf("failed to update target user contact info: %w", err)
		}

		return nil, nil
	})

	if err != nil {
		return fmt.Errorf("transaction failed: %w", err)
	}

	return nil
}

func AcceptOrDeclineContactRequest(ctx context.Context, userID, targetRequestID bson.ObjectID, action string) error {
	sess, err := objects.DBClient.StartSession()
	if err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}
	defer sess.EndSession(context.Background())

	_, err = sess.WithTransaction(context.Background(), func(sessCtx context.Context) (interface{}, error) {
		userFilter := bson.M{"_id": userID, "contactInfo.pendingIn": bson.M{"$in": []bson.ObjectID{targetRequestID}}}
		targetFilter := bson.M{"_id": targetRequestID, "contactInfo.pendingOut": bson.M{"$in": []bson.ObjectID{userID}}}

		count, err := objects.DB.Collection(string(objects.UserColl)).CountDocuments(context.Background(), userFilter)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch user contact: %w", err)
		}
		if count == 0 {
			return nil, fmt.Errorf("contact request not found on user")
		}

		count, err = objects.DB.Collection(string(objects.UserColl)).CountDocuments(context.Background(), targetFilter)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch target contact: %w", err)
		}
		if count == 0 {
			return nil, fmt.Errorf("contact request not found on target")
		}

		now := time.Now()

		userUpdate, targetUpdate := bson.M{}, bson.M{}

		switch objects.ContactStatus(action) {
		case objects.StatusAccepted:
			userUpdate = bson.M{
				"$pull": bson.M{
					"contactInfo.pendingIn": targetRequestID,
				},
				"$push": bson.M{
					"contactInfo.contacts": targetRequestID,
				},
				"$set": bson.M{
					"contactInfo.updatedAt": now,
				},
			}

			targetUpdate = bson.M{
				"$pull": bson.M{
					"contactInfo.pendingOut": userID,
				},
				"$push": bson.M{
					"contactInfo.contacts": userID,
				},
				"$set": bson.M{
					"contactInfo.updatedAt": now,
				},
			}

		case objects.StatusDeclined:
			userUpdate = bson.M{
				"$pull": bson.M{
					"contactInfo.pendingIn": targetRequestID,
				},
			}

			targetUpdate = bson.M{
				"$pull": bson.M{
					"contactInfo.pendingOut": userID,
				},
			}
		}

		_, err = UpdateOne(sessCtx, objects.DB.Collection(string(objects.UserColl)), userFilter, userUpdate)
		if err != nil {
			return nil, fmt.Errorf("failed to update user contact: %w", err)
		}

		_, err = UpdateOne(sessCtx, objects.DB.Collection(string(objects.UserColl)), targetFilter, targetUpdate)
		if err != nil {
			return nil, fmt.Errorf("failed to update target contact: %w", err)
		}

		return nil, nil
	})

	if err != nil {
		return fmt.Errorf("transaction failed: %w", err)
	}

	return nil
}

func RemoveContact(ctx context.Context, userID, targetUserID bson.ObjectID) error {
	sess, err := objects.DBClient.StartSession()
	if err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}
	defer sess.EndSession(ctx)

	_, err = sess.WithTransaction(ctx, func(sessCtx context.Context) (interface{}, error) {
		userFilter := bson.M{"_id": userID, "contactInfo.contacts." + targetUserID.Hex(): bson.M{"$exists": true}}

		var userContact models.ContactRequest
		err = objects.DB.Collection(string(objects.UserColl)).FindOne(sessCtx, userFilter, options.FindOne().SetProjection(bson.M{
			"_id":                  1,
			"contactInfo.contacts": bson.M{targetUserID.Hex(): 1},
		})).Decode(&userContact)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				return nil, fmt.Errorf("user not found")
			}
			return nil, fmt.Errorf("failed to fetch user contact: %w", err)
		}

		targetId := findIndex(userContact.ContactInfo.Contacts, targetUserID)

		if targetId == -1 {
			return nil, fmt.Errorf("target is not present in user contacts")
		}

		now := time.Now()

		// update user collection
		userUpdate := bson.M{
			// to remove the target user from the user contact which is an array of object ids
			"$pull": bson.M{
				"contactInfo.blockedChats": targetUserID,
				"contactInfo.favorites":    targetUserID,
				"contactInfo.contacts":     targetUserID,
			},
			"$set": bson.M{
				"contactInfo.updatedAt": now,
			},
		}

		_, err = UpdateOne(sessCtx, objects.DB.Collection(string(objects.UserColl)), userFilter, userUpdate)
		if err != nil {
			return nil, fmt.Errorf("failed to update user contact: %w", err)
		}

		targetFilter := bson.M{"_id": targetUserID, "contactInfo.contacts." + userID.Hex(): bson.M{"$exists": true}}

		targetUpdate := bson.M{
			"$pull": bson.M{
				"contactInfo.blockedChats": userID,
				"contactInfo.favorites":    userID,
				"contactInfo.contacts":     userID,
			},
			"$set": bson.M{
				"contactInfo.updatedAt": now,
			},
		}

		_, err = UpdateOne(sessCtx, objects.DB.Collection(string(objects.UserColl)), targetFilter, targetUpdate)
		if err != nil {
			return nil, fmt.Errorf("failed to update target contact: %w", err)
		}

		return nil, nil
	})

	return err
}

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
	userUpdate := bson.M{}

	switch objects.ContactStatus(action) {
	case objects.StatusBlocked:
		if contains(userContact.ContactInfo.BlockedChats, targetUserID) {
			return fmt.Errorf("user is already blocked")
		}
		userUpdate = bson.M{
			"$push": bson.M{
				"contactInfo.blockedChats": targetUserID,
			},
			"$set": bson.M{
				"contactInfo.updatedAt": now,
			},
		}
	case objects.StatusAccepted:
		if !contains(userContact.ContactInfo.BlockedChats, targetUserID) {
			return fmt.Errorf("user is already unblocked")
		}
		userUpdate = bson.M{
			"$pull": bson.M{
				"contactInfo.blockedChats": targetUserID,
			},
			"$set": bson.M{
				"contactInfo.updatedAt": now,
			},
		}
	}

	_, err = UpdateOne(ctx, objects.DB.Collection(string(objects.UserColl)), userFilter, userUpdate)
	if err != nil {
		return fmt.Errorf("failed to update user contact: %w", err)
	}

	return err
}

func AddToFavorites(ctx context.Context, userID, targetUserID bson.ObjectID) error {
	userFilter := bson.M{"_id": userID, "contactInfo.contacts." + targetUserID.Hex(): bson.M{"$exists": true}}

	var userContact models.ContactRequest
	err := objects.DB.Collection(string(objects.UserColl)).FindOne(ctx, userFilter, options.FindOne().SetProjection(bson.M{
		"_id":                      1,
		"contactInfo.favorites":    1,
		"contactInfo.blockedChats": 1,
		"contactInfo.contacts":     1,
	})).Decode(&userContact)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("failed to fetch user contact: %w", err)
	}

	if contains(userContact.ContactInfo.BlockedChats, targetUserID) {
		return fmt.Errorf("user is blocked")
	}

	if contains(userContact.ContactInfo.Favorites, targetUserID) {
		return fmt.Errorf("user is already in favorites contacts")
	}

	now := time.Now()

	updateProjection := bson.M{
		"$push": bson.M{
			"contactInfo.favorites": targetUserID,
		},
		"$set": bson.M{
			"contactInfo.updatedAt": now,
		},
	}

	_, err = UpdateOne(ctx, objects.DB.Collection(string(objects.UserColl)), userFilter, updateProjection)
	if err != nil {
		return fmt.Errorf("failed to update user contact: %w", err)
	}

	return nil
}

func RemoveFromFavorites(ctx context.Context, userID, targetUserID bson.ObjectID) error {
	userFilter := bson.M{"_id": userID, "contactInfo.favorites." + targetUserID.Hex(): bson.M{"$exists": true}}

	var userContact models.ContactRequest
	err := objects.DB.Collection(string(objects.UserColl)).FindOne(ctx, userFilter, options.FindOne().SetProjection(bson.M{
		"_id":                   1,
		"contactInfo.favorites": 1,
		"contactInfo.contacts":  1,
	})).Decode(&userContact)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("failed to fetch user contact: %w", err)
	}

	if !contains(userContact.ContactInfo.Favorites, targetUserID) {
		return fmt.Errorf("user is already not in favorites contacts")
	}

	now := time.Now()

	updateProjection := bson.M{
		"$pull": bson.M{
			"contactInfo.favorites": targetUserID,
		},
		"$set": bson.M{
			"contactInfo.updatedAt": now,
		},
	}

	_, err = UpdateOne(ctx, objects.DB.Collection(string(objects.UserColl)), userFilter, updateProjection)
	if err != nil {
		return fmt.Errorf("failed to update user contact: %w", err)
	}
	return nil
}
