package services

import (
	"context"
	"fmt"
	"gin/internal/models"
	"gin/internal/utils"
	"gin/objects"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// ============ CONTACTS & FRIENDS MANAGEMENT ============

// // this return array of users id with their name and avatar
func GetContacts(userID string, contactStatus objects.ContactStatus, limit int) ([]models.ContactInfo, error) {
	contactRequests := []models.ContactInfo{}

	objectID, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return []models.ContactInfo{}, err
	}

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

	// first we have to get the information from the contact collection
	contact, err := FindByID[models.ContactRequest](context.Background(), objects.DB.Collection(string(objects.UserColl)), bson.M{"_id": objectID}, contactProjection)
	if err != nil {
		return []models.ContactInfo{}, err
	}

	contactIds := contact.ContactInfo

	addToMap := func(contacts []bson.ObjectID, map2 map[bson.ObjectID]bson.ObjectID) []bson.ObjectID {
		for _, v := range map2 {
			contacts = append(contacts, v)
		}
		return contacts
	}

	contacts := make([]bson.ObjectID, 0)
	switch contactStatus {
	case objects.StatusAccepted:
		contacts = addToMap(contacts, contactIds.Contacts)
	case objects.StatusPending:
		contacts = addToMap(contacts, contactIds.PendingIn)
		contacts = addToMap(contacts, contactIds.PendingOut)
	case objects.StatusFavorite:
		contacts = addToMap(contacts, contactIds.Favorites)
	case objects.StatusBlocked:
		contacts = addToMap(contacts, contactIds.BlockedChats)
	case objects.StatusContact:
		contacts = addToMap(contacts, contactIds.BlockedChats)
		contacts = addToMap(contacts, contactIds.PendingOut)
		contacts = addToMap(contacts, contactIds.PendingIn)
		contacts = addToMap(contacts, contactIds.Favorites)
		contacts = addToMap(contacts, contactIds.Contacts)
	}

	chatProjection := bson.M{
		"_id":          1,
		"participants": 1,
		"chatType":     1,
		"name":         1,
		"avatar":       1,
	}

	chats, err := FindAll[models.Chat](context.Background(), objects.DB.Collection(string(objects.ChatColl)), bson.M{"_id": bson.M{"$in": contacts}}, chatProjection, bson.M{}, int64(limit), 0)
	if err != nil {
		return []models.ContactInfo{}, err
	}

	for _, chat := range chats {
		if chat.ChatType == string(objects.ChatTypeDirect) {
			for _, participant := range chat.Participants {
				contactInfo := models.ContactInfo{}
				if participant.UserInfo.UserID != objectID {
					contactInfo = models.ContactInfo{
						ID:           participant.UserInfo.UserID,
						ChatID:       chat.ChatID,
						OnlineStatus: participant.OnlineStatus,
						Username:     participant.UserInfo.Username,
						DisplayName:  participant.UserInfo.DisplayName,
						Avatar:       participant.UserInfo.Avatar,
						IsFavorite:   false,
					}
				}
				contactRequests = append(contactRequests, contactInfo)
			}
		} else {
			contactRequests = append(contactRequests, models.ContactInfo{
				ID:           chat.OwnerID,
				ChatID:       chat.ChatID,
				OnlineStatus: "",
				Username:     chat.Name,
				DisplayName:  chat.Name,
				Avatar:       chat.Avatar,
				IsFavorite:   false,
			})
		}
	}

	return contactRequests, nil
}

