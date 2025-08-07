package services

import (
	"context"
	"fmt"
	"gin/internal/models"
	"gin/objects"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ============ ENHANCED CHAT SERVICE WITH REAL-TIME SUPPORT ============

// GetChat retrieves chat information with basic details
func GetChat(chatInfo models.GetContactInfo) (models.Chat, error) {
	chatFilter := bson.M{"_id": chatInfo.ChatID}
	chatProjection := bson.M{
		"_id":           1,
		"type":          1,
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
	chat, err := GetChat(models.GetContactInfo{ChatID: chatID})
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
		"type": "direct",
		"$or": []bson.M{
			{"participants." + userID1.Hex(): bson.M{"$exists": true}, "participants." + userID2.Hex(): bson.M{"$exists": true}},
		},
	}

	existingChat, err := FindByFilter[models.Chat](ctx, objects.DB.Collection(string(objects.ChatColl)), filter, nil)
	if err == nil {
		return &existingChat, nil
	}

	// Get user details for participants
	user1, err := GetUserByID(userID1)
	if err != nil {
		return nil, fmt.Errorf("failed to get user1: %w", err)
	}

	user2, err := GetUserByID(userID2)
	if err != nil {
		return nil, fmt.Errorf("failed to get user2: %w", err)
	}

	// Create new direct chat
	now := time.Now()
	chat := models.Chat{
		Type: "direct",
		Participants: map[string]models.ParticipantEmbed{
			userID1.Hex(): {
				UserID:      userID1,
				Username:    user1.Username,
				DisplayName: user1.Profile.DisplayName,
				Avatar:      user1.Profile.Avatar,
				Role:        "member",
				JoinedAt:    now,
				LastActive:  now,
			},
			userID2.Hex(): {
				UserID:      userID2,
				Username:    user2.Username,
				DisplayName: user2.Profile.DisplayName,
				Avatar:      user2.Profile.Avatar,
				Role:        "member",
				JoinedAt:    now,
				LastActive:  now,
			},
		},
		Stats: models.ChatStatsEmbed{
			ParticipantCount: 2,
			UnreadCount:      make(map[string]int),
		},
		Settings: models.ChatSettingsEmbed{
			AllowFileSharing: true,
			MessageRetention: 0, // Forever
			MaxParticipants:  2,
		},
		ReadReceipts: make(map[string]time.Time),
		TypingUsers:  make(map[string]time.Time),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	result, err := objects.DB.Collection(string(objects.ChatColl)).InsertOne(ctx, chat)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat: %w", err)
	}

	chat.ChatID = result.InsertedID.(bson.ObjectID)
	return &chat, nil
}

// CreateGroupChat creates a new group chat
func CreateGroupChat(creatorID bson.ObjectID, name, description string, participantIDs []bson.ObjectID) (*models.Chat, error) {
	ctx := context.Background()
	now := time.Now()

	// Get creator details
	creator, err := GetUserByID(creatorID)
	if err != nil {
		return nil, fmt.Errorf("failed to get creator: %w", err)
	}

	participants := make(map[string]models.ParticipantEmbed)

	// Add creator as owner
	participants[creatorID.Hex()] = models.ParticipantEmbed{
		UserID:      creatorID,
		Username:    creator.Username,
		DisplayName: creator.Profile.DisplayName,
		Avatar:      creator.Profile.Avatar,
		Role:        "owner",
		JoinedAt:    now,
		LastActive:  now,
	}

	// Add other participants
	for _, userID := range participantIDs {
		if userID == creatorID {
			continue // Skip creator, already added
		}

		user, err := GetUserByID(userID)
		if err != nil {
			continue // Skip invalid users
		}

		participants[userID.Hex()] = models.ParticipantEmbed{
			UserID:      userID,
			Username:    user.Username,
			DisplayName: user.Profile.DisplayName,
			Avatar:      user.Profile.Avatar,
			Role:        "member",
			JoinedAt:    now,
			LastActive:  now,
		}
	}

	chat := models.Chat{
		Type:         "group",
		Name:         name,
		Description:  description,
		OwnerID:      creatorID,
		AdminIDs:     []bson.ObjectID{creatorID},
		Participants: participants,
		Stats: models.ChatStatsEmbed{
			ParticipantCount: len(participants),
			UnreadCount:      make(map[string]int),
		},
		Settings: models.ChatSettingsEmbed{
			AllowInvites:     true,
			AllowFileSharing: true,
			MessageRetention: 0, // Forever
			MaxParticipants:  100,
		},
		ReadReceipts: make(map[string]time.Time),
		TypingUsers:  make(map[string]time.Time),
		CreatedAt:    now,
		UpdatedAt:    now,
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
	go updateChatLastMessage(chatID, message.ID, now)

	// Broadcast message in real-time
	go broadcastNewMessage(chatID, message)

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
		go updateChatTypingStatus(chatID, userID, time.Now())
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
	go updateChatReadReceipts(chatID, userID, now)

	// Broadcast read receipts to other participants
	go broadcastReadReceipts(chatID, userID, messageIDs, now)

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
