package services

import (
	"context"
	"encoding/hex"
	"fmt"
	"gin/internal/models"
	"gin/internal/utils"
	"gin/logger"
	"gin/objects"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ============ ENHANCED CHAT SERVICE WITH REAL-TIME SUPPORT ============

// GetUserChats retrieves all chats where the user is a participant
func GetUserChats(ctx context.Context, userID bson.ObjectID) ([]bson.ObjectID, error) {
	log := logger.WithContext(ctx)

	userFilter := bson.M{"_id": userID}
	userProjection := bson.M{
		"chats": 1,
	}

	user, err := FindByID[models.User](ctx, objects.DB.Collection(string(objects.UserColl)), userFilter, userProjection)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.WithField("user_id", userID.Hex()).Error("User have no contacts")
			return nil, fmt.Errorf("user have no contacts")
		}
		log.WithError(err).Error("Failed to get user")
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	chats := make([]bson.ObjectID, 0)

	for chatID := range user.Chats {
		chats = append(chats, chatID)
	}

	return chats, nil
}

func GetChat(ctx context.Context, chatInfo models.ChatInfo) (models.Chat, error) {
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

	var chat models.Chat

	err := objects.DB.Collection(string(objects.ChatColl)).FindOne(ctx, chatFilter, options.FindOne().SetProjection(chatProjection)).Decode(&chat)
	if err != nil {
		return chat, fmt.Errorf("failed to get chat: %w", err)
	}

	return chat, nil
}

func GetChatMessages(ctx context.Context, chatID bson.ObjectID, limit int, offset int) ([]models.Message, error) {
	messageFilter := bson.M{"chatId": chatID}

	messageProjection := bson.M{
		"_id":             1,
		"senderId":        1,
		"sender":          1,
		"content":         1,
		"messageType":     1,
		"thread.threadId": 1,
		"reactions":       1,
		"mentions":        1,
		"status":          1,
		"createdAt":       1,
		"updatedAt":       1,
	}

	findOpts := options.Find().
		SetProjection(messageProjection).
		SetSort(bson.M{"_id": -1}).
		SetSkip(int64(offset)).
		SetLimit(int64(limit))

	cursor, err := objects.DB.Collection(string(objects.MessageColl)).Find(ctx, messageFilter, findOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer cursor.Close(ctx)

	messages := make([]models.Message, 0)
	for cursor.Next(ctx) {
		var msg models.Message
		if err := cursor.Decode(&msg); err != nil {
			return nil, fmt.Errorf("failed to decode message: %w", err)
		}
		messages = append(messages, msg)
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	return messages, nil
}

// ============ CHAT MANAGEMENT ============
func CreateDirectChat(ctx context.Context, targetUserID bson.ObjectID) (*models.Chat, error) {
	user := ctx.Value(objects.UserDataKey).(models.LoginUserResponse)
	if user == (models.LoginUserResponse{}) {
		return nil, fmt.Errorf("invalid user data")
	}

	filter := bson.M{
		"chatType": "direct",
		"$and": []bson.M{
			{"participants." + user.ID.Hex(): bson.M{"$exists": true}},
			{"participants." + targetUserID.Hex(): bson.M{"$exists": true}},
		},
	}

	chatProjection := bson.M{
		"_id":          1,
		"chatType":     1,
		"participants": 1,
	}

	existingChat := models.Chat{}

	err := objects.DB.Collection(string(objects.ChatColl)).FindOne(ctx, filter, options.FindOne().SetProjection(chatProjection)).Decode(&existingChat)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			targetUser, err := GetUserByID(ctx, targetUserID)
			if err != nil {
				return nil, fmt.Errorf("failed to get target user: %w", err)
			}

			chat := utils.NewChatWithDefaults(user.ID, "direct")
			chat.Participants[user.ID] = models.ParticipantEmbed{
				Role: string(objects.ChatRoleMember),
				UserInfo: models.ContactUserInfo{
					UserID:      user.ID,
					DisplayName: user.Profile.DisplayName,
					Username:    user.Username,
					Avatar:      user.Profile.Avatar,
				},
			}
			chat.Participants[targetUserID] = models.ParticipantEmbed{
				Role: string(objects.ChatRoleMember),
				UserInfo: models.ContactUserInfo{
					UserID:      targetUserID,
					DisplayName: targetUser.Profile.DisplayName,
					Username:    targetUser.Username,
					Avatar:      targetUser.Profile.Avatar,
				},
			}

			_, err = objects.DB.Collection(string(objects.ChatColl)).InsertOne(ctx, chat)
			if err != nil {
				return nil, fmt.Errorf("failed to insert chat: %w", err)
			}

			userUpdateFilter := bson.M{
				"_id": bson.M{
					"$in": []bson.ObjectID{user.ID, targetUserID},
				},
			}

			userUpdate := bson.M{
				"$set": bson.M{
					"chats." + chat.ChatID.Hex(): 0,
				},
			}

			_, err = objects.DB.Collection(string(objects.UserColl)).UpdateMany(ctx, userUpdateFilter, userUpdate)
			if err != nil {
				return nil, fmt.Errorf("failed to update user: %w", err)
			}

			return &chat, nil
		}
		return nil, fmt.Errorf("failed to fetch existing chat: %w", err)
	}

	return &existingChat, nil
}