// Contact Actions
func SendContactRequest(userID, targetUserID string) (models.ContactInfo, error) {
	// Convert string IDs to ObjectIDs
	userObjectID, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return models.ContactInfo{}, fmt.Errorf("failed to convert user id to object id: %w", err)
	}

	targetObjectID, err := bson.ObjectIDFromHex(targetUserID)
	if err != nil {
		return models.ContactInfo{}, fmt.Errorf("failed to convert target user id to object id: %w", err)
	}

	// Start session for atomic transaction
	sess, err := objects.DBClient.StartSession()
	if err != nil {
		return models.ContactInfo{}, fmt.Errorf("failed to start session: %w", err)
	}
	defer sess.EndSession(context.Background())

	var userRelationship models.ContactInfo

	// Execute all operations in a single transaction
	_, err = sess.WithTransaction(context.Background(), func(sessCtx context.Context) (interface{}, error) {
		// First, check if both users exist and validate the request
		userFilter := bson.M{"_id": userObjectID}
		targetFilter := bson.M{"_id": targetObjectID}

		Projection := bson.M{
			"_id":                      1,
			"contactInfo.blockedChats": 1,
			"contactInfo.pendingOut":   1,
			"contactInfo.pendingIn":    1,
			"contactInfo.favorites":    1,
			"contactInfo.contacts":     1,
			"profile.displayName":      1,
			"profile.avatar":           1,
			"username":                 1,
		}

		// Check if users exist and validate contact request
		user, err := FindByID[models.User](sessCtx, objects.DB.Collection(string(objects.UserColl)), userFilter, Projection)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				return nil, fmt.Errorf("requesting user not found")
			}
			return nil, fmt.Errorf("failed to fetch requesting user: %w", err)
		}

		targetUser, err := FindByID[models.User](sessCtx, objects.DB.Collection(string(objects.UserColl)), targetFilter, Projection)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				return nil, fmt.Errorf("target user not found")
			}
			return nil, fmt.Errorf("failed to fetch target user: %w", err)
		}

		checkContactRequest := func(user *models.User, targetObjectID bson.ObjectID) error {
			if _, ok := user.ContactInfo.PendingOut[targetObjectID]; ok {
				return fmt.Errorf("contact request already sent")
			}

			if _, ok := user.ContactInfo.PendingIn[targetObjectID]; ok {
				return fmt.Errorf("contact request already sent - accept it instead")
			}

			if _, ok := user.ContactInfo.Favorites[targetObjectID]; ok {
				return fmt.Errorf("user is already in your favorites")
			}

			if _, ok := user.ContactInfo.Contacts[targetObjectID]; ok {
				return fmt.Errorf("user is already in your contacts")
			}

			if _, ok := user.ContactInfo.BlockedChats[targetObjectID]; ok {
				return fmt.Errorf("user is blocked")
			}

			return nil
		}

		if err := checkContactRequest(user, targetObjectID); err != nil {
			return nil, err
		}

		if err := checkContactRequest(targetUser, userObjectID); err != nil {
			return nil, err
		}

		// first we create an new chat for two users with default config
		chat := utils.NewChatWithDefaults(userObjectID, string(objects.ChatTypeDirect))
		// now add data to the chat
		participant := utils.NewParticipantWithDefaults(
			string(objects.StatusPending),
			string(objects.UserStatusOnline),
			userObjectID,
			[]string{string(objects.ChatPermissionRead), string(objects.ChatPermissionWrite)},
			models.ContactUserInfo{
				DisplayName: user.Profile.DisplayName,
				Username:    user.Username,
				UserID:      userObjectID,
				Avatar:      user.Profile.Avatar,
			},
		)
		chat.Participants[userObjectID] = participant

		participant = utils.NewParticipantWithDefaults(
			string(objects.StatusPending),
			string(objects.UserStatusOffline),
			targetObjectID,
			[]string{string(objects.ChatPermissionRead), string(objects.ChatPermissionWrite)},
			models.ContactUserInfo{
				DisplayName: targetUser.Profile.DisplayName,
				Username:    targetUser.Username,
				UserID:      targetObjectID,
				Avatar:      targetUser.Profile.Avatar,
			},
		)
		chat.Participants[targetObjectID] = participant

		// now we have to save the chat to the database
		_, err = InsertOne(sessCtx, objects.DB.Collection(string(objects.ChatColl)), chat)
		if err != nil {
			return nil, fmt.Errorf("failed to save chat: %w", err)
		}

		// Create relationship data
		now := time.Now()

		// Update requesting user's contact info
		userUpdateProjection := bson.M{
			"$set": bson.M{
				"contactInfo.pendingOut": bson.M{targetObjectID.Hex(): chat.ChatID},
				"contactInfo.updatedAt":  now,
			},
		}

		_, err = UpdateOne(sessCtx, objects.DB.Collection(string(objects.UserColl)), userFilter, userUpdateProjection)
		if err != nil {
			return nil, fmt.Errorf("failed to update requesting user contact info: %w", err)
		}

		// Update target user's contact info
		targetUpdateProjection := bson.M{
			"$set": bson.M{
				"contactInfo.pendingIn": bson.M{userObjectID.Hex(): chat.ChatID},
				"contactInfo.updatedAt": now,
			},
		}

		_, err = UpdateOne(sessCtx, objects.DB.Collection(string(objects.UserColl)), targetFilter, targetUpdateProjection)
		if err != nil {
			return nil, fmt.Errorf("failed to update target user contact info: %w", err)
		}

		userRelationship = models.ContactInfo{
			ID:           userObjectID,
			ChatID:       chat.ChatID,
			ChatName:     chat.Name,
			ChatAvatar:   chat.Avatar,
			OnlineStatus: chat.Participants[targetObjectID].OnlineStatus,
			Username:     chat.Participants[targetObjectID].UserInfo.Username,
			DisplayName:  chat.Participants[targetObjectID].UserInfo.DisplayName,
			Avatar:       chat.Participants[targetObjectID].UserInfo.Avatar,
			IsFavorite:   false,
		}

		return nil, nil
	})

	if err != nil {
		return models.ContactInfo{}, fmt.Errorf("transaction failed: %w", err)
	}

	return userRelationship, nil
}

