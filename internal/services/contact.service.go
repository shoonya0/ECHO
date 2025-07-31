package services

// // ============ CONTACTS & FRIENDS MANAGEMENT ============

// // this return array of users id with their name and avatar
// func GetContacts(userID string, contactType objects.ContactType, contactStatus objects.ContactStatus) ([]models.User, error) {
// 	users := []models.User{}

// 	objectID, err := bson.ObjectIDFromHex(userID)
// 	if err != nil {
// 		return []models.User{}, err
// 	}

// 	contactProjection := bson.M{
// 		"_id":       1,
// 		"contacts":  1,
// 		"updatedAt": 1,
// 	}

// 	// first we have to get the information from the contact collection
// 	contact, err := FindByID[models.Contact](context.Background(), objects.DB.Collection(string(objects.ContactColl)), bson.M{"_id": objectID}, contactProjection)
// 	if err != nil {
// 		return []models.User{}, err
// 	}

// 	// now we will get all the chat id from the contact collection
// 	chatIds := []bson.ObjectID{}
// 	contactRequestProjection := bson.M{
// 		"_id":         1,
// 		"requestedTo": 1,
// 		"status":      1,
// 		"updatedAt":   1,
// 	}
// 	contactRequestCursor, err := FindMany(context.Background(), objects.DB.Collection(string(objects.ContactRequestColl)), bson.M{"_id": bson.M{"$in": contact.Contacts}}, contactRequestProjection, bson.M{"updatedAt": -1}, 0, 0)
// 	if err != nil {
// 		return []models.User{}, fmt.Errorf("failed to fetch recent users: %w", err)
// 	}
// 	defer contactRequestCursor.Close(context.Background())

// 	for contactRequestCursor.Next(context.Background()) {
// 		var contactRequest models.ContactRequest
// 		if err := contactRequestCursor.Decode(&contactRequest); err != nil {
// 			return []models.User{}, fmt.Errorf("failed to decode contact request: %w", err)
// 		}
// 		switch contactStatus {
// 		case objects.StatusAccepted:
// 			chatIds = append(chatIds, contactRequest.ChatID)
// 		case objects.StatusPending:
// 			chatIds = append(chatIds, contactRequest.ChatID)
// 		case objects.StatusBlocked:
// 			chatIds = append(chatIds, contactRequest.ChatID)
// 		case objects.StatusFavorite:
// 			chatIds = append(chatIds, contactRequest.ChatID)
// 		}
// 	}

// 	// now we find all the chat id(this is mongo id) and fetch the projection of name ,avater and sort it by updated at
// 	chatProjection := bson.M{
// 		"_id":       1,
// 		"name":      1,
// 		"avatar":    1,
// 		"updatedAt": 1,
// 	}

// 	var chatCursor *mongo.Cursor

// 	if contactType == objects.RecentContacts {
// 		chatCursor, err = FindMany(context.Background(), objects.DB.Collection(string(objects.ChatColl)), bson.M{"_id": bson.M{"$in": chatIds}}, chatProjection, bson.M{"updatedAt": -1}, 0, 0)
// 		if err != nil {
// 			return []models.User{}, fmt.Errorf("failed to fetch recent users: %w", err)
// 		}
// 	} else if contactType == objects.AllContacts {
// 		chatCursor, err = FindMany(context.Background(), objects.DB.Collection(string(objects.ChatColl)), bson.M{"_id": bson.M{"$in": contact.Contacts}}, chatProjection, bson.M{}, 0, 0)
// 		if err != nil {
// 			return []models.User{}, fmt.Errorf("failed to fetch all users: %w", err)
// 		}
// 	}

// 	defer chatCursor.Close(context.Background())

// 	for chatCursor.Next(context.Background()) {
// 		var chat models.Chat
// 		if err := chatCursor.Decode(&chat); err != nil {
// 			return []models.User{}, fmt.Errorf("failed to decode chat: %w", err)
// 		}
// 		users = append(users, models.User{
// 			ID:       chat.ChatID,
// 			Username: *chat.Name,
// 			Avatar:   *chat.Avatar,
// 		})
// 	}

// 	return users, nil
// }

// func GetSentContactRequests(userID string) ([]models.User, error) {
// 	users := []models.User{}

// 	objectID, err := bson.ObjectIDFromHex(userID)
// 	if err != nil {
// 		return []models.User{}, err
// 	}

// 	contactRequestProjection := bson.M{
// 		"_id":          1,
// 		"sentRequests": 1,
// 	}

// 	contact, err := FindByID[models.Contact](context.Background(), objects.DB.Collection(string(objects.ContactColl)), bson.M{"_id": objectID}, contactRequestProjection)
// 	if err != nil {
// 		return []models.User{}, fmt.Errorf("failed to fetch sent contact requests: %w", err)
// 	}

