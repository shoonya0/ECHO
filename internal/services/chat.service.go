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

// ============ ENHANCED CHAT SERVICE WITH REAL-TIME SUPPORT ============

// GetUserChats retrieves all chats where the user is a participant
func GetUserChats(userID bson.ObjectID) (map[bson.ObjectID]bson.ObjectID, error) {
	ctx := context.Background()

	userFilter := bson.M{"_id": userID}
	userProjection := bson.M{
		"contactInfo.contacts": 1,
	}

	user, err := FindByID[models.ContactRequest](ctx, objects.DB.Collection(string(objects.UserColl)), userFilter, userProjection)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("user have no contacts")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	chatInfo := make(map[bson.ObjectID]bson.ObjectID)

	// now we will retrive the chat info which we want to get from the infoToGetFromChat
	for chatUserID, chatID := range user.ContactInfo.Contacts {
		chatInfo[chatUserID] = chatID
	}

	return chatInfo, nil
}

// GetChat retrieves chat information with basic details
func GetChat(chatInfo models.ContactInfo) (models.Chat, error) {
	chatFilter := bson.M{"_id": chatInfo.ChatID}
	chatProjection := bson.M{
		"_id":           1,
		"chatType":      1,
		"name":          1,
		"lastMessageId": 1,
		"participants":  1,
		"stats":         1,
		"createdAt":     1,
		"updatedAt":     1,
	}

	chat, err := FindByID[models.Chat](context.Background(), objects.DB.Collection(string(objects.ChatColl)), chatFilter, chatProjection)
	if err != nil {
		return models.Chat{}, fmt.Errorf("failed to get chat: %w", err)
	}

	return *chat, nil
}

// GetChatWithMessages retrieves chat with recent messages
func GetChatWithMessages(chatID bson.ObjectID, limit int, offset int) (models.Chat, []models.Message, error) {
	ctx := context.Background()

	// Get chat details
	chat, err := GetChat(models.ContactInfo{ChatID: chatID})
	if err != nil {
		return models.Chat{}, nil, fmt.Errorf("failed to get chat: %w", err)
	}

	// Get recent messages
	messageFilter := bson.M{"chatId": chatID}

	// Get recent messages using FindAll
	messages, err := FindAll[models.Message](
		ctx,
		objects.DB.Collection(string(objects.MessageColl)),
		messageFilter,
		bson.M{},                // No projection, get all fields
		bson.M{"createdAt": -1}, // Sort by createdAt descending (most recent first)
		int64(limit),
		int64(offset),
	)
	if err != nil {
		return chat, nil, fmt.Errorf("failed to get messages: %w", err)
	}

	// Reverse messages to show oldest first
	for i := 0; i < len(messages)/2; i++ {
		j := len(messages) - 1 - i
		messages[i], messages[j] = messages[j], messages[i]
	}

	return chat, messages, nil
}

// CreateDirectChat creates a new direct chat between two users
func CreateDirectChat(userID1, userID2 bson.ObjectID) (*models.Chat, error) {
	ctx := context.Background()

	// Check if direct chat already exists
	filter := bson.M{
		"chatType": "direct",
		"$and": []bson.M{
			{"participants." + userID1.Hex(): bson.M{"$exists": true}},
			{"participants." + userID2.Hex(): bson.M{"$exists": true}},
		},
	}

	existingChat, err := FindByFilter[models.Chat](ctx, objects.DB.Collection(string(objects.ChatColl)), filter, nil)
	if err == nil {
		return &existingChat, nil
	}

	return &existingChat, nil
}