func AcceptOrDeclineContactRequest(userID, targetUserID, action string) (models.ContactInfo, error) {
	// get the contact request from the database
	userObjectID, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return models.ContactInfo{}, fmt.Errorf("failed to convert user id to object id: %w", err)
	}

	targetObjectID, err := bson.ObjectIDFromHex(targetUserID)
	if err != nil {
		return models.ContactInfo{}, fmt.Errorf("failed to convert target user id to object id: %w", err)
	}

	sess, err := objects.DBClient.StartSession()
	if err != nil {
		return models.ContactInfo{}, fmt.Errorf("failed to start session: %w", err)
	}
	defer sess.EndSession(context.Background())

	var userRelationship models.ContactInfo

	_, err = sess.WithTransaction(context.Background(), func(sessCtx context.Context) (interface{}, error) {
		// first we have to get the contact request from the database
		userFilter := bson.M{"_id": userObjectID, "contactInfo.pendingIn." + targetObjectID.Hex(): bson.M{"$exists": true}}
		targetFilter := bson.M{"_id": targetObjectID, "contactInfo.pendingOut." + userObjectID.Hex(): bson.M{"$exists": true}}

		contactProjection := bson.M{
			"_id":                    1,
			"contactInfo.pendingIn":  1,
			"contactInfo.pendingOut": 1,
			"contactInfo.favorites":  1,
			"contactInfo.updatedAt":  1,
		}

		userContact, err := FindByID[models.User](sessCtx, objects.DB.Collection(string(objects.UserColl)), userFilter, contactProjection)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				return nil, fmt.Errorf("contact request not found")
			}
			return nil, fmt.Errorf("failed to fetch user contact: %w", err)
		}

		targetContact, err := FindByID[models.User](sessCtx, objects.DB.Collection(string(objects.UserColl)), targetFilter, contactProjection)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				return nil, fmt.Errorf("contact request not found")
			}
			return nil, fmt.Errorf("failed to fetch target contact: %w", err)
		}

		if userContact.ContactInfo.Contacts[targetObjectID] != (bson.ObjectID{}) {
			return nil, fmt.Errorf("user is already in contacts")
		}

		// if (userContact.ContactInfo.PendingIn[targetObjectID] != bson.ObjectID{}) {
		// 	return nil, fmt.Errorf("contact request already sent by user %s please try again", userContact.ContactInfo.PendingIn[targetObjectID].Hex())
		// }

		// if (targetContact.ContactInfo.PendingOut[userObjectID] != bson.ObjectID{}) {
		// 	return nil, fmt.Errorf("contact request already sent by user %s please try again", targetContact.ContactInfo.PendingOut[userObjectID].Hex())
		// }

		now := time.Now()

		chatProjection := bson.M{
			"_id":          1,
			"chatType":     1,
			"name":         1,
			"participants": 1,
			"avatar":       1,
			"ownerId":      1,
		}

		// get the chat from the database
		chat, err := FindByID[models.Chat](sessCtx, objects.DB.Collection(string(objects.ChatColl)), bson.M{"_id": userContact.ContactInfo.PendingIn[targetObjectID]}, chatProjection)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch chat: %w", err)
		}

		switch objects.ContactStatus(action) {
		case objects.StatusAccepted:
			// update the stats
			// in future if the in contacts we store [chatID] -> [chatID] mapping means it's an group

			if userContact.ContactInfo.Contacts == nil {
				userContact.ContactInfo.Contacts = make(map[bson.ObjectID]bson.ObjectID)
			}

			userContact.ContactInfo.Contacts[targetObjectID] = userContact.ContactInfo.PendingIn[targetObjectID]
			userContact.ContactInfo.PendingIn[targetObjectID] = bson.ObjectID{}

			if targetContact.ContactInfo.Contacts == nil {
				targetContact.ContactInfo.Contacts = make(map[bson.ObjectID]bson.ObjectID)
			}

			targetContact.ContactInfo.Contacts[userObjectID] = targetContact.ContactInfo.PendingOut[userObjectID]
			targetContact.ContactInfo.PendingOut[userObjectID] = bson.ObjectID{}

			userRelationship = models.ContactInfo{
				ID:           targetObjectID,
				ChatID:       chat.ChatID,
				ChatName:     chat.Name,
				ChatAvatar:   chat.Avatar,
				OnlineStatus: chat.Participants[targetObjectID].OnlineStatus,
				Username:     chat.Participants[targetObjectID].UserInfo.Username,
				DisplayName:  chat.Participants[targetObjectID].UserInfo.DisplayName,
				Avatar:       chat.Participants[targetObjectID].UserInfo.Avatar,
				IsFavorite:   false,
			}

			if (userContact.ContactInfo.Favorites[targetObjectID] != bson.ObjectID{}) {
				userRelationship.IsFavorite = true
			}

		case objects.StatusDeclined:
			// update the relationship status
			userContact.ContactInfo.PendingIn[targetObjectID] = bson.ObjectID{}
			targetContact.ContactInfo.PendingOut[userObjectID] = bson.ObjectID{}
		}

		userUpdate := bson.M{
			"$unset": bson.M{
				"contactInfo.pendingIn." + targetObjectID.Hex(): "",
				"contactInfo.pendingOut." + userObjectID.Hex():  "",
			},
			"$set": bson.M{
				"contactInfo.contacts":  bson.M{targetObjectID.Hex(): userContact.ContactInfo.Contacts[targetObjectID]},
				"contactInfo.updatedAt": now,
			},
		}

		_, err = UpdateOne(sessCtx, objects.DB.Collection(string(objects.UserColl)), userFilter, userUpdate)
		if err != nil {
			return nil, fmt.Errorf("failed to update user contact: %w", err)
		}

		// update the target contact
		targetUpdate := bson.M{
			"$unset": bson.M{
				"contactInfo.pendingIn." + targetObjectID.Hex(): "",
				"contactInfo.pendingOut." + userObjectID.Hex():  "",
			},
			"$set": bson.M{
				"contactInfo.contacts":  bson.M{userObjectID.Hex(): targetContact.ContactInfo.Contacts[userObjectID]},
				"contactInfo.updatedAt": now,
			},
		}

		_, err = UpdateOne(sessCtx, objects.DB.Collection(string(objects.UserColl)), targetFilter, targetUpdate)
		if err != nil {
			return nil, fmt.Errorf("failed to update target contact: %w", err)
		}

		return nil, nil
	})

	if err != nil {
		return models.ContactInfo{}, fmt.Errorf("transaction failed: %w", err)
	}

	return userRelationship, nil
}

