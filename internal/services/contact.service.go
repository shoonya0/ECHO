package services

import (
	"fmt"
	"gin/internal/models"
	"gin/objects"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func SendContactRequest(c *gin.Context) (models.User, error) {
	userID, ok := c.Get("user_id")
	if !ok {
		return models.User{}, fmt.Errorf("user id not found")
	}

	// get user params
	targetUserID := c.Param("targetUserId")

	// get the contact of the target user from the database
	contact := models.Contact{}
	if err := objects.DBClient.Database("ECHO").Collection("contacts").FindOne(c.Request.Context(), bson.M{"user_id": targetUserID}).Decode(&contact); err != nil {
		if err == mongo.ErrNoDocuments {
			return models.User{}, fmt.Errorf("no contact found")
		}
		return models.User{}, fmt.Errorf("failed to fetch contact: %w", err)
	}

	// now we will find all the contact of the target user and check if the user id is already in the contact collection
	for _, contact := range contact.Contacts {
		if contact.RequestedBy == userID.(string) && contact.Status == "pending" {
			return models.User{}, fmt.Errorf("request already sent")
		}
	}

	// now we will create a new contact request
	contactRequest := models.ContactStatus{
		RequestedBy: userID.(string),
		Status:      "pending",
		ChatId:      "",
	}

	// now we will insert the contact request into the database
	contact.Contacts[targetUserID] = contactRequest

	// now we will update the contact in the database
	if _, err := objects.DBClient.Database("ECHO").Collection("contacts").UpdateOne(c.Request.Context(), bson.M{"user_id": userID.(string)}, bson.M{"$set": contact}); err != nil {
		return models.User{}, fmt.Errorf("failed to update contact: %w", err)
	}

	return models.User{
		ID:       userID.(string),
		Username: *chat.Name,
		Avatar:   chat.Avatar,
	}, nil
}

func AcceptOrDeclineContactRequest(c *gin.Context) (models.User, error) {
	userID, ok := c.Get("user_id")
	if !ok {
		return models.User{}, fmt.Errorf("user id not found")
	}

	requestId := c.Param("requestId")
	action := c.Query("action")

	// get the contact request from the database
	contactRequest := models.ContactStatus{}
	if err := objects.DBClient.Database("ECHO").Collection("contacts").FindOne(c.Request.Context(), bson.M{"contacts.requested_by": requestId, "contacts.status": "pending"}).Decode(&contactRequest); err != nil {
		return models.User{}, fmt.Errorf("failed to fetch contact request: %w", err)
	}

	if action == "accept" {
		contactRequest.Status = "accepted"
		contactRequest.ChatId = uuid.New().String()
		// now we will create a new chat
		chat := models.Chat{
			ID:        contactRequest.ChatId,
			Users:     []string{userID.(string), contactRequest.RequestedBy},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		// now we will insert the chat into the database
		if _, err := objects.DBClient.Database("ECHO").Collection("chats").InsertOne(c.Request.Context(), chat); err != nil {
			return models.User{}, fmt.Errorf("failed to insert chat: %w", err)
		}
		contact.Contacts[userID.(string)] = contactRequest
		contact.Contacts[contactRequest.RequestedBy] = contactRequest

		// now we will update the contact request in the database
		if _, err := objects.DBClient.Database("ECHO").Collection("contacts").UpdateOne(c.Request.Context(), bson.M{"user_id": userID.(string)}, bson.M{"$set": contact}); err != nil {
			return models.User{}, fmt.Errorf("failed to update contact: %w", err)
		}
	} else {
		contactRequest.Status = "declined"
		contactRequest.ChatId = ""
	}

	// update the contact request in the database
	if _, err := objects.DBClient.Database("ECHO").Collection("contacts").UpdateOne(c.Request.Context(), bson.M{"user_id": userID.(string)}, bson.M{"$set": contact}); err != nil {
		return models.User{}, fmt.Errorf("failed to update contact: %w", err)
	}

	return models.User{
		ID:       userID.(string),
		Username: *chat.Name,
		Avatar:   chat.Avatar,
	}, nil
}

func RemoveContact(c *gin.Context) (string, error) {
	userID, ok := c.Get("user_id")
	if !ok {
		return "", fmt.Errorf("user id not found")
	}

	contactId := c.Param("contactId")

	// get the chat Id from request body
	chatId := c.Request.Body.ChatId

	// search in contact collection for the contactId
	contact := models.Contact{}
	if err := objects.DBClient.Database("ECHO").Collection("contacts").FindOne(c.Request.Context(), bson.M{"_id": contactId}).Decode(&contact); err != nil {
		return "", fmt.Errorf("failed to fetch contact: %w", err)
	}

	// remove the chatId from the contact
	for _, contactStatus := range contact.Contacts {
		if contactStatus.ChatId == chatId {
			delete(contact.Contacts, contactStatus)
			// update the contact in the database
			if _, err := objects.DBClient.Database("ECHO").Collection("contacts").UpdateOne(c.Request.Context(), bson.M{"_id": contactId}, bson.M{"$set": contact}); err != nil {
				return "", fmt.Errorf("failed to update contact: %w", err)
			}

			// remove the chat from the chat collection
			if _, err := objects.DBClient.Database("ECHO").Collection("chats").DeleteOne(c.Request.Context(), bson.M{"_id": chatId}); err != nil {
				return "", fmt.Errorf("failed to delete chat: %w", err)
			}
			return "contact removed", nil
		}
	}

	return "", fmt.Errorf("chat not found")
}

func BlockUser(c *gin.Context) (string, error) {
	userID, ok := c.Get("user_id")
	if !ok {
		return "", fmt.Errorf("user id not found")
	}

	targetUserId := c.Param("userId")

	// get the contact of the user from the database
	contact := models.Contact{}
	if err := objects.DBClient.Database("ECHO").Collection("contacts").FindOne(c.Request.Context(), bson.M{"_id": userID.(string)}).Decode(&contact); err != nil {
		return "", fmt.Errorf("failed to fetch contact: %w", err)
	}

	// update the contact status to blocked
	contact.Contacts[targetUserId] = models.ContactStatus{
		RequestedBy: userID.(string),
		Status:      "blocked",
		ChatId:      contact.Contacts[targetUserId].ChatId,
	}

	// update the contact in the database
	if _, err := objects.DBClient.Database("ECHO").Collection("contacts").UpdateOne(c.Request.Context(), bson.M{"user_id": userID.(string)}, bson.M{"$set": contact}); err != nil {
		return "", fmt.Errorf("failed to update contact: %w", err)
	}

	return "user blocked", nil
}

func UnblockUser(c *gin.Context) (string, error) {
	userID, ok := c.Get("user_id")
	if !ok {
		return "", fmt.Errorf("user id not found")
	}

	targetUserId := c.Param("userId")

	// get the contact of the user from the database
	contact := models.Contact{}
	if err := objects.DBClient.Database("ECHO").Collection("contacts").FindOne(c.Request.Context(), bson.M{"_id": userID.(string)}).Decode(&contact); err != nil {
		return "", fmt.Errorf("failed to fetch contact: %w", err)
	}

	// update the contact status to unblocked
	contact.Contacts[targetUserId] = models.ContactStatus{
		RequestedBy: userID.(string),
		Status:      "accepted",
		ChatId:      contact.Contacts[targetUserId].ChatId,
	}

	// update the contact in the database
	if _, err := objects.DBClient.Database("ECHO").Collection("contacts").UpdateOne(c.Request.Context(), bson.M{"_id": userID.(string)}, bson.M{"$set": contact}); err != nil {
		return "", fmt.Errorf("failed to update contact: %w", err)
	}

	return "user unblocked", nil
}

func AddToFavorites(c *gin.Context) (string, error) {
	userID, ok := c.Get("user_id")
	if !ok {
		return "", fmt.Errorf("user id not found")
	}

	targetUserId := c.Param("userId")

	// get the contact of the user from the database
	contact := models.Contact{}
	if err := objects.DBClient.Database("ECHO").Collection("contacts").FindOne(c.Request.Context(), bson.M{"_id": userID.(string)}).Decode(&contact); err != nil {
		return "", fmt.Errorf("failed to fetch contact: %w", err)
	}

	// update the contact status to favorite
	contact.Contacts[targetUserId] = models.ContactStatus{
		RequestedBy: userID.(string),
		Status:      "favorite",
		ChatId:      contact.Contacts[targetUserId].ChatId,
	}

	// update the contact in the database
	if _, err := objects.DBClient.Database("ECHO").Collection("contacts").UpdateOne(c.Request.Context(), bson.M{"_id": userID.(string)}, bson.M{"$set": contact}); err != nil {
		return "", fmt.Errorf("failed to update contact: %w", err)
	}

	return "user added to favorites", nil
}

func RemoveFromFavorites(c *gin.Context) (string, error) {
	userID, ok := c.Get("user_id")
	if !ok {
		return "", fmt.Errorf("user id not found")
	}
	targetUserId := c.Param("userId")

	// get the contact of the user from the database
	contact := models.Contact{}
	if err := objects.DBClient.Database("ECHO").Collection("contacts").FindOne(c.Request.Context(), bson.M{"_id": userID.(string)}).Decode(&contact); err != nil {
		return "", fmt.Errorf("failed to fetch contact: %w", err)
	}

	// update the contact status to not favorite
	contact.Contacts[targetUserId] = models.ContactStatus{
		RequestedBy: userID.(string),
		Status:      "accepted",
		ChatId:      contact.Contacts[targetUserId].ChatId,
	}

	// update the contact in the database
	if _, err := objects.DBClient.Database("ECHO").Collection("contacts").UpdateOne(c.Request.Context(), bson.M{"_id": userID.(string)}, bson.M{"$set": contact}); err != nil {
		return "", fmt.Errorf("failed to update contact: %w", err)
	}

	return "user removed from favorites", nil
}

func GetFavoriteContacts(c *gin.Context) ([]models.Chat, error) {
	userID, ok := c.Get("user_id")
	if !ok {
		return nil, fmt.Errorf("user id not found")
	}

	// get the contact of the user from the database
	contact := models.Contact{}
	if err := objects.DBClient.Database("ECHO").Collection("contacts").FindOne(c.Request.Context(), bson.M{"_id": userID.(string)}).Decode(&contact); err != nil {
		return nil, fmt.Errorf("failed to fetch contact: %w", err)
	}

	favoriteChats := []models.Chat{}

	// get the favorite contacts
	for _, contactStatus := range contact.Contacts {
		if contactStatus.Status == "favorite" {
			chat := models.Chat{}

			// in this case we have to get the chat info from the chat collection
			if err := objects.DBClient.Database("ECHO").Collection("chats").FindOne(c.Request.Context(), bson.M{"_id": contactStatus.ChatId}).Decode(&chat); err != nil {
				return nil, fmt.Errorf("failed to fetch chat: %w", err)
			}
			favoriteChats = append(favoriteChats, chat)
		}
	}

	return favoriteChats, nil
}
