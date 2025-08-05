package services

import (
	"context"
	"fmt"
	"gin/internal/models"
	"gin/objects"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// ============ CONTACTS & FRIENDS MANAGEMENT ============

// // this return array of users id with their name and avatar
func GetContacts(userID string, contactStatus objects.ContactStatus, limit int) ([]models.GetContactInfo, error) {
	contactRequests := []models.GetContactInfo{}

	objectID, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return []models.GetContactInfo{}, err
	}

	contactProjection := bson.M{
		"_id":       1,
		"updatedAt": 1,
	}

	switch contactStatus {
	case objects.StatusAccepted:
		contactProjection["contactInfo.activeChats"] = 1
		contactProjection["contactInfo.favorites"] = 1
	case objects.StatusPending:
		contactProjection["contactInfo.pendingIn"] = 1
		contactProjection["contactInfo.pendingOut"] = 1
	case objects.StatusFavorite:
		contactProjection["contactInfo.favorites"] = 1
	case objects.StatusBlocked:
		contactProjection["contactInfo.blockedUsers"] = 1
	case objects.StatusContact:
		contactProjection["contactInfo.activeChats"] = 1
		contactProjection["contactInfo.favorites"] = 1
		contactProjection["contactInfo.pendingIn"] = 1
		contactProjection["contactInfo.pendingOut"] = 1
		contactProjection["contactInfo.blockedUsers"] = 1
	}

	// first we have to get the information from the contact collection
	contact, err := FindByID[models.ContactRequest](context.Background(), objects.DB.Collection(string(objects.UserColl)), bson.M{"_id": objectID}, contactProjection)
	if err != nil {
		return []models.GetContactInfo{}, err
	}

	contactIds := contact.ContactInfo

	contacts := []bson.ObjectID{}
	switch contactStatus {
	case objects.StatusAccepted:
		contacts = append(contacts, contactIds.ActiveChats...)
		contacts = append(contacts, contactIds.Favorites...)
	case objects.StatusPending:
		contacts = append(contacts, contactIds.PendingIn...)
		contacts = append(contacts, contactIds.PendingOut...)
	case objects.StatusFavorite:
		contacts = append(contacts, contactIds.Favorites...)
	case objects.StatusBlocked:
		contacts = append(contacts, contactIds.BlockedUsers...)
	case objects.StatusContact:
		contacts = append(contacts, contactIds.BlockedUsers...)
		contacts = append(contacts, contactIds.PendingOut...)
		contacts = append(contacts, contactIds.PendingIn...)
		contacts = append(contacts, contactIds.Favorites...)
		contacts = append(contacts, contactIds.ActiveChats...)
	}

	userProjection := bson.M{
		"_id":                       1,
		"contactInfo.relationships": 1,
	}

	userContactsInfo, err := FindByID[models.User](context.Background(), objects.DB.Collection(string(objects.UserColl)), bson.M{"_id": bson.M{"$in": contacts}}, userProjection)
	if err != nil {
		return []models.GetContactInfo{}, err
	}

	for _, userRelationship := range userContactsInfo.ContactInfo.Relationships {
		contactRequests = append(contactRequests, models.GetContactInfo{
			ID:          userRelationship.TargetUserID,
			Status:      userRelationship.Status,
			Username:    userRelationship.UserInfo.Username,
			DisplayName: userRelationship.UserInfo.DisplayName,
			Avatar:      userRelationship.UserInfo.Avatar,
			IsFavorite:  userRelationship.IsFavorite,
			ChatID:      userRelationship.ChatID,
		})
	}

	fmt.Println("contactRequests", contacts)

	return contactRequests, nil
}