func RemoveContact(userID, targetUserID string) (bool, error) {
	userObjectID, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return false, fmt.Errorf("failed to convert user id to object id: %w", err)
	}

	targetObjectID, err := bson.ObjectIDFromHex(targetUserID)
	if err != nil {
		return false, fmt.Errorf("failed to convert target user id to object id: %w", err)
	}

	sess, err := objects.DBClient.StartSession()
	if err != nil {
		return false, fmt.Errorf("failed to start session: %w", err)
	}
	defer sess.EndSession(context.Background())

	iscontactRemove := false

	// the sess will return the userContact and the error
	_, err = sess.WithTransaction(context.Background(), func(sessCtx context.Context) (interface{}, error) {
		// search in contact collection for the contactId
		userFilter := bson.M{"_id": userObjectID, "contactInfo.contacts." + targetObjectID.Hex(): bson.M{"$exists": true}}

		userContact, err := FindByID[models.ContactRequest](sessCtx, objects.DB.Collection(string(objects.UserColl)), userFilter, bson.M{
			"_id":                  1,
			"contactInfo.contacts": bson.M{targetObjectID.Hex(): 1},
		})
		if err != nil {
			return iscontactRemove, fmt.Errorf("failed to fetch user contact: %w", err)
		}

		chatId := userContact.ContactInfo.Contacts[targetObjectID]
		if (chatId == bson.ObjectID{}) {
			return iscontactRemove, fmt.Errorf("target is not present in user contacts")
		}

		now := time.Now()

		// update chat collection
		chatFilter := bson.M{"_id": chatId}
		chatUpdateProjection := bson.M{
			"$set": bson.M{
				"participants." + userObjectID.Hex(): bson.M{},
			},
		}
		_, err = UpdateOne(sessCtx, objects.DB.Collection(string(objects.ChatColl)), chatFilter, chatUpdateProjection)
		if err != nil {
			return iscontactRemove, fmt.Errorf("failed to update user chat: %w", err)
		}

		// update user collection
		userUpdate := bson.M{
			"$set": bson.M{
				"contactInfo.blockedChats": bson.M{targetObjectID.Hex(): bson.ObjectID{}},
				"contactInfo.favorites":    bson.M{targetObjectID.Hex(): bson.ObjectID{}},
				"contactInfo.contacts":     bson.M{targetObjectID.Hex(): bson.ObjectID{}},
				"updatedAt":                now,
			},
		}
		_, err = UpdateOne(sessCtx, objects.DB.Collection(string(objects.UserColl)), userFilter, userUpdate)
		if err != nil {
			return iscontactRemove, fmt.Errorf("failed to update user contact: %w", err)
		}
		iscontactRemove = true
		return iscontactRemove, nil
	})

	return iscontactRemove, err
}