// CreateGroupChat creates a new group chat
func CreateGroupChat(creatorID bson.ObjectID, name, description string, participantIDs []bson.ObjectID) (*models.Chat, error) {
	ctx := context.Background()
	now := time.Now()

	// Get creator details
	_, err := GetUserByID(creatorID)
	if err != nil {
		return nil, fmt.Errorf("failed to get creator: %w", err)
	}

	participants := make(map[bson.ObjectID]models.ParticipantEmbed)

	// Add creator as owner (using ParticipantRef without embedded user data)
	participants[creatorID] = models.ParticipantEmbed{
		Role: "owner",
		UserInfo: models.ContactUserInfo{
			UserID:      creatorID,
			DisplayName: "Owner",
			Username:    "owner",
			Avatar:      "https://example.com/avatar.png",
		},
		JoinedAt: now,
	}

	// Add other participants
	for _, userID := range participantIDs {
		if userID == creatorID {
			continue // Skip creator, already added
		}

		// Just verify user exists, but don't embed user data
		_, err := GetUserByID(userID)
		if err != nil {
			continue // Skip invalid users
		}

		participants[userID] = models.ParticipantEmbed{
			Role: "member",
			UserInfo: models.ContactUserInfo{
				UserID:      userID,
				DisplayName: "Member",
				Username:    "member",
				Avatar:      "https://example.com/avatar.png",
			},
			JoinedAt: now,
		}
	}

	chat := models.Chat{
		ChatType:     "group",
		Name:         name,
		Description:  description,
		OwnerID:      creatorID,
		AdminIDs:     []bson.ObjectID{creatorID},
		Participants: participants,
		Stats: models.ChatStatsEmbed{
			ParticipantCount: len(participants),
			UnreadCount:      make(map[bson.ObjectID]int),
		},
		Settings: models.ChatSettingsEmbed{
			AllowInvites:     true,
			AllowFileSharing: true,
			MessageRetention: 0, // Forever
			MaxParticipants:  100,
		},
		ReadReceipts:  make(map[bson.ObjectID]time.Time),
		TypingUsers:   make(map[bson.ObjectID]time.Time),
		ActiveClients: make(map[string]*models.Client), // Initialize for real-time capabilities
		LastActivity:  now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	result, err := objects.DB.Collection(string(objects.ChatColl)).InsertOne(ctx, chat)
	if err != nil {
		return nil, fmt.Errorf("failed to create group chat: %w", err)
	}

	chat.ChatID = result.InsertedID.(bson.ObjectID)
	return &chat, nil
}

// SendMessage creates and persists a new message, then broadcasts it
func SendMessage(chatID, senderID bson.ObjectID, content, messageType string, attachments []models.AttachmentEmbed, mentions []string) (*models.Message, error) {
	ctx := context.Background()
	now := time.Now()

	// Get sender details
	sender, err := GetUserByID(senderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get sender: %w", err)
	}

	// Create message
	message := models.Message{
		ChatID:   chatID,
		SenderID: senderID,
		Sender: models.MessageSenderEmbed{
			UserID:      senderID,
			Username:    sender.Username,
			DisplayName: sender.Profile.DisplayName,
			Avatar:      sender.Profile.Avatar,
		},
		Content:       content,
		MessageType:   messageType,
		Attachments:   attachments,
		SearchContent: content, // TODO: Process for search optimization
		Status: models.MessageStatusEmbed{
			IsEdited:  false,
			IsDeleted: false,
			IsPinned:  false,
		},
		ReadBy:    make(map[string]time.Time),
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Handle mentions
	if len(mentions) > 0 {
		mentionUserIDs := make([]bson.ObjectID, 0, len(mentions))
		mentionUserNames := make([]string, 0, len(mentions))

		for _, mention := range mentions {
			if userID, err := bson.ObjectIDFromHex(mention); err == nil {
				mentionUserIDs = append(mentionUserIDs, userID)
				if user, err := GetUserByID(userID); err == nil {
					mentionUserNames = append(mentionUserNames, user.Username)
				}
			}
		}

		message.Mentions = models.MentionsEmbed{
			UserIDs:   mentionUserIDs,
			UserNames: mentionUserNames,
		}
	}

	// Insert message to database
	result, err := objects.DB.Collection(string(objects.MessageColl)).InsertOne(ctx, message)
	if err != nil {
		return nil, fmt.Errorf("failed to save message: %w", err)
	}

	message.ID = result.InsertedID.(bson.ObjectID)

	// Update chat's last message and stats
	updateChatLastMessage(chatID, message.ID, now)

	// Broadcast message in real-time
	broadcastNewMessage(chatID, message)

	return &message, nil
}

// UpdateTypingStatus updates typing indicator for a user in a chat
func UpdateTypingStatus(chatID, userID bson.ObjectID, isTyping bool) error {
	// Get user details
	user, err := GetUserByID(userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Create typing indicator message
	typingMsg := models.WebSocketMessage{
		Type:   models.WSMessageTypeTyping,
		ChatID: chatID.Hex(),
		UserID: userID.Hex(),
		Data: models.TypingIndicator{
			UserID:    userID.Hex(),
			Username:  user.Username,
			IsTyping:  isTyping,
			Timestamp: time.Now(),
		},
		Timestamp: time.Now(),
	}

	// Broadcast to chat room
	BroadcastToChat(chatID.Hex(), typingMsg)

	// Update typing users in chat document (with TTL)
	if isTyping {
		updateChatTypingStatus(chatID, userID, time.Now())
	}

	return nil
}

// MarkMessagesAsRead marks messages as read by a user
func MarkMessagesAsRead(chatID, userID bson.ObjectID, messageIDs []bson.ObjectID) error {
	ctx := context.Background()
	now := time.Now()

	// Update read receipts for messages
	filter := bson.M{
		"_id":    bson.M{"$in": messageIDs},
		"chatId": chatID,
	}

	update := bson.M{
		"$set": bson.M{
			"readBy." + userID.Hex(): now,
		},
	}

	_, err := objects.DB.Collection(string(objects.MessageColl)).UpdateMany(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update read receipts: %w", err)
	}

	// Update chat read receipts
	updateChatReadReceipts(chatID, userID, now)

	// Broadcast read receipts to other participants
	broadcastReadReceipts(chatID, userID, messageIDs, now)

	return nil
}

// ============ HELPER FUNCTIONS ============

// updateChatLastMessage updates the last message reference in chat
func updateChatLastMessage(chatID, messageID bson.ObjectID, timestamp time.Time) {
	ctx := context.Background()

	filter := bson.M{"_id": chatID}
	update := bson.M{
		"$set": bson.M{
			"lastMessageId": messageID,
			"updatedAt":     timestamp,
		},
		"$inc": bson.M{
			"stats.messageCount": 1,
		},
	}

	objects.DB.Collection(string(objects.ChatColl)).UpdateOne(ctx, filter, update)
}

// updateChatTypingStatus updates typing status in chat document
func updateChatTypingStatus(chatID, userID bson.ObjectID, timestamp time.Time) {
	ctx := context.Background()

	filter := bson.M{"_id": chatID}
	update := bson.M{
		"$set": bson.M{
			"typingUsers." + userID.Hex(): timestamp,
		},
	}

	objects.DB.Collection(string(objects.ChatColl)).UpdateOne(ctx, filter, update)

	// Remove typing status after 10 seconds
	time.AfterFunc(10*time.Second, func() {
		removeFilter := bson.M{"_id": chatID}
		removeUpdate := bson.M{
			"$unset": bson.M{
				"typingUsers." + userID.Hex(): "",
			},
		}
		objects.DB.Collection(string(objects.ChatColl)).UpdateOne(context.Background(), removeFilter, removeUpdate)
	})
}

// updateChatReadReceipts updates read receipts in chat document
func updateChatReadReceipts(chatID, userID bson.ObjectID, timestamp time.Time) {
	ctx := context.Background()

	filter := bson.M{"_id": chatID}
	update := bson.M{
		"$set": bson.M{
			"readReceipts." + userID.Hex(): timestamp,
		},
	}

	objects.DB.Collection(string(objects.ChatColl)).UpdateOne(ctx, filter, update)
}

// broadcastNewMessage broadcasts a new message to chat participants
func broadcastNewMessage(chatID bson.ObjectID, message models.Message) {
	wsMessage := models.WebSocketMessage{
		Type:   models.WSMessageTypeChat,
		ChatID: chatID.Hex(),
		UserID: message.SenderID.Hex(),
		Data: models.ChatMessage{
			ID:          message.ID.Hex(),
			ChatID:      chatID.Hex(),
			SenderID:    message.SenderID.Hex(),
			Content:     message.Content,
			MessageType: message.MessageType,
			Attachments: message.Attachments,
			CreatedAt:   message.CreatedAt,
		},
		Timestamp: message.CreatedAt,
	}

	BroadcastToChat(chatID.Hex(), wsMessage)
}

// broadcastReadReceipts broadcasts read receipt updates
func broadcastReadReceipts(chatID, userID bson.ObjectID, messageIDs []bson.ObjectID, timestamp time.Time) {
	messageIDStrings := make([]string, len(messageIDs))
	for i, id := range messageIDs {
		messageIDStrings[i] = id.Hex()
	}

	wsMessage := models.WebSocketMessage{
		Type:   models.WSMessageTypeDelivery,
		ChatID: chatID.Hex(),
		UserID: userID.Hex(),
		Data: map[string]interface{}{
			"messageIds": messageIDStrings,
			"status":     "read",
			"timestamp":  timestamp,
		},
		Timestamp: timestamp,
	}

	BroadcastToChat(chatID.Hex(), wsMessage)
}

// AddMessageReaction adds a reaction to a message
func AddMessageReaction(chatID, messageID, userID bson.ObjectID, emoji string) error {
	ctx := context.Background()

	// Validate emoji (basic validation)
	if len(emoji) == 0 || len(emoji) > 10 {
		return fmt.Errorf("invalid emoji")
	}

	// Update message with new reaction
	filter := bson.M{
		"_id":    messageID,
		"chatId": chatID,
	}

	update := bson.M{
		"$addToSet": bson.M{
			"reactions." + emoji + ".users": userID,
		},
		"$inc": bson.M{
			"reactions." + emoji + ".count": 1,
		},
		"$set": bson.M{
			"reactions." + emoji + ".details." + userID.Hex(): "", // Will be filled with username
			"updatedAt": time.Now(),
		},
	}

	result, err := objects.DB.Collection(string(objects.MessageColl)).UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to add reaction: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("message not found")
	}

	return nil
}

// RemoveMessageReaction removes a reaction from a message
func RemoveMessageReaction(chatID, messageID, userID bson.ObjectID, emoji string) error {
	ctx := context.Background()

	// Remove user from reaction
	filter := bson.M{
		"_id":    messageID,
		"chatId": chatID,
	}

	update := bson.M{
		"$pull": bson.M{
			"reactions." + emoji + ".users": userID,
		},
		"$inc": bson.M{
			"reactions." + emoji + ".count": -1,
		},
		"$unset": bson.M{
			"reactions." + emoji + ".details." + userID.Hex(): "",
		},
		"$set": bson.M{
			"updatedAt": time.Now(),
		},
	}

	result, err := objects.DB.Collection(string(objects.MessageColl)).UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to remove reaction: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("message not found")
	}

	// Clean up empty reaction if count is 0
	cleanupEmptyReaction(messageID, emoji)

	return nil
}

// cleanupEmptyReaction removes reaction entry if count reaches 0
func cleanupEmptyReaction(messageID bson.ObjectID, emoji string) {
	ctx := context.Background()

	filter := bson.M{
		"_id":                           messageID,
		"reactions." + emoji + ".count": bson.M{"$lte": 0},
	}

	update := bson.M{
		"$unset": bson.M{
			"reactions." + emoji: "",
		},
	}

	objects.DB.Collection(string(objects.MessageColl)).UpdateOne(ctx, filter, update)
}