// Contact Actions
func SendContactRequest(userID, targetUserID string) (models.ContactRelationship, error) {
	// Convert string IDs to ObjectIDs
	userObjectID, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return models.ContactRelationship{}, fmt.Errorf("failed to convert user id to object id: %w", err)
	}

	targetObjectID, err := bson.ObjectIDFromHex(targetUserID)
	if err != nil {
		return models.ContactRelationship{}, fmt.Errorf("failed to convert target user id to object id: %w", err)
	}

	// Start session for atomic transaction
	sess, err := objects.DBClient.StartSession()
	if err != nil {
		return models.ContactRelationship{}, fmt.Errorf("failed to start session: %w", err)
	}
	defer sess.EndSession(context.Background())

	var userRelationship models.ContactRelationship

	// Execute all operations in a single transaction
	_, err = sess.WithTransaction(context.Background(), func(sessCtx context.Context) (interface{}, error) {
		// First, check if both users exist and validate the request
		userFilter := bson.M{"_id": userObjectID}
		targetFilter := bson.M{"_id": targetObjectID}

		projection := bson.M{
			"_id":                       1,
			"username":                  1,
			"profile.displayName":       1,
			"profile.avatar":            1,
			"contactInfo.pendingOut":    1,
			"contactInfo.pendingIn":     1,
			"contactInfo.relationships": 1,
		}

		// Check if users exist and validate contact request
		user, err := FindByID[models.User](sessCtx, objects.DB.Collection(string(objects.UserColl)), userFilter, projection)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				return nil, fmt.Errorf("requesting user not found")
			}
			return nil, fmt.Errorf("failed to fetch requesting user: %w", err)
		}

		targetUser, err := FindByID[models.User](sessCtx, objects.DB.Collection(string(objects.UserColl)), targetFilter, projection)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				return nil, fmt.Errorf("target user not found")
			}
			return nil, fmt.Errorf("failed to fetch target user: %w", err)
		}

		// Check if users are already connected or have pending requests
		if user.ContactInfo.Relationships != nil {
			if rel, exists := user.ContactInfo.Relationships[targetUserID]; exists {
				switch rel.Status {
				case string(objects.StatusAccepted):
					return nil, fmt.Errorf("users are already contacts")
				case string(objects.StatusPending):
					return nil, fmt.Errorf("contact request already sent")
				case string(objects.StatusBlocked):
					return nil, fmt.Errorf("cannot send request to blocked user")
				}
			}
		}

		// Check if target user already sent a request (can accept instead)
		if targetUser.ContactInfo.Relationships != nil {
			if rel, exists := targetUser.ContactInfo.Relationships[userID]; exists && rel.Status == string(objects.StatusPending) {
				return nil, fmt.Errorf("target user already sent you a request - accept it instead")
			}
		}

		// Create relationship data
		now := time.Now()

		// update the user's contact info
		userRelationship = models.ContactRelationship{
			TargetUserID: targetObjectID,
			Status:       string(objects.StatusPending),
			RequestedBy:  userObjectID,
			IsFavorite:   false,
			ChatID:       bson.NewObjectIDFromTimestamp(now),
			UserInfo: models.ContactUserInfo{
				Username:    targetUser.Username,
				DisplayName: targetUser.Profile.DisplayName,
				Avatar:      targetUser.Profile.Avatar,
			},
			CreatedAt: now,
		}

		// Update requesting user's contact info
		userUpdate := bson.M{
			"$addToSet": bson.M{
				"contactInfo.pendingOut": targetObjectID,
			},
			"$set": bson.M{
				"contactInfo.relationships." + targetUserID: userRelationship,
				"contactInfo.updatedAt":                     now,
			},
			"$inc": bson.M{
				"contactInfo.stats.pendingOutCount": 1,
			},
		}

		_, err = UpdateOne(sessCtx, objects.DB.Collection(string(objects.UserColl)), userFilter, userUpdate)
		if err != nil {
			return nil, fmt.Errorf("failed to update requesting user contact info: %w", err)
		}

		// update the target user's contact info
		targetRelationship := models.ContactRelationship{
			TargetUserID: userObjectID,
			Status:       string(objects.StatusPending),
			RequestedBy:  userObjectID,
			IsFavorite:   false,
			ChatID:       bson.NewObjectIDFromTimestamp(now),
			UserInfo: models.ContactUserInfo{
				Username:    user.Username,
				DisplayName: user.Profile.DisplayName,
				Avatar:      user.Profile.Avatar,
			},
			CreatedAt: now,
		}

		// Update target user's contact info
		targetUpdate := bson.M{
			"$addToSet": bson.M{
				"contactInfo.pendingIn": userObjectID,
			},
			"$set": bson.M{
				"contactInfo.relationships." + userID: targetRelationship,
				"contactInfo.updatedAt":               now,
			},
			"$inc": bson.M{
				"contactInfo.stats.pendingInCount": 1,
			},
		}

		_, err = UpdateOne(sessCtx, objects.DB.Collection(string(objects.UserColl)), targetFilter, targetUpdate)
		if err != nil {
			return nil, fmt.Errorf("failed to update target user contact info: %w", err)
		}

		return nil, nil
	})

	if err != nil {
		return models.ContactRelationship{}, fmt.Errorf("transaction failed: %w", err)
	}

	return userRelationship, nil
}