// 	userProjection := bson.M{
// 		"_id":      1,
// 		"username": 1,
// 		"avatar":   1,
// 	}

// 	userCursor, err := FindMany(context.Background(), objects.DB.Collection(string(objects.UserColl)), bson.M{"_id": bson.M{"$in": contact.SentRequests}}, userProjection, bson.M{"updatedAt": -1}, 0, 0)
// 	if err != nil {
// 		return []models.User{}, fmt.Errorf("failed to fetch sent contact requests: %w", err)
// 	}
// 	defer userCursor.Close(context.Background())

// 	for userCursor.Next(context.Background()) {
// 		var user models.User
// 		if err := userCursor.Decode(&user); err != nil {
// 			return []models.User{}, fmt.Errorf("failed to decode user: %w", err)
// 		}
// 		users = append(users, user)
// 	}

// 	return users, nil
// }

// // Contact Actions
// func SendContactRequest(userID, targetUserID string) (models.ContactRequest, error) {
// 	// get the contact of the target user from the database
// 	userObjectID, err := bson.ObjectIDFromHex(userID)
// 	if err != nil {
// 		return models.ContactRequest{}, fmt.Errorf("failed to convert target user id to object id: %w", err)
// 	}

// 	targetObjectID, err := bson.ObjectIDFromHex(targetUserID)
// 	if err != nil {
// 		return models.ContactRequest{}, fmt.Errorf("failed to convert target user id to object id: %w", err)
// 	}

// 	targetProjection := bson.M{
// 		"_id":      1,
// 		"contacts": 1,
// 	}

// 	// check if user exist if yes then add the contact request to the contact collection
// 	targetContact, err := FindByID[models.Contact](context.Background(), objects.DB.Collection(string(objects.ContactColl)), bson.M{"_id": targetObjectID}, targetProjection)
// 	if err != nil {
// 		if err == mongo.ErrNoDocuments {
// 			// create a new contact for the user
// 			targetContact = models.Contact{
// 				ID:           targetObjectID,
// 				Contacts:     []bson.ObjectID{},
// 				CreatedAt:    time.Now(),
// 				UpdatedAt:    time.Now(),
// 				SentRequests: []bson.ObjectID{},
// 			}
// 			_, err = InsertOne(context.Background(), objects.DB.Collection(string(objects.ContactColl)), targetContact)
// 			if err != nil {
// 				return models.ContactRequest{}, fmt.Errorf("failed to insert contact: %w", err)
// 			}
// 		} else {
// 			return models.ContactRequest{}, fmt.Errorf("failed to fetch contact: %w", err)
// 		}
// 	}

// 	for _, contact := range targetContact.Contacts {
// 		if contact == userObjectID {
// 			return models.ContactRequest{}, fmt.Errorf("user already in contact")
// 		}
// 	}

// 	// now we will check if the user is already in the contact collection
// 	contactRequestDocument := models.ContactRequest{
// 		RequestedBy: userObjectID,
// 		RequestedTo: targetObjectID,
// 		Status:      objects.StatusPending,
// 		CreatedAt:   time.Now(),
// 		UpdatedAt:   time.Now(),
// 	}

// 	newChatID, err := InsertOne(context.Background(), objects.DB.Collection(string(objects.ContactRequestColl)), contactRequestDocument)
// 	if err != nil {
// 		return models.ContactRequest{}, fmt.Errorf("failed to insert contact request: %w", err)
// 	}

// 	targetContact.Contacts = append(targetContact.Contacts, newChatID)

// 	_, err = UpdateOne(context.Background(), objects.DB.Collection(string(objects.ContactColl)), bson.M{"_id": targetObjectID}, bson.M{"$set": bson.M{"contacts": targetContact.Contacts}})
// 	if err != nil {
// 		return models.ContactRequest{}, fmt.Errorf("failed to update contact: %w", err)
// 	}

// 	userProjection := bson.M{
// 		"_id":          1,
// 		"contacts":     1,
// 		"sentRequests": 1,
// 	}
// 	userContact, err := FindByID[models.Contact](context.Background(), objects.DB.Collection(string(objects.ContactColl)), bson.M{"_id": userObjectID}, userProjection)
// 	if err != nil {
// 		return models.ContactRequest{}, fmt.Errorf("failed to fetch contact: %w", err)
// 	}

// 	userContact.Contacts = append(userContact.Contacts, newChatID)
// 	userContact.SentRequests = append(userContact.SentRequests, newChatID)

// 	_, err = UpdateOne(context.Background(), objects.DB.Collection(string(objects.ContactColl)), bson.M{"_id": userObjectID}, bson.M{"$set": userContact})
// 	if err != nil {
// 		return models.ContactRequest{}, fmt.Errorf("failed to update contact: %w", err)
// 	}

// 	contactRequestDocument.ChatID = newChatID
// 	return contactRequestDocument, nil
// }