func BlockUser(userID, targetUserID string) (bool, error) {
	userObjectID, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return false, fmt.Errorf("failed to convert user id to object id: %w", err)
	}

	targetObjectID, err := bson.ObjectIDFromHex(targetUserID)
	if err != nil {
		return false, fmt.Errorf("failed to convert target user id to object id: %w", err)
	}

	isContactBlocked := false

	sess, err := objects.DBClient.StartSession()
	if err != nil {
		return isContactBlocked, fmt.Errorf("failed to start session: %w", err)
	}
	defer sess.EndSession(context.Background())

	_, err = sess.WithTransaction(context.Background(), func(sessCtx context.Context) (interface{}, error) {
		// search in contact collection for the contactId
		userFilter := bson.M{"_id": userObjectID, "contactInfo.contacts." + targetObjectID.Hex(): bson.M{"$exists": true}}

		userContact, err := FindByID[models.ContactRequest](sessCtx, objects.DB.Collection(string(objects.UserColl)), userFilter, bson.M{
			"_id":                      1,
			"contactInfo.blockedChats": bson.M{targetObjectID.Hex(): 1},
			"contactInfo.contacts":     bson.M{targetObjectID.Hex(): 1},
		})
		if err != nil {
			if err == mongo.ErrNoDocuments {
				return nil, fmt.Errorf("user not found")
			}
			return nil, fmt.Errorf("failed to fetch user contact: %w", err)
		}

		chatId := userContact.ContactInfo.Contacts[targetObjectID]

		chatFilter := bson.M{"_id": chatId}

		// we have to check if this works or not
		chatUpdateProjection := bson.M{
			"$set": bson.M{
				"participants." + targetObjectID.Hex(): bson.M{
					"isBlocked": true,
				},
			},
		}

		_, err = UpdateOne(sessCtx, objects.DB.Collection(string(objects.ChatColl)), chatFilter, chatUpdateProjection)
		if err != nil {
			return nil, fmt.Errorf("failed to update chat contact: %w", err)
		}

		now := time.Now()

		userUpdateProjection := bson.M{}
		userUpdateProjection["contactInfo.updatedAt"] = now

		_, err = UpdateOne(context.Background(), objects.DB.Collection(string(objects.UserColl)), userFilter, userUpdateProjection)
		if err != nil {
			return nil, fmt.Errorf("failed to update user contact: %w", err)
		}
		return nil, err
	})
	isContactBlocked = true

	return isContactBlocked, err
}