func AcceptOrDeclineContactRequest(userID, targetUserID, action string) (models.ContactRelationship, error) {
	// get the contact request from the database
	userObjectID, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return models.ContactRelationship{}, fmt.Errorf("failed to convert user id to object id: %w", err)
	}

	targetObjectID, err := bson.ObjectIDFromHex(targetUserID)
	if err != nil {
		return models.ContactRelationship{}, fmt.Errorf("failed to convert target user id to object id: %w", err)
	}

	sess, err := objects.DBClient.StartSession()
	if err != nil {
		return models.ContactRelationship{}, fmt.Errorf("failed to start session: %w", err)
	}
	defer sess.EndSession(context.Background())

	var userRelationship models.ContactRelationship

	_, err = sess.WithTransaction(context.Background(), func(sessCtx context.Context) (interface{}, error) {
		// first we have to get the contact request from the database
		userFilter := bson.M{"_id": userObjectID, "contactInfo.relationships." + targetObjectID.Hex(): bson.M{"$exists": true}}
		targetFilter := bson.M{"_id": targetObjectID, "contactInfo.relationships." + userObjectID.Hex(): bson.M{"$exists": true}}

		contactProjection := bson.M{
			"_id":                               1,
			"contactInfo.relationships":         1,
			"contactInfo.pendingIn":             1,
			"contactInfo.pendingOut":            1,
			"contactInfo.updatedAt":             1,
			"contactInfo.stats.pendingInCount":  1,
			"contactInfo.stats.pendingOutCount": 1,
			"contactInfo.stats.totalContacts":   1,
		}

		userContact, err := FindByID[models.ContactRequest](sessCtx, objects.DB.Collection(string(objects.UserColl)), userFilter, contactProjection)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				return nil, fmt.Errorf("contact request not found")
			}
			return nil, fmt.Errorf("failed to fetch user contact: %w", err)
		}

		targetContact, err := FindByID[models.ContactRequest](sessCtx, objects.DB.Collection(string(objects.UserColl)), targetFilter, contactProjection)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				return nil, fmt.Errorf("contact request not found")
			}
			return nil, fmt.Errorf("failed to fetch target contact: %w", err)
		}

		if userContact.ContactInfo.Relationships[targetObjectID.Hex()].Status == string(objects.StatusAccepted) || userContact.ContactInfo.Relationships[targetObjectID.Hex()].Status == string(objects.StatusDeclined) {
			return nil, fmt.Errorf("contact request already %s please try again", userContact.ContactInfo.Relationships[targetObjectID.Hex()].Status)
		}

		now := time.Now()

		switch objects.ContactStatus(action) {
		case objects.StatusAccepted:
			// update the relationship status
			targetKey := targetObjectID.Hex()
			userKey := userObjectID.Hex()

			// Dereference and update relationships
			userRel := userContact.ContactInfo.Relationships[targetKey]
			userRel.Status = string(objects.StatusAccepted)
			userRel.AcceptedAt = &now
			userContact.ContactInfo.Relationships[targetKey] = userRel

			targetRel := targetContact.ContactInfo.Relationships[userKey]
			targetRel.Status = string(objects.StatusAccepted)
			targetRel.AcceptedAt = &now
			targetContact.ContactInfo.Relationships[userKey] = targetRel

			// update the stats
			userContact.ContactInfo.PendingIn = *removeElement(&userContact.ContactInfo.PendingIn, targetObjectID)
			newUserChats := append(userContact.ContactInfo.ActiveChats, targetObjectID)
			userContact.ContactInfo.ActiveChats = newUserChats

			targetContact.ContactInfo.PendingOut = *removeElement(&targetContact.ContactInfo.PendingOut, userObjectID)
			newTargetChats := append(targetContact.ContactInfo.ActiveChats, userObjectID)
			targetContact.ContactInfo.ActiveChats = newTargetChats

			userRelationship = userRel

		case objects.StatusDeclined:
			// update the relationship status
			targetKey := targetObjectID.Hex()
			userKey := userObjectID.Hex()

			userRel := userContact.ContactInfo.Relationships[targetKey]
			userRel.Status = string(objects.StatusDeclined)
			userContact.ContactInfo.Relationships[targetKey] = userRel

			targetRel := targetContact.ContactInfo.Relationships[userKey]
			targetRel.Status = string(objects.StatusDeclined)
			targetContact.ContactInfo.Relationships[userKey] = targetRel

			return nil, nil
		}

		userUpdate := bson.M{
			"$set": bson.M{
				"contactInfo.relationships." + targetObjectID.Hex(): (userContact.ContactInfo.Relationships)[targetObjectID.Hex()],
				"contactInfo.pendingIn":                             userContact.ContactInfo.PendingIn,
				"contactInfo.activeChats":                           userContact.ContactInfo.ActiveChats,
				"contactInfo.updatedAt":                             now,
			},
			"$inc": bson.M{
				"contactInfo.stats.pendingInCount": -1,
				"contactInfo.stats.totalContacts":  +1,
			},
		}

		_, err = UpdateOne(sessCtx, objects.DB.Collection(string(objects.UserColl)), userFilter, userUpdate)
		if err != nil {
			return nil, fmt.Errorf("failed to update user contact: %w", err)
		}

		// update the target contact
		targetUpdate := bson.M{
			"$set": bson.M{
				"contactInfo.relationships." + userObjectID.Hex(): (targetContact.ContactInfo.Relationships)[userObjectID.Hex()],
				"contactInfo.pendingOut":                          targetContact.ContactInfo.PendingOut,
				"contactInfo.activeChats":                         targetContact.ContactInfo.ActiveChats,
				"contactInfo.updatedAt":                           now,
			},
			"$inc": bson.M{
				"contactInfo.stats.pendingOutCount": -1,
				"contactInfo.stats.totalContacts":   +1,
			},
		}

		_, err = UpdateOne(sessCtx, objects.DB.Collection(string(objects.UserColl)), targetFilter, targetUpdate)
		if err != nil {
			return nil, fmt.Errorf("failed to update target contact: %w", err)
		}

		return nil, nil
	})

	if err != nil {
		return models.ContactRelationship{}, fmt.Errorf("transaction failed: %w", err)
	}

	return userRelationship, nil
}