// // func AcceptOrDeclineContactRequest(c *gin.Context) (models.User, error) {
// // 	userID, ok := c.Get("user_id")
// // 	if !ok {
// // 		return models.User{}, fmt.Errorf("user id not found")
// // 	}

// // 	requestId := c.Param("requestId")
// // 	action := c.Query("action")

// // 	// get the contact request from the database
// // 	contactRequest := models.ContactRequest{}
// // 	if err := objects.DBClient.Database("ECHO").Collection("contacts").FindOne(c.Request.Context(), bson.M{"contacts.requested_by": requestId, "contacts.status": "pending"}).Decode(&contactRequest); err != nil {
// // 		return models.User{}, fmt.Errorf("failed to fetch contact request: %w", err)
// // 	}

// // 	if action == "accept" {
// // 		contactRequest.Status = "accepted"
// // 		contactRequest.ChatID = uuid.New().String()
// // 		// now we will create a new chat
// // 		objectID, err := bson.ObjectIDFromHex(contactRequest.ChatID)
// // 		if err != nil {
// // 			return models.User{}, fmt.Errorf("failed to convert chat id to object id: %w", err)
// // 		}

// // 		chat := models.Chat{
// // 			ChatID: objectID,
// // 			Participants: map[string]string{
// // 				userID.(string):            userID.(string),
// // 				contactRequest.RequestedTo: contactRequest.RequestedTo,
// // 			},
// // 			CreatedAt: time.Now(),
// // 			UpdatedAt: time.Now(),
// // 		}
// // 		// now we will insert the chat into the database
// // 		if _, err := objects.DBClient.Database("ECHO").Collection("chats").InsertOne(c.Request.Context(), chat); err != nil {
// // 			return models.User{}, fmt.Errorf("failed to insert chat: %w", err)
// // 		}

// // 		// now we will update the contact request in the database
// // 		if _, err := objects.DBClient.Database("ECHO").Collection("contacts").UpdateOne(c.Request.Context(), bson.M{"user_id": userID.(string)}, bson.M{"$set": contactRequest}); err != nil {
// // 			return models.User{}, fmt.Errorf("failed to update contact: %w", err)
// // 		}
// // 		return models.User{}, nil
// // 	} else {
// // 		contactRequest.Status = "declined"
// // 		contactRequest.ChatID = ""
// // 	}

// // 	return models.User{}, nil
// // }

// // func RemoveContact(c *gin.Context) (string, error) {
// // 	// userID, ok := c.Get("user_id")
// // 	// if !ok {
// // 	// 	return "", fmt.Errorf("user id not found")
// // 	// }

// // 	contactId := c.Param("contactId")

// // 	// get the chat Id from request body
// // 	// chatId := c.Request.Body.ChatId

// // 	// search in contact collection for the contactId
// // 	contact := models.Contact{}
// // 	if err := objects.DBClient.Database("ECHO").Collection("contacts").FindOne(c.Request.Context(), bson.M{"_id": contactId}).Decode(&contact); err != nil {
// // 		return "", fmt.Errorf("failed to fetch contact: %w", err)
// // 	}

// // 	// remove the chatId from the contact
// // 	// for _, contactStatus := range contact.Contacts {
// // 	// if contactStatus.ChatId == chatId {
// // 	// 	delete(contact.Contacts, contactStatus)
// // 	// 	// update the contact in the database
// // 	// 	if _, err := objects.DBClient.Database("ECHO").Collection("contacts").UpdateOne(c.Request.Context(), bson.M{"_id": contactId}, bson.M{"$set": contact}); err != nil {
// // 	// 		return "", fmt.Errorf("failed to update contact: %w", err)
// // 	// 	}

// // 	// 	// remove the chat from the chat collection
// // 	// 	if _, err := objects.DBClient.Database("ECHO").Collection("chats").DeleteOne(c.Request.Context(), bson.M{"_id": chatId}); err != nil {
// // 	// 		return "", fmt.Errorf("failed to delete chat: %w", err)
// // 	// 	}
// // 	// 	return "contact removed", nil
// // 	// }
// // 	// }

// // 	return "contact removed", nil
// // }

// // func BlockUser(c *gin.Context) (string, error) {
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

// // 	// update the contact status to blocked
// // 	contact.Contacts[targetUserId] = models.ContactRequest{
// // 		RequestedBy: userID.(string),
// // 		Status:      "blocked",
// // 		ChatID:      contact.Contacts[targetUserId].ChatID,
// // 	}

// // 	// update the contact in the database
// // 	if _, err := objects.DBClient.Database("ECHO").Collection("contacts").UpdateOne(c.Request.Context(), bson.M{"user_id": userID.(string)}, bson.M{"$set": contact}); err != nil {
// // 		return "", fmt.Errorf("failed to update contact: %w", err)
// // 	}

// // 	return "user blocked", nil
// // }

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
