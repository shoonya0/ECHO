package services

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/logger"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ============ ENHANCED CHAT SERVICE WITH REAL-TIME SUPPORT ============

// GetUserChats returns the chats the hub subscribes userID to: the chats in the
// user's chats list where the user is also a participant. The list alone isn't
// trusted: before #9 was fixed, a forged invite code added a group to it
// without making the user a participant, and those entries are still stored.
func GetUserChats(ctx context.Context, userID bson.ObjectID) ([]bson.ObjectID, error) {
	log := logger.WithContext(ctx)

	userFilter := bson.M{"_id": userID}
	userProjection := bson.M{
		"chats": 1,
	}

	user, err := FindByID[models.User](ctx, database.DB.Collection(string(constants.UserColl)), userFilter, userProjection)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.WithField("user_id", userID.Hex()).Error("User have no contacts")
			return nil, fmt.Errorf("user have no contacts")
		}
		log.WithError(err).Error("Failed to get user")
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	listed := make([]bson.ObjectID, 0, len(user.Chats))
	for chatID := range user.Chats {
		listed = append(listed, chatID)
	}
	if len(listed) == 0 {
		return listed, nil
	}

	cursor, err := database.DB.Collection(string(constants.ChatColl)).Find(ctx,
		bson.M{"_id": bson.M{"$in": listed}, participantField(userID): bson.M{"$exists": true}},
		options.Find().SetProjection(bson.M{"_id": 1}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find user chats: %w", err)
	}
	var found []struct {
		ID bson.ObjectID `bson:"_id"`
	}
	if err := cursor.All(ctx, &found); err != nil {
		return nil, fmt.Errorf("failed to decode user chats: %w", err)
	}

	chats := make([]bson.ObjectID, 0, len(found))
	for _, c := range found {
		chats = append(chats, c.ID)
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

	err := database.DB.Collection(string(constants.ChatColl)).FindOne(ctx, chatFilter, options.FindOne().SetProjection(chatProjection)).Decode(&chat)
	if errors.Is(err, mongo.ErrNoDocuments) {
		// Callers tell "no such chat" from a failed lookup by this error (#83).
		return chat, ErrChatNotFound
	}
	if err != nil {
		return chat, fmt.Errorf("failed to get chat: %w", err)
	}

	return chat, nil
}

// messagePageFilter matches a chat's messages older than the before cursor,
// or its newest messages when before is nil.
func messagePageFilter(chatID bson.ObjectID, before *bson.ObjectID) bson.M {
	filter := bson.M{"chatId": chatID}
	if before != nil {
		filter["_id"] = bson.M{"$lt": *before}
	}
	return filter
}

// trimMessagePage cuts a result fetched with limit+1 down to limit and
// reports whether older messages remain.
func trimMessagePage(messages []models.Message, limit int) ([]models.Message, bool) {
	if len(messages) > limit {
		return messages[:limit], true
	}
	return messages, false
}

// participantField is the key of userID's entry in a chat's participants map.
func participantField(userID bson.ObjectID) string {
	return "participants." + userID.Hex()
}

// chatMemberFilter matches the chat only when userID is one of its participants.
func chatMemberFilter(chatID, userID bson.ObjectID) bson.M {
	return bson.M{"_id": chatID, participantField(userID): bson.M{"$exists": true}}
}

// RequireChatMember returns ErrNotParticipant unless userID is a participant of
// the chat. An unknown chat gives the same error, so chat IDs can't be probed.
func RequireChatMember(ctx context.Context, chatID, userID bson.ObjectID) error {
	count, err := database.DB.Collection(string(constants.ChatColl)).CountDocuments(ctx, chatMemberFilter(chatID, userID))
	if err != nil {
		return fmt.Errorf("failed to check chat membership: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("%w", ErrNotParticipant)
	}
	return nil
}

// GetChatMessages returns up to limit messages of a chat, newest first,
// starting just before the before cursor (a message ID; nil for the newest
// page). hasMore reports whether older messages exist; pass the last
// returned message's ID as before to fetch them. It returns ErrChatNotFound
// when userID is not a participant, so chat IDs can't be probed.
func GetChatMessages(ctx context.Context, userID bson.ObjectID, chatID bson.ObjectID, limit int, before *bson.ObjectID) (messages []models.Message, hasMore bool, err error) {
	if err := RequireChatMember(ctx, chatID, userID); err != nil {
		if errors.Is(err, ErrNotParticipant) {
			return nil, false, fmt.Errorf("%w", ErrChatNotFound)
		}
		return nil, false, err
	}

	messageFilter := messagePageFilter(chatID, before)

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
		SetLimit(int64(limit) + 1) // one extra row tells us whether more remain

	cursor, err := database.DB.Collection(string(constants.MessageColl)).Find(ctx, messageFilter, findOpts)
	if err != nil {
		return nil, false, fmt.Errorf("failed to query messages: %w", err)
	}
	defer cursor.Close(ctx)

	messages = make([]models.Message, 0, limit+1)
	for cursor.Next(ctx) {
		var msg models.Message
		if err := cursor.Decode(&msg); err != nil {
			return nil, false, fmt.Errorf("failed to decode message: %w", err)
		}
		messages = append(messages, msg)
	}
	if err := cursor.Err(); err != nil {
		return nil, false, fmt.Errorf("cursor error: %w", err)
	}

	messages, hasMore = trimMessagePage(messages, limit)
	return messages, hasMore, nil
}

// ============ CHAT MANAGEMENT ============
func CreateDirectChat(ctx context.Context, targetUserID bson.ObjectID) (*models.Chat, error) {
	user := ctx.Value(constants.UserDataKey).(models.LoginUserResponse)
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

	err := database.DB.Collection(string(constants.ChatColl)).FindOne(ctx, filter, options.FindOne().SetProjection(chatProjection)).Decode(&existingChat)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			targetUser, err := GetUserByID(ctx, targetUserID)
			if err != nil {
				return nil, fmt.Errorf("failed to get target user: %w", err)
			}

			chat := utils.NewChatWithDefaults(user.ID, "direct")
			chat.Participants[user.ID] = models.ParticipantEmbed{
				Role: string(constants.ChatRoleMember),
				UserInfo: models.ContactUserInfo{
					UserID:      user.ID,
					DisplayName: user.Profile.DisplayName,
					Username:    user.Username,
					Avatar:      user.Profile.Avatar,
				},
			}
			chat.Participants[targetUserID] = models.ParticipantEmbed{
				Role: string(constants.ChatRoleMember),
				UserInfo: models.ContactUserInfo{
					UserID:      targetUserID,
					DisplayName: targetUser.Profile.DisplayName,
					Username:    targetUser.Username,
					Avatar:      targetUser.Profile.Avatar,
				},
			}

			_, err = database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, chat)
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

			_, err = database.DB.Collection(string(constants.UserColl)).UpdateMany(ctx, userUpdateFilter, userUpdate)
			if err != nil {
				return nil, fmt.Errorf("failed to update user: %w", err)
			}

			return &chat, nil
		}
		return nil, fmt.Errorf("failed to fetch existing chat: %w", err)
	}

	return &existingChat, nil
}