// func RemoveContact(userID, targetUserID string) (string, error) {
// 	// userObjectID, err := bson.ObjectIDFromHex(userID)
// 	// if err != nil {
// 	// 	return "", fmt.Errorf("failed to convert user id to object id: %w", err)
// 	// }

// 	// // search in contact collection for the contactId
// 	// contact := models.Contact{}
// 	// if err := objects.DBClient.Database("ECHO").Collection("contacts").FindOne(c.Request.Context(), bson.M{"_id": contactId}).Decode(&contact); err != nil {
// 	// 	return "", fmt.Errorf("failed to fetch contact: %w", err)
// 	// }

// 	// remove the chatId from the contact
// 	// for _, contactStatus := range contact.Contacts {
// 	// if contactStatus.ChatId == chatId {
// 	// 	delete(contact.Contacts, contactStatus)
// 	// 	// update the contact in the database
// 	// 	if _, err := objects.DBClient.Database("ECHO").Collection("contacts").UpdateOne(c.Request.Context(), bson.M{"_id": contactId}, bson.M{"$set": contact}); err != nil {
// 	// 		return "", fmt.Errorf("failed to update contact: %w", err)
// 	// 	}

// 	// 	// remove the chat from the chat collection
// 	// 	if _, err := objects.DBClient.Database("ECHO").Collection("chats").DeleteOne(c.Request.Context(), bson.M{"_id": chatId}); err != nil {
// 	// 		return "", fmt.Errorf("failed to delete chat: %w", err)
// 	// 	}
// 	// 	return "contact removed", nil
// 	// }
// 	// }