func AddToFavorites(userID, targetUserID string) (bool, error) {
	userObjectID, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return false, fmt.Errorf("failed to convert user id to object id: %w", err)
	}

	targetObjectID, err := bson.ObjectIDFromHex(targetUserID)
	if err != nil {
		return false, fmt.Errorf("failed to convert target user id to object id: %w", err)
	}

	isContactFavorite := false

	// search in contact collection for the contactId
	userFilter := bson.M{"_id": userObjectID, "contactInfo.contacts." + targetObjectID.Hex(): bson.M{"$exists": true}}

	userContact, err := FindByID[models.ContactRequest](context.Background(), objects.DB.Collection(string(objects.UserColl)), userFilter, bson.M{
		"_id":                   1,
		"contactInfo.favorites": 1,
		"contactInfo.contacts":  1,
	})
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return isContactFavorite, fmt.Errorf("user not found")
		}
		return isContactFavorite, fmt.Errorf("failed to fetch user contact: %w", err)
	}

	if (userContact.ContactInfo.Favorites[targetObjectID] != bson.ObjectID{}) {
		return isContactFavorite, fmt.Errorf("user is already in favorites contacts")
	}

	now := time.Now()

	chatId := userContact.ContactInfo.Contacts[targetObjectID]

	userUpdateProjection := bson.M{
		"$set": bson.M{
			"contactInfo.blockedChats": bson.M{targetObjectID.Hex(): bson.ObjectID{}},
			"contactInfo.favorites":    bson.M{targetObjectID.Hex(): userContact.ContactInfo.Favorites[chatId]},
			"updatedAt":                now,
		},
	}

	_, err = UpdateOne(context.Background(), objects.DB.Collection(string(objects.UserColl)), userFilter, userUpdateProjection)
	if err != nil {
		return isContactFavorite, fmt.Errorf("failed to update user contact: %w", err)
	}

	isContactFavorite = true

	return isContactFavorite, err
}