// newGroupParticipants builds the participants of a new group: the creator as the
// accepted owner, every invitee as a pending member. A creator who also lists
// themselves as an invitee stays owner.
func newGroupParticipants(creator models.LoginUserResponse, invitees []models.LoginUserResponse) map[bson.ObjectID]models.ParticipantEmbed {
	perms := []string{string(constants.ChatPermissionRead), string(constants.ChatPermissionWrite)}
	participants := make(map[bson.ObjectID]models.ParticipantEmbed, len(invitees)+1)

	participants[creator.ID] = utils.NewParticipantWithDefaults(string(constants.ChatRoleOwner), string(constants.StatusAccepted), string(constants.UserStatusOffline), creator.ID, perms, models.ContactUserInfo{
		UserID:      creator.ID,
		DisplayName: creator.Profile.DisplayName,
		Username:    creator.Username,
		Avatar:      creator.Profile.Avatar,
	})

	for _, u := range invitees {
		if u.ID == creator.ID {
			continue
		}
		participants[u.ID] = utils.NewParticipantWithDefaults(string(constants.ChatRoleMember), string(constants.StatusPending), string(constants.UserStatusOffline), creator.ID, perms, models.ContactUserInfo{
			UserID:      u.ID,
			DisplayName: u.Profile.DisplayName,
			Username:    u.Username,
			Avatar:      u.Profile.Avatar,
		})
	}
	return participants
}