func CreateGroupChat(ctx context.Context, chatName, description string, participantIDs []bson.ObjectID) (*models.Chat, error) {
	creator := ctx.Value(objects.UserDataKey).(models.LoginUserResponse)
	if creator == (models.LoginUserResponse{}) {
		return nil, fmt.Errorf("invalid user data")
	}

	participants := make(map[bson.ObjectID]models.ParticipantEmbed)

	participants[creator.ID] = utils.NewParticipantWithDefaults(string(objects.StatusAccepted), string(objects.UserStatusOffline), creator.ID, []string{string(objects.ChatPermissionRead), string(objects.ChatPermissionWrite)}, models.ContactUserInfo{
		UserID:      creator.ID,
		DisplayName: creator.Profile.DisplayName,
		Username:    creator.Username,
		Avatar:      creator.Profile.Avatar,
	})

	users, err := GetUsersByIDs(ctx, participantIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get users: %w", err)
	}

	for _, u := range users {
		participants[u.ID] = utils.NewParticipantWithDefaults(string(objects.StatusPending), string(objects.UserStatusOffline), creator.ID, []string{string(objects.ChatPermissionRead), string(objects.ChatPermissionWrite)}, models.ContactUserInfo{
			UserID:      u.ID,
			DisplayName: u.Profile.DisplayName,
			Username:    u.Username,
			Avatar:      u.Profile.Avatar,
		})
	}

	chat := utils.NewChatWithDefaults(creator.ID, string(objects.ChatTypeGroup))
	chat.Participants = participants
	chat.Name = chatName
	chat.Description = description
	chat.Stats.ParticipantCount = len(participants)
	chat.Settings.MaxParticipants = 100

	// Create invite code
	invite, ok := utils.NewInviteCodeWithDefaults(creator, chat.ChatID, time.Now().Add(24*time.Hour))
	if !ok {
		return nil, fmt.Errorf("failed to create invite code")
	}
	chat.InviteCode = append(chat.InviteCode, invite)

	_, err = objects.DB.Collection(string(objects.ChatColl)).InsertOne(ctx, chat)
	if err != nil {
		return nil, fmt.Errorf("failed to create group chat: %w", err)
	}

	// Add chat ref to creator
	_, _ = objects.DB.Collection(string(objects.UserColl)).UpdateOne(ctx,
		bson.M{"_id": creator.ID},
		bson.M{"$set": bson.M{"chats." + chat.ChatID.Hex(): 0}},
	)

	// Send invites to participants (creator is the acting user sending invites)
	creatingGroup := true
	for _, u := range users {
		if u.ID == creator.ID {
			continue
		}
		if err := SendInviteToUser(ctx, creator.ID, invite.InviteCode, u.ID, creatingGroup); err != nil {
			return nil, fmt.Errorf("failed to send invite to user %s: %w", u.ID.Hex(), err)
		}
	}

	return &chat, nil
}