// 	return "contact removed", nil
// }

// func BlockUser(c *gin.Context) (string, error) {
// 	// userID, ok := c.Get("user_id")
// 	// if !ok {
// 	// 	return "", fmt.Errorf("user id not found")
// 	// }

// 	// targetUserId := c.Param("userId")

// 	// // get the contact of the user from the database
// 	// contact := models.Contact{}
// 	// if err := objects.DBClient.Database("ECHO").Collection("contacts").FindOne(c.Request.Context(), bson.M{"_id": userID.(string)}).Decode(&contact); err != nil {
// 	// 	return "", fmt.Errorf("failed to fetch contact: %w", err)
// 	// }

// 	// // update the contact status to blocked
// 	// contact.Contacts[targetUserId] = models.ContactRequest{
// 	// 	RequestedBy: userID.(string),
// 	// 	Status:      "blocked",
// 	// 	ChatID:      contact.Contacts[targetUserId].ChatID,
// 	// }

// 	// // update the contact in the database
// 	// if _, err := objects.DBClient.Database("ECHO").Collection("contacts").UpdateOne(c.Request.Context(), bson.M{"user_id": userID.(string)}, bson.M{"$set": contact}); err != nil {
// 	// 	return "", fmt.Errorf("failed to update contact: %w", err)
// 	// }

// 	return "user blocked", nil
// }

// // func UnblockUser(c *gin.Context) (string, error) {
// // 	userID, ok := c.Get("user_id")
// // 	if !ok {
// // 		return "", fmt.Errorf("user id not found")
// // 	}

// // 	targetUserId := c.Param("userId")

// // 	// get the contact of the user from the database
// // 	contact := models.Contact{}
// // 	if err := objects.DBClient.Database("ECHO").Collection("contacts").FindOne(c.Request.Context(), bson.M{"_id": userID.(string)}).Decode(&contact); err != nil {
// // 		return "", fmt.Errorf("failed to fetch contact: %w", err)
// // 	}

// // 	// update the contact status to unblocked
// // 	contact.Contacts[targetUserId] = models.ContactRequest{
// // 		RequestedBy: userID.(string),
// // 		Status:      "accepted",
// // 		ChatID:      contact.Contacts[targetUserId].ChatID,
// // 	}

// // 	// update the contact in the database
// // 	if _, err := objects.DBClient.Database("ECHO").Collection("contacts").UpdateOne(c.Request.Context(), bson.M{"_id": userID.(string)}, bson.M{"$set": contact}); err != nil {
// // 		return "", fmt.Errorf("failed to update contact: %w", err)
// // 	}

// // 	return "user unblocked", nil
// // }