func CreateGroupChat(ctx context.Context, chatName, description string, participantIDs []bson.ObjectID) (*models.Chat, error) {
	creator := ctx.Value(constants.UserDataKey).(models.LoginUserResponse)
	if creator == (models.LoginUserResponse{}) {
		return nil, fmt.Errorf("invalid user data")
	}

	users, err := GetUsersByIDs(ctx, participantIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get users: %w", err)
	}

	participants := newGroupParticipants(creator, users)

	chat := utils.NewChatWithDefaults(creator.ID, string(constants.ChatTypeGroup))
	chat.Participants = participants
	chat.Name = chatName
	chat.Description = description
	chat.Stats.ParticipantCount = len(participants)
	chat.Settings.MaxParticipants = 100

	// Create invite code
	invite, ok := utils.NewInviteCodeWithDefaults(time.Now().Add(24 * time.Hour))
	if !ok {
		return nil, fmt.Errorf("failed to create invite code")
	}
	chat.InviteCode = append(chat.InviteCode, invite)

	_, err = database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, chat)
	if err != nil {
		return nil, fmt.Errorf("failed to create group chat: %w", err)
	}

	// Add chat ref to creator
	_, _ = database.DB.Collection(string(constants.UserColl)).UpdateOne(ctx,
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
		// Both maps must be stored as {}, not null: updates create paths inside them
		// such as "reactions.👍.count", which MongoDB refuses inside null (#72).
		Reactions: make(map[string]models.ReactionEmbed),
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
	result, err := database.DB.Collection(string(constants.MessageColl)).InsertOne(ctx, message)
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
	if err := RequireChatMember(ctx, chatID, userID); err != nil {
		return err
	}

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

// MarkMessagesAsRead marks messages as read by a user. IDs that aren't messages
// of this chat are skipped; it returns ErrMessageNotFound when none are left.
func MarkMessagesAsRead(chatID, userID bson.ObjectID, messageIDs []bson.ObjectID) error {
	ctx := context.Background()
	now := time.Now()

	if err := RequireChatMember(ctx, chatID, userID); err != nil {
		return err
	}

	// Update read receipts for messages
	filter := bson.M{
		"_id":    bson.M{"$in": messageIDs},
		"chatId": chatID,
	}

	messages := database.DB.Collection(string(constants.MessageColl))

	cursor, err := messages.Find(ctx, filter, options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return fmt.Errorf("failed to find messages: %w", err)
	}
	var found []struct {
		ID bson.ObjectID `bson:"_id"`
	}
	if err := cursor.All(ctx, &found); err != nil {
		return fmt.Errorf("failed to read messages: %w", err)
	}
	if len(found) == 0 {
		return fmt.Errorf("%w", ErrMessageNotFound)
	}

	chatMessageIDs := make([]bson.ObjectID, len(found))
	for i, message := range found {
		chatMessageIDs[i] = message.ID
	}

	update := bson.M{
		"$set": bson.M{
			"readBy." + userID.Hex(): now,
		},
	}

	if _, err := messages.UpdateMany(ctx, filter, update); err != nil {
		return fmt.Errorf("failed to update read receipts: %w", err)
	}

	// Update chat read receipts
	updateChatReadReceipts(chatID, userID, now)

	// Broadcast read receipts to other participants
	broadcastReadReceipts(chatID, userID, chatMessageIDs, now)

	return nil
}

// ============ BROADCASTING FUNCTIONS ============

// Broadcaster delivers a WebSocket event to every client in a chat.
// The realtime hub registers itself via SetBroadcaster at startup; services
// cannot import the hub directly because the hub depends on services.
type Broadcaster interface {
	PublishToChat(chatID string, message *models.WebSocketMessage) error
}

var broadcaster Broadcaster

// SetBroadcaster wires the realtime hub into the service layer.
func SetBroadcaster(b Broadcaster) { broadcaster = b }

// BroadcastToChat broadcasts a WebSocket message to all clients in a chat
func BroadcastToChat(chatID string, message models.WebSocketMessage) error {
	if broadcaster == nil {
		return fmt.Errorf("websocket hub not initialized")
	}
	return broadcaster.PublishToChat(chatID, &message)
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

	database.DB.Collection(string(constants.ChatColl)).UpdateOne(ctx, filter, update)
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

	database.DB.Collection(string(constants.ChatColl)).UpdateOne(ctx, filter, update)

	// Remove typing status after 10 seconds
	time.AfterFunc(10*time.Second, func() {
		removeFilter := bson.M{"_id": chatID}
		removeUpdate := bson.M{
			"$unset": bson.M{
				"typingUsers." + userID.Hex(): "",
			},
		}
		database.DB.Collection(string(constants.ChatColl)).UpdateOne(context.Background(), removeFilter, removeUpdate)
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

	database.DB.Collection(string(constants.ChatColl)).UpdateOne(ctx, filter, update)
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

// maxEmojiBytes caps a reaction's emoji. The longest standard emoji, a kiss with two
// skin tones such as 🧑🏻‍❤️‍💋‍🧑🏼, is 35 bytes.
const maxEmojiBytes = 64

// validateEmoji refuses an emoji that isn't safe as one field name in
// "reactions.<emoji>.users": a "." would split the path and reach other fields
// (#58), "$" marks operators, and NUL or invalid UTF-8 can't be stored as a key.
// It also refuses anything that isn't made of emoji characters, such as letters,
// spaces, line breaks or a right-to-left override, because every member's app shows
// the value. It doesn't check that the value is exactly one emoji: "👍👍" passes.
func validateEmoji(emoji string) error {
	if emoji == "" || len(emoji) > maxEmojiBytes || !utf8.ValidString(emoji) ||
		strings.ContainsAny(emoji, ".$\x00") {
		return fmt.Errorf("%w", ErrInvalidEmoji)
	}
	for _, r := range emoji {
		if !isEmojiRune(r) {
			return fmt.Errorf("%w", ErrInvalidEmoji)
		}
	}
	return nil
}

// isEmojiRune reports whether r can be part of an emoji.
func isEmojiRune(r rune) bool {
	switch {
	case r >= 0x1F000 && r <= 0x1FAFF:
		// The emoji blocks, including emoji newer than Go's Unicode tables.
		return true
	case r >= 0xE0020 && r <= 0xE007F:
		// Tag characters, which spell subdivision flags such as 🏴󠁧󠁢󠁳󠁣󠁴󠁿.
		return true
	case unicode.In(r, unicode.So, unicode.Sm, unicode.Sk):
		// Older emoji such as ❤ and ↔ are symbols.
		return true
	}
	switch r {
	case '#', '*', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', // keycap bases: 1️⃣
		'\u200d',           // zero-width joiner: 👩‍💻
		'\u20e3',           // keycap
		'\ufe0e', '\ufe0f', // text or emoji style
		'‼', '⁉', 'ℹ', '〰', '〽': // emoji that Unicode doesn't class as symbols
		return true
	}
	return false
}

// AddMessageReaction adds a reaction to a message
func AddMessageReaction(chatID, messageID, userID bson.ObjectID, emoji string) error {
	ctx := context.Background()

	if err := validateEmoji(emoji); err != nil {
		return err
	}

	if err := RequireChatMember(ctx, chatID, userID); err != nil {
		return err
	}

	if err := repairNullReactions(ctx, chatID, messageID); err != nil {
		return err
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

	result, err := database.DB.Collection(string(constants.MessageColl)).UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to add reaction: %w", err)
	}

	if result.MatchedCount == 0 {
		return ErrMessageNotFound
	}

	return nil
}

// RemoveMessageReaction removes a reaction from a message
func RemoveMessageReaction(chatID, messageID, userID bson.ObjectID, emoji string) error {
	ctx := context.Background()

	if err := validateEmoji(emoji); err != nil {
		return err
	}

	if err := RequireChatMember(ctx, chatID, userID); err != nil {
		return err
	}

	if err := repairNullReactions(ctx, chatID, messageID); err != nil {
		return err
	}

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

	result, err := database.DB.Collection(string(constants.MessageColl)).UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to remove reaction: %w", err)
	}

	if result.MatchedCount == 0 {
		return ErrMessageNotFound
	}

	// Clean up empty reaction if count is 0
	cleanupEmptyReaction(messageID, emoji)

	return nil
}

// repairNullReactions sets a message's reactions to {} when it is null, so the
// reaction paths can be created in it. Every message saved before #72 has null there,
// and so does one written by an older server copy during a deploy, which a startup
// migration would miss. Matching nothing is normal: the message is already fine, or
// the reaction update that follows reports that it doesn't exist.
func repairNullReactions(ctx context.Context, chatID, messageID bson.ObjectID) error {
	_, err := database.DB.Collection(string(constants.MessageColl)).UpdateOne(ctx,
		bson.M{"_id": messageID, "chatId": chatID, "reactions": bson.M{"$type": "null"}},
		bson.M{"$set": bson.M{"reactions": bson.M{}}},
	)
	if err != nil {
		return fmt.Errorf("failed to prepare reactions: %w", err)
	}
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

	database.DB.Collection(string(constants.MessageColl)).UpdateOne(ctx, filter, update)
}

// AddGroupMember adds users to a group chat as pending members. The acting
// user must be an owner or admin of the chat. Users who are already
// participants are left untouched, so their role and status are kept.
func AddGroupMember(ctx context.Context, userID bson.ObjectID, chatID bson.ObjectID, participantIDs []bson.ObjectID) error {
	filter := bson.M{
		"_id":      chatID,
		"chatType": string(constants.ChatTypeGroup),
	}

	var chat models.Chat
	err := database.DB.Collection(string(constants.ChatColl)).FindOne(ctx, filter, options.FindOne().SetProjection(bson.M{
		"participants": 1,
	})).Decode(&chat)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return fmt.Errorf("%w", ErrChatNotFound)
		}
		return fmt.Errorf("failed to get chat: %w", err)
	}

	actingParticipant, ok := chat.Participants[userID]
	if !ok {
		return fmt.Errorf("%w", ErrNotParticipant)
	}
	if actingParticipant.Role != string(constants.ChatRoleOwner) && actingParticipant.Role != string(constants.ChatRoleAdmin) {
		return fmt.Errorf("%w: only owners and admins can add members", ErrPermissionDenied)
	}

	var newIDs []bson.ObjectID
	for _, id := range participantIDs {
		if _, exists := chat.Participants[id]; !exists {
			newIDs = append(newIDs, id)
		}
	}
	if len(newIDs) == 0 {
		return fmt.Errorf("%w", ErrAlreadyParticipant)
	}

	users, err := GetUsersByIDs(ctx, newIDs)
	if err != nil {
		return fmt.Errorf("failed to get users: %w", err)
	}
	if len(users) == 0 {
		return fmt.Errorf("%w", ErrUserNotFound)
	}

	// Re-check the acting user's role and the targets' absence in the update
	// itself, so a concurrent demotion or add between the read and the write
	// can't be overridden.
	updateFilter := bson.M{
		"_id":      chatID,
		"chatType": string(constants.ChatTypeGroup),
		"participants." + userID.Hex() + ".role": bson.M{"$in": []string{
			string(constants.ChatRoleOwner), string(constants.ChatRoleAdmin),
		}},
	}

	// Build per-participant $set entries (merge, not replace)
	setEntries := bson.M{"updatedAt": time.Now()}
	for _, u := range users {
		updateFilter["participants."+u.ID.Hex()] = bson.M{"$exists": false}
		setEntries["participants."+u.ID.Hex()] = utils.NewParticipantWithDefaults(
			string(constants.ChatRoleMember),
			string(constants.StatusPending),
			string(constants.UserStatusOffline),
			userID,
			[]string{string(constants.ChatPermissionRead), string(constants.ChatPermissionWrite)},
			models.ContactUserInfo{
				UserID:      u.ID,
				DisplayName: u.Profile.DisplayName,
				Username:    u.Username,
				Avatar:      u.Profile.Avatar,
			},
		)
	}

	res, err := database.DB.Collection(string(constants.ChatColl)).UpdateOne(ctx, updateFilter, bson.M{"$set": setEntries})
	if err != nil {
		return fmt.Errorf("failed to update chat: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("%w: chat changed while adding members", ErrPermissionDenied)
	}

	return nil
}

// RemoveGroupMember removes one or more participants from a group chat.
// The acting user must be an owner or admin of the chat.
func RemoveGroupMember(ctx context.Context, actingUserID bson.ObjectID, chatID bson.ObjectID, targetUserIDs []bson.ObjectID) error {

	// Verify chat exists and is a group
	chatFilter := bson.M{
		"_id":      chatID,
		"chatType": string(constants.ChatTypeGroup),
	}

	var chat models.Chat
	err := database.DB.Collection(string(constants.ChatColl)).FindOne(ctx, chatFilter, options.FindOne().SetProjection(bson.M{
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
	if actingParticipant.Role != string(constants.ChatRoleOwner) && actingParticipant.Role != string(constants.ChatRoleAdmin) {
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

	_, err = database.DB.Collection(string(constants.ChatColl)).UpdateOne(ctx, bson.M{"_id": chatID}, chatUpdate)
	if err != nil {
		return fmt.Errorf("failed to update chat: %w", err)
	}

	// Remove chat ref from each removed user
	_, err = database.DB.Collection(string(constants.UserColl)).UpdateMany(ctx, userFilter, userUpdate)
	if err != nil {
		return fmt.Errorf("failed to update users: %w", err)
	}

	return nil
}

// ================ Invites ================
func CreateInvite(ctx context.Context, userID bson.ObjectID, chatID bson.ObjectID) (inviteCode string, err error) {
	filter := bson.M{
		"_id":      chatID,
		"chatType": string(constants.ChatTypeGroup),
		"$and": []bson.M{
			{"participants." + userID.Hex(): bson.M{"$exists": true}},
		},
	}

	count, err := database.DB.Collection(string(constants.ChatColl)).CountDocuments(ctx, filter)
	if err != nil {
		return "", fmt.Errorf("failed to get chat: %w", err)
	}
	if count == 0 {
		return "", fmt.Errorf("%w", ErrChatNotFound)
	}

	invite, ok := utils.NewInviteCodeWithDefaults(time.Now().Add(24 * time.Hour))
	if !ok {
		return "", fmt.Errorf("failed to create invite code")
	}

	update := bson.M{
		"$push": bson.M{
			"inviteCode": invite,
		},
	}

	_, err = database.DB.Collection(string(constants.ChatColl)).UpdateOne(ctx, filter, update)
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

	err := database.DB.Collection(string(constants.ChatColl)).FindOne(ctx, filter, options.FindOne().SetProjection(bson.M{"inviteCode": 1})).Decode(&chat)
	if err != nil {
		return nil, fmt.Errorf("failed to get group invites: %w", err)
	}

	return chat.InviteCode, nil
}

func DeleteInvite(ctx context.Context, userID bson.ObjectID, inviteID string) error {
	chatID, _, err := findInvite(ctx, inviteID)
	if err != nil {
		return err
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

	_, err = database.DB.Collection(string(constants.ChatColl)).UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to delete invite: %w", err)
	}

	return nil
}

func GetJoinedUsersByInvite(ctx context.Context, userID bson.ObjectID, inviteID string) ([]bson.ObjectID, error) {
	chatID, _, err := findInvite(ctx, inviteID)
	if err != nil {
		return nil, err
	}

	filter := bson.M{
		"_id": chatID,
		"$and": []bson.M{
			{"participants." + userID.Hex(): bson.M{"$exists": true}},
			{"inviteCode.inviteCode": inviteID},
		},
	}

	chat := models.Chat{}

	// "inviteCode.$" returns only the entry that matched inviteID, not the
	// chat's first invite (#29).
	err = database.DB.Collection(string(constants.ChatColl)).FindOne(ctx, filter, options.FindOne().SetProjection(bson.M{"inviteCode.$": 1})).Decode(&chat)
	if err != nil {
		return nil, fmt.Errorf("failed to get joined users by invite: %w", err)
	}

	if len(chat.InviteCode) == 0 || chat.InviteCode[0].UserIDs == nil {
		return []bson.ObjectID{}, nil
	}

	return chat.InviteCode[0].UserIDs, nil
}

func SendInviteToUser(ctx context.Context, actingUserID bson.ObjectID, inviteID string, targetUserID bson.ObjectID, creatingGroup bool) error {
	chatID, invite, err := findInvite(ctx, inviteID)
	if err != nil {
		return err
	}

	// A deleted or expired code can't be sent (#91).
	if invite.Deleted || invite.ExpiredDate.Before(time.Now()) {
		return fmt.Errorf("%w", ErrInvalidInvite)
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
		cntDoc, err := database.DB.Collection(string(constants.ChatColl)).CountDocuments(ctx, filter)
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

	_, err = database.DB.Collection(string(constants.UserColl)).UpdateOne(ctx, bson.M{"_id": targetUserID}, targetUserProjection)
	if err != nil {
		return fmt.Errorf("failed to update target user: %w", err)
	}

	return nil
}

// JoinGroupByInvite adds userID to the group that stores the invite code. The
// code must not be deleted or past its stored expiry, and the chat must be a
// group.
func JoinGroupByInvite(ctx context.Context, userID bson.ObjectID, inviteID string) error {
	chatObjectID, _, err := findInvite(ctx, inviteID)
	if err != nil {
		return err
	}

	user, err := GetUserBasicInfo(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	userParticipant := utils.NewParticipantWithDefaults(
		string(constants.ChatRoleMember),
		string(constants.StatusAccepted),
		string(constants.UserStatusOffline),
		userID,
		[]string{string(constants.ChatPermissionRead), string(constants.ChatPermissionWrite)},
		models.ContactUserInfo{
			UserID:      userID,
			DisplayName: user.Profile.DisplayName,
			Username:    user.Username,
			Avatar:      user.Profile.Avatar,
		},
	)

	updateInviteCodeUserIDs := bson.M{
		"$set": bson.M{
			participantField(userID): userParticipant,
		},
		"$addToSet": bson.M{
			"inviteCode.$.userIDs": userID,
		},
	}

	res, err := database.DB.Collection(string(constants.ChatColl)).UpdateOne(ctx,
		bson.M{
			"_id":                    chatObjectID,
			"chatType":               string(constants.ChatTypeGroup),
			participantField(userID): bson.M{"$exists": false},
			"inviteCode": bson.M{"$elemMatch": bson.M{
				"inviteCode":  inviteID,
				"deleted":     bson.M{"$ne": true},
				"expiredDate": bson.M{"$gt": time.Now()},
			}},
		},
		updateInviteCodeUserIDs,
	)
	if err != nil {
		return fmt.Errorf("failed to update invite code user IDs: %w", err)
	}
	if res.MatchedCount == 0 {
		return joinNotApplied(ctx, chatObjectID, userID)
	}

	// Add chat ref to user
	userRes, err := database.DB.Collection(string(constants.UserColl)).UpdateOne(ctx,
		bson.M{"_id": userID},
		bson.M{"$set": bson.M{"chats." + chatObjectID.Hex(): 0}},
	)
	if err != nil {
		return fmt.Errorf("failed to add chat to user: %w", err)
	}
	if userRes.MatchedCount == 0 {
		return fmt.Errorf("add chat to user: %w", ErrUserNotFound)
	}

	return nil
}

// joinNotApplied picks the error for a join whose update matched nothing:
// ErrAlreadyParticipant if userID is already in the chat, else ErrInvalidInvite.
// A member can get here because an earlier join stopped after adding them to
// participants but before adding the chat to their chats list, which the hub
// needs to deliver live messages. A retry would never get past this point, so
// the missing entry is added here. An existing entry keeps its unread count.
func joinNotApplied(ctx context.Context, chatID, userID bson.ObjectID) error {
	count, err := database.DB.Collection(string(constants.ChatColl)).CountDocuments(ctx, chatMemberFilter(chatID, userID))
	if err != nil {
		return fmt.Errorf("failed to check if user is in the chat: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("%w", ErrInvalidInvite)
	}

	chatRef := "chats." + chatID.Hex()
	_, err = database.DB.Collection(string(constants.UserColl)).UpdateOne(ctx,
		bson.M{"_id": userID, chatRef: bson.M{"$exists": false}},
		bson.M{"$set": bson.M{chatRef: 0}},
	)
	if err != nil {
		return fmt.Errorf("failed to restore chat ref of user: %w", err)
	}

	return fmt.Errorf("%w", ErrAlreadyParticipant)
}

func GetAllInvitesOfUser(ctx context.Context, userID bson.ObjectID) ([]models.ChatInvitationEmbed, error) {
	var result struct {
		ChatInvitations []models.ChatInvitationEmbed `bson:"chatInvitations"`
	}

	err := database.DB.Collection(string(constants.UserColl)).FindOne(ctx,
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

// maxInviteCodeLen bounds the codes findInvite looks up. New codes have 32 hex
// characters; codes made before #15 hold the encrypted chat ID, creator and
// expiry, and stay well under this.
const maxInviteCodeLen = 512

// findInvite returns the chat that stores inviteCode and that code's stored
// entry. Codes are opaque (#15): nothing is read from the code itself, so a
// code that isn't stored on a chat, or isn't hex at all, is ErrInvalidInvite.
// A deleted or expired code is still found: JoinGroupByInvite refuses it in its
// update filter, and SendInviteToUser checks the returned entry.
func findInvite(ctx context.Context, inviteCode string) (bson.ObjectID, models.InviteCodeEmbed, error) {
	if inviteCode == "" || len(inviteCode) > maxInviteCodeLen {
		return bson.ObjectID{}, models.InviteCodeEmbed{}, fmt.Errorf("%w: bad invite code length", ErrInvalidInvite)
	}
	if _, err := hex.DecodeString(inviteCode); err != nil {
		return bson.ObjectID{}, models.InviteCodeEmbed{}, fmt.Errorf("%w: invite code is not hex", ErrInvalidInvite)
	}

	var chat struct {
		ID         bson.ObjectID            `bson:"_id"`
		InviteCode []models.InviteCodeEmbed `bson:"inviteCode"`
	}
	err := database.DB.Collection(string(constants.ChatColl)).FindOne(ctx,
		bson.M{"inviteCode.inviteCode": inviteCode},
		options.FindOne().SetProjection(bson.M{"_id": 1, "inviteCode.$": 1}),
	).Decode(&chat)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return bson.ObjectID{}, models.InviteCodeEmbed{}, fmt.Errorf("%w: invite code not found", ErrInvalidInvite)
	}
	if err != nil {
		return bson.ObjectID{}, models.InviteCodeEmbed{}, fmt.Errorf("failed to find invite: %w", err)
	}
	if len(chat.InviteCode) == 0 {
		return bson.ObjectID{}, models.InviteCodeEmbed{}, fmt.Errorf("%w: invite code not found", ErrInvalidInvite)
	}

	return chat.ID, chat.InviteCode[0], nil
}