// SendMessage creates and persists a new message, then broadcasts it
func SendMessage(ctx context.Context, chatID, senderID bson.ObjectID, content, messageType string, attachments []models.AttachmentEmbed, mentions []string) (*models.Message, error) {
	now := time.Now()

	// Get sender details
	sender, err := GetUserByID(ctx, senderID)
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
				if user, err := GetUserByID(ctx, userID); err == nil {
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
func UpdateTypingStatus(ctx context.Context, chatID, userID bson.ObjectID, isTyping bool) error {
	// Get user details
	user, err := GetUserByID(ctx, userID)
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

// ============ BROADCASTING FUNCTIONS ============

// BroadcastToChat broadcasts a WebSocket message to all clients in a chat
func BroadcastToChat(chatID string, message models.WebSocketMessage) error {
	hub := GetHubInstance()
	if hub == nil {
		return fmt.Errorf("websocket hub not initialized")
	}

	// Use the enhanced hub's pub/sub manager to publish the message
	if hub.pubSubManager != nil {
		return hub.pubSubManager.PublishToChat(chatID, &message)
	}

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

func AddGroupMember(ctx context.Context, userID bson.ObjectID, chatID bson.ObjectID, participantIDs []bson.ObjectID) error {
	filter := bson.M{
		"_id":      chatID,
		"chatType": string(objects.ChatTypeGroup),
	}

	count, err := objects.DB.Collection(string(objects.ChatColl)).CountDocuments(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to get chat: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("%w", ErrChatNotFound)
	}

	users, err := GetUsersByIDs(ctx, participantIDs)
	if err != nil {
		return fmt.Errorf("failed to get users: %w", err)
	}

	// Build per-participant $set entries (merge, not replace)
	setEntries := bson.M{"updatedAt": time.Now()}
	for _, u := range users {
		setEntries["participants."+u.ID.Hex()] = utils.NewParticipantWithDefaults(
			string(objects.StatusPending),
			string(objects.UserStatusOffline),
			userID,
			[]string{string(objects.ChatPermissionRead), string(objects.ChatPermissionWrite)},
			models.ContactUserInfo{
				UserID:      u.ID,
				DisplayName: u.Profile.DisplayName,
				Username:    u.Username,
				Avatar:      u.Profile.Avatar,
			},
		)
	}

	_, err = objects.DB.Collection(string(objects.ChatColl)).UpdateOne(ctx, filter, bson.M{"$set": setEntries})
	if err != nil {
		return fmt.Errorf("failed to update chat: %w", err)
	}

	return nil
}

// RemoveGroupMember removes one or more participants from a group chat.
// The acting user must be an owner or admin of the chat.
func RemoveGroupMember(ctx context.Context, actingUserID bson.ObjectID, chatID bson.ObjectID, targetUserIDs []bson.ObjectID) error {

	// Verify chat exists and is a group
	chatFilter := bson.M{
		"_id":      chatID,
		"chatType": string(objects.ChatTypeGroup),
	}

	var chat models.Chat
	err := objects.DB.Collection(string(objects.ChatColl)).FindOne(ctx, chatFilter, options.FindOne().SetProjection(bson.M{
		"participants": 1,
		"ownerId":      1,
		"adminIds":     1,
	})).Decode(&chat)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return fmt.Errorf("%w", ErrChatNotFound)
		}
		return fmt.Errorf("failed to get chat: %w", err)
	}

	// Verify acting user is a participant
	actingParticipant, ok := chat.Participants[actingUserID]
	if !ok {
		return fmt.Errorf("%w", ErrNotParticipant)
	}

	// Must be owner or admin
	if actingParticipant.Role != string(objects.ChatRoleOwner) && actingParticipant.Role != string(objects.ChatRoleAdmin) {
		return fmt.Errorf("%w: only owners and admins can remove members", ErrPermissionDenied)
	}

	// Build $unset entries for participants to remove
	unsetEntries := bson.M{}
	userFilter := bson.M{"_id": bson.M{"$in": targetUserIDs}}
	userUpdate := bson.M{"$unset": bson.M{}}

	for _, targetID := range targetUserIDs {
		if _, isParticipant := chat.Participants[targetID]; !isParticipant {
			continue // Skip users not in the chat
		}
		if targetID == actingUserID {
			continue // Cannot remove self
		}
		unsetEntries["participants."+targetID.Hex()] = ""
		userUpdate["$unset"].(bson.M)["chats."+chatID.Hex()] = ""
	}

	if len(unsetEntries) == 0 {
		return fmt.Errorf("%w", ErrNoValidParticipants)
	}

	// Update chat: remove participants + decrement count
	chatUpdate := bson.M{
		"$unset": unsetEntries,
		"$inc": bson.M{
			"stats.participantCount": -len(unsetEntries),
		},
		"$set": bson.M{
			"updatedAt": time.Now(),
		},
	}

	_, err = objects.DB.Collection(string(objects.ChatColl)).UpdateOne(ctx, bson.M{"_id": chatID}, chatUpdate)
	if err != nil {
		return fmt.Errorf("failed to update chat: %w", err)
	}

	// Remove chat ref from each removed user
	_, err = objects.DB.Collection(string(objects.UserColl)).UpdateMany(ctx, userFilter, userUpdate)
	if err != nil {
		return fmt.Errorf("failed to update users: %w", err)
	}

	return nil
}

// ================ Invites ================
func CreateInvite(ctx context.Context, userID bson.ObjectID, chatID bson.ObjectID) (inviteCode string, err error) {
	filter := bson.M{
		"_id":      chatID,
		"chatType": string(objects.ChatTypeGroup),
		"$and": []bson.M{
			{"participants." + userID.Hex(): bson.M{"$exists": true}},
		},
	}

	count, err := objects.DB.Collection(string(objects.ChatColl)).CountDocuments(ctx, filter)
	if err != nil {
		return "", fmt.Errorf("failed to get chat: %w", err)
	}
	if count == 0 {
		return "", fmt.Errorf("%w", ErrChatNotFound)
	}

	user, ok := ctx.Value(objects.UserDataKey).(models.LoginUserResponse)
	if !ok {
		return "", fmt.Errorf("user not found")
	}

	invite, ok := utils.NewInviteCodeWithDefaults(user, chatID, time.Now().Add(24*time.Hour))
	if !ok {
		return "", fmt.Errorf("failed to create invite code")
	}

	update := bson.M{
		"$push": bson.M{
			"inviteCode": invite,
		},
	}

	_, err = objects.DB.Collection(string(objects.ChatColl)).UpdateOne(ctx, filter, update)
	if err != nil {
		return "", fmt.Errorf("failed to create invite code: %w", err)
	}

	return invite.InviteCode, nil
}

func GetGroupInvites(ctx context.Context, userID bson.ObjectID, chatID bson.ObjectID) ([]models.InviteCodeEmbed, error) {
	filter := bson.M{
		"_id": chatID,
		"$and": []bson.M{
			{"participants." + userID.Hex(): bson.M{"$exists": true}},
		},
	}

	chat := models.Chat{}

	err := objects.DB.Collection(string(objects.ChatColl)).FindOne(ctx, filter, options.FindOne().SetProjection(bson.M{"inviteCode": 1})).Decode(&chat)
	if err != nil {
		return nil, fmt.Errorf("failed to get group invites: %w", err)
	}

	return chat.InviteCode, nil
}

func DeleteInvite(ctx context.Context, userID bson.ObjectID, inviteID string) error {
	inviteIDBytes, err := hex.DecodeString(inviteID)
	if err != nil {
		return fmt.Errorf("failed to decode invite ID: %w", err)
	}

	inviteInfo, err := utils.Decrypt(inviteIDBytes)
	if err != nil {
		return fmt.Errorf("failed to decrypt invite ID: %w", err)
	}

	chatIDStr := strings.Split(inviteInfo, "_")[0]
	chatID, err := bson.ObjectIDFromHex(chatIDStr)
	if err != nil {
		return fmt.Errorf("failed to parse chat ID from invite: %w", err)
	}

	filter := bson.M{
		"_id": chatID,
		"$and": []bson.M{
			{"participants." + userID.Hex(): bson.M{"$exists": true}},
			{"inviteCode.inviteCode": inviteID},
		},
	}

	update := bson.M{
		"$set": bson.M{
			"inviteCode.$.deleted": true,
		},
	}

	_, err = objects.DB.Collection(string(objects.ChatColl)).UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to delete invite: %w", err)
	}

	return nil
}

func UpdateInviteStatus(ctx context.Context, chatID bson.ObjectID, inviteID string, status string) error {
	log := logger.WithContext(ctx)

	filter := bson.M{
		"_id": chatID,
		"$and": []bson.M{
			{"inviteCode.inviteCode": inviteID},
		},
	}

	update := bson.M{
		"$set": bson.M{
			"inviteCode.$.status": status,
		},
	}

	_, err := objects.DB.Collection(string(objects.ChatColl)).UpdateOne(ctx, filter, update)
	if err != nil {
		log.WithError(err).Error("failed to update invite status")
		return fmt.Errorf("failed to update invite status: %w", err)
	}

	return nil
}

func GetJoinedUsersByInvite(ctx context.Context, userID bson.ObjectID, inviteID string) ([]bson.ObjectID, error) {
	inviteIDBytes, err := hex.DecodeString(inviteID)
	if err != nil {
		return nil, fmt.Errorf("failed to decode invite ID: %w", err)
	}

	inviteInfo, err := utils.Decrypt(inviteIDBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt invite ID: %w", err)
	}

	chatIDStr := strings.Split(inviteInfo, "_")[0]
	chatID, err := bson.ObjectIDFromHex(chatIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse chat ID from invite: %w", err)
	}

	filter := bson.M{
		"_id": chatID,
		"$and": []bson.M{
			{"participants." + userID.Hex(): bson.M{"$exists": true}},
			{"inviteCode.inviteCode": inviteID},
		},
	}

	chat := models.Chat{}

	err = objects.DB.Collection(string(objects.ChatColl)).FindOne(ctx, filter, options.FindOne().SetProjection(bson.M{"inviteCode": 1})).Decode(&chat)
	if err != nil {
		return nil, fmt.Errorf("failed to get joined users by invite: %w", err)
	}

	if len(chat.InviteCode) == 0 {
		return []bson.ObjectID{}, nil
	}

	return chat.InviteCode[0].UserIDs, nil
}

func SendInviteToUser(ctx context.Context, actingUserID bson.ObjectID, inviteID string, targetUserID bson.ObjectID, creatingGroup bool) error {
	inviteIDBytes, err := hex.DecodeString(inviteID)
	if err != nil {
		return fmt.Errorf("failed to decode invite ID: %w", err)
	}

	inviteInfo, err := utils.Decrypt(inviteIDBytes)
	if err != nil {
		return fmt.Errorf("failed to decrypt invite ID: %w", err)
	}

	inviteInfoSplit := strings.Split(inviteInfo, "_")
	expireTime, err := time.Parse(time.RFC3339, inviteInfoSplit[3])
	if err != nil {
		return fmt.Errorf("failed to parse expire time: %w", err)
	}

	if expireTime.Before(time.Now()) {
		return fmt.Errorf("%w", ErrInvalidInvite)
	}

	chatIDStr := inviteInfoSplit[0]
	chatID, err := bson.ObjectIDFromHex(chatIDStr)
	if err != nil {
		return fmt.Errorf("failed to parse chat ID from invite: %w", err)
	}

	filter := bson.M{
		"_id": chatID,
		"$and": []bson.M{
			{"participants." + targetUserID.Hex(): bson.M{"$exists": false}},
			{"participants." + actingUserID.Hex(): bson.M{"$exists": true}},
			{"inviteCode.inviteCode": inviteID},
		},
	}

	if !creatingGroup {
		cntDoc, err := objects.DB.Collection(string(objects.ChatColl)).CountDocuments(ctx, filter)
		if err != nil {
			return fmt.Errorf("failed to get joined users by invite: %w", err)
		}

		if cntDoc == 0 {
			return fmt.Errorf("user you do not have permission to send invite or user already in the chat")
		}
	}

	targetUserProjection := bson.M{
		"$push": bson.M{
			"chatInvitations": models.ChatInvitationEmbed{
				Token:  inviteID,
				Status: "pending",
			},
		},
	}

	_, err = objects.DB.Collection(string(objects.UserColl)).UpdateOne(ctx, bson.M{"_id": targetUserID}, targetUserProjection)
	if err != nil {
		return fmt.Errorf("failed to update target user: %w", err)
	}

	return nil
}

func JoinGroupByInvite(ctx context.Context, userID bson.ObjectID, inviteID string) error {
	inviteIDBytes, err := hex.DecodeString(inviteID)
	if err != nil {
		return fmt.Errorf("failed to decode invite ID: %w", err)
	}

	inviteInfo, err := utils.Decrypt(inviteIDBytes)
	if err != nil {
		return fmt.Errorf("failed to decrypt invite ID: %w", err)
	}

	inviteInfoSplit := strings.Split(inviteInfo, "_")

	chatIDStr := inviteInfoSplit[0]
	chatObjectID, err := bson.ObjectIDFromHex(chatIDStr)
	if err != nil {
		return fmt.Errorf("failed to decode chat ID: %w", err)
	}

	expireTime, err := time.Parse(time.RFC3339, inviteInfoSplit[3])
	if err != nil {
		return fmt.Errorf("failed to parse expire time: %w", err)
	}

	if expireTime.Before(time.Now()) || expireTime.After(time.Now().Add(24*time.Hour)) {
		return fmt.Errorf("%w", ErrInvalidInvite)
	}

	// Check if user already in chat
	count, err := objects.DB.Collection(string(objects.ChatColl)).CountDocuments(ctx,
		bson.M{"_id": chatObjectID, "participants." + userID.Hex(): bson.M{"$exists": true}},
	)
	if err != nil {
		return fmt.Errorf("failed to check if user is in the chat: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("user already in the chat")
	}

	user, err := GetUserBasicInfo(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	userParticipant := utils.NewParticipantWithDefaults(
		string(objects.StatusAccepted),
		string(objects.UserStatusOffline),
		userID,
		[]string{string(objects.ChatPermissionRead), string(objects.ChatPermissionWrite)},
		models.ContactUserInfo{
			UserID:      userID,
			DisplayName: user.Profile.DisplayName,
			Username:    user.Username,
			Avatar:      user.Profile.Avatar,
		},
	)

	updateInviteCodeUserIDs := bson.M{
		"$set": bson.M{
			"participants." + userID.Hex(): userParticipant,
		},
		"$push": bson.M{
			"inviteCode.$.userIDs": userID,
		},
	}

	_, err = objects.DB.Collection(string(objects.ChatColl)).UpdateOne(ctx,
		bson.M{"_id": chatObjectID, "inviteCode.inviteCode": inviteID},
		updateInviteCodeUserIDs,
	)
	if err != nil {
		return fmt.Errorf("failed to update invite code user IDs: %w", err)
	}

	// Add chat ref to user
	_, _ = objects.DB.Collection(string(objects.UserColl)).UpdateOne(ctx,
		bson.M{"_id": userID},
		bson.M{"$set": bson.M{"chats." + chatObjectID.Hex(): 0}},
	)

	return nil
}

func GetAllInvitesOfUser(ctx context.Context, userID bson.ObjectID) ([]models.ChatInvitationEmbed, error) {
	var result struct {
		ChatInvitations []models.ChatInvitationEmbed `bson:"chatInvitations"`
	}

	err := objects.DB.Collection(string(objects.UserColl)).FindOne(ctx,
		bson.M{"_id": userID},
		options.FindOne().SetProjection(bson.M{"chatInvitations": 1}),
	).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return []models.ChatInvitationEmbed{}, nil
		}
		return nil, fmt.Errorf("failed to get all invites of user: %w", err)
	}

	return result.ChatInvitations, nil
}