// // func AddToFavorites(c *gin.Context) (string, error) {
// // 	userID, ok := c.Get("user_id")
// // 	if !ok {
// // 		return "", fmt.Errorf("user id not found")
// // 	}

// // 	targetUserId := c.Param("userId")

// // 	// get the contact of the user from the database
// // 	contact := models.Contact{}
// // 	if err := objects.DBClient.Database("ECHO").Collection("contacts").FindOne(c.Request.Context(), bson.M{"_id": userID.(string)}).Decode(&contact); err != nil {
// // 		return "", fmt.Errorf("failed to fetch contact: %w", err)
// // 	}

// // 	// update the contact status to favorite
// // 	contact.Contacts[targetUserId] = models.ContactRequest{
// // 		RequestedBy: userID.(string),
// // 		Status:      "favorite",
// // 		ChatID:      contact.Contacts[targetUserId].ChatID,
// // 	}

// // 	// update the contact in the database
// // 	if _, err := objects.DBClient.Database("ECHO").Collection("contacts").UpdateOne(c.Request.Context(), bson.M{"_id": userID.(string)}, bson.M{"$set": contact}); err != nil {
// // 		return "", fmt.Errorf("failed to update contact: %w", err)
// // 	}

// // 	return "user added to favorites", nil
// // }

// // func RemoveFromFavorites(c *gin.Context) (string, error) {
// // 	userID, ok := c.Get("user_id")
// // 	if !ok {
// // 		return "", fmt.Errorf("user id not found")
// // 	}
// // 	targetUserId := c.Param("userId")

// // 	// get the contact of the user from the database
// // 	contact := models.Contact{}
// // 	if err := objects.DBClient.Database("ECHO").Collection("contacts").FindOne(c.Request.Context(), bson.M{"_id": userID.(string)}).Decode(&contact); err != nil {
// // 		return "", fmt.Errorf("failed to fetch contact: %w", err)
// // 	}

// // 	// update the contact status to not favorite
// // 	contact.Contacts[targetUserId] = models.ContactRequest{
// // 		RequestedBy: userID.(string),
// // 		Status:      "accepted",
// // 		ChatID:      contact.Contacts[targetUserId].ChatID,
// // 	}

// // 	// update the contact in the database
// // 	if _, err := objects.DBClient.Database("ECHO").Collection("contacts").UpdateOne(c.Request.Context(), bson.M{"_id": userID.(string)}, bson.M{"$set": contact}); err != nil {
// // 		return "", fmt.Errorf("failed to update contact: %w", err)
// // 	}

// // 	return "user removed from favorites", nil
// // }

// // func GetFavoriteContacts(c *gin.Context) ([]models.Chat, error) {
// // 	userID, ok := c.Get("user_id")
// // 	if !ok {
// // 		return nil, fmt.Errorf("user id not found")
// // 	}

// // 	// get the contact of the user from the database
// // 	contact := models.Contact{}
// // 	if err := objects.DBClient.Database("ECHO").Collection("contacts").FindOne(c.Request.Context(), bson.M{"_id": userID.(string)}).Decode(&contact); err != nil {
// // 		return nil, fmt.Errorf("failed to fetch contact: %w", err)
// // 	}

// // 	favoriteChats := []models.Chat{}

// // 	// get the favorite contacts
// // 	for _, contactStatus := range contact.Contacts {
// // 		if contactStatus.Status == "favorite" {
// // 			chat := models.Chat{}

// // 			// in this case we have to get the chat info from the chat collection
// // 			if err := objects.DBClient.Database("ECHO").Collection("chats").FindOne(c.Request.Context(), bson.M{"_id": contactStatus.ChatID}).Decode(&chat); err != nil {
// // 				return nil, fmt.Errorf("failed to fetch chat: %w", err)
// // 			}
// // 			favoriteChats = append(favoriteChats, chat)
// // 		}
// // 	}

// // 	return favoriteChats, nil
// // }
