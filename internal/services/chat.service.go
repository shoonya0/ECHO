package services

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
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
// user's chats list where the user is also a participant who has accepted (a
// pending invite is listed too, #5, but not followed). The same query also
// guards against an entry for a chat that doesn't list the user: forged invite
// joins left those before #9, and RemoveStaleChatEntries removes them on each start (#69).
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
		bson.M{
			"_id":                    bson.M{"$in": listed},
			participantField(userID): bson.M{"$exists": true},
			// A pending invite isn't followed until it is accepted (#5, #17).
			participantField(userID) + ".requestStatus": bson.M{"$ne": string(constants.StatusPending)},
		},
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
	chat.Stats.ParticipantCount = len(chat.Participants) // not stored (#110)

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
	_, err := findChatForMember(ctx, chatID, userID, bson.M{"_id": 1})
	return err
}

// GetChatForMember returns the chat's type, participants and size limit, read
// in the same query that checks userID is a participant, so a handler that
// checks membership first doesn't read the chat twice (#106). Like
// RequireChatMember, an unknown chat gives ErrNotParticipant. Only ChatID,
// ChatType, Participants and Settings.MaxParticipants are filled; every other
// field is zero, so add to the projection before reading another one.
func GetChatForMember(ctx context.Context, chatID, userID bson.ObjectID) (models.Chat, error) {
	return findChatForMember(ctx, chatID, userID, bson.M{
		"chatType":                 1,
		"participants":             1,
		"settings.maxParticipants": 1,
	})
}

func findChatForMember(ctx context.Context, chatID, userID bson.ObjectID, projection bson.M) (models.Chat, error) {
	var chat models.Chat
	opts := options.FindOne().SetProjection(projection)
	err := database.DB.Collection(string(constants.ChatColl)).FindOne(ctx, chatMemberFilter(chatID, userID), opts).Decode(&chat)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return chat, fmt.Errorf("%w", ErrNotParticipant)
	}
	if err != nil {
		return chat, fmt.Errorf("failed to check chat membership: %w", err)
	}
	return chat, nil
}

// ChatMembers returns the participants of each chat, as hex IDs keyed by the
// chat's hex ID, in one query. A chat that doesn't exist has no entry. The hub
// uses it to take connections out of rooms their user is no longer a member
// of (#117).
func ChatMembers(ctx context.Context, chatIDs []bson.ObjectID) (map[string]map[string]struct{}, error) {
	members := make(map[string]map[string]struct{}, len(chatIDs))
	if len(chatIDs) == 0 {
		return members, nil
	}

	// Only the participants map's keys are sent back, as strings: the entries
	// can be large, and one that doesn't decode must not fail the whole batch.
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"_id": bson.M{"$in": chatIDs}}}},
		{{Key: "$project", Value: bson.M{"members": bson.M{"$map": bson.M{
			"input": bson.M{"$objectToArray": bson.M{"$ifNull": bson.A{"$participants", bson.M{}}}},
			"in":    "$$this.k",
		}}}}},
	}
	cursor, err := database.DB.Collection(string(constants.ChatColl)).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to read chat members: %w", err)
	}
	var chats []struct {
		ID      bson.ObjectID `bson:"_id"`
		Members []string      `bson:"members"`
	}
	if err := cursor.All(ctx, &chats); err != nil {
		return nil, fmt.Errorf("failed to decode chat members: %w", err)
	}

	for _, chat := range chats {
		users := make(map[string]struct{}, len(chat.Members))
		for _, userID := range chat.Members {
			users[userID] = struct{}{}
		}
		members[chat.ID.Hex()] = users
	}
	return members, nil
}

// RequireChatAccess is RequireChatMember for actions the other members see:
// typing, read receipts and reactions. In a direct chat, a block either way
// returns ErrBlocked (#27). Reading history needs membership only.
func RequireChatAccess(ctx context.Context, chatID, userID bson.ObjectID) error {
	chat, err := findChatForMember(ctx, chatID, userID, bson.M{"chatType": 1, "participants": 1})
	if err != nil {
		return err
	}
	return CheckDirectChatBlock(ctx, &chat, userID)
}

// CheckDirectChatBlock returns ErrBlocked when chat is a direct chat and userID
// and the other participant have a block between them, either way (#27). A valid
// direct chat has one other participant, so this is one lookup; a legacy chat
// with yourself (#38) has none and is never blocked.
func CheckDirectChatBlock(ctx context.Context, chat *models.Chat, userID bson.ObjectID) error {
	if chat.ChatType != string(constants.ChatTypeDirect) {
		return nil
	}
	for otherID := range chat.Participants {
		if otherID == userID {
			continue
		}
		blocked, err := IsBlockedBetween(ctx, userID, otherID)
		if err != nil {
			return err
		}
		if blocked {
			return fmt.Errorf("%w", ErrBlocked)
		}
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
	// The stored names are from when the message was written (#36).
	if err := fillMessageNames(ctx, messages); err != nil {
		return nil, false, err
	}
	return messages, hasMore, nil
}

// ============ CHAT MANAGEMENT ============
func CreateDirectChat(ctx context.Context, targetUserID bson.ObjectID) (*models.Chat, error) {
	user := ctx.Value(constants.UserDataKey).(models.LoginUserResponse)
	if user == (models.LoginUserResponse{}) {
		return nil, fmt.Errorf("invalid user data")
	}
	user, err := WithCurrentDisplayInfo(ctx, user) // copied into the chat, so not the JWT's (#36)
	if err != nil {
		return nil, err
	}
	// A chat with yourself would have one participant: both entries share
	// one map key (#38).
	if user.ID == targetUserID {
		return nil, fmt.Errorf("%w", ErrSelfDirectChat)
	}
	// Neither side of a block may open the chat, new or existing (#27).
	blocked, err := IsBlockedBetween(ctx, user.ID, targetUserID)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, fmt.Errorf("%w", ErrBlocked)
	}

	found, err := findOrCreateDirectChat(ctx, user, targetUserID)
	if err != nil || found.created {
		return found.chat, err
	}

	// Write back an entry a failed or partial earlier write left out (#132).
	chatID := found.chat.ChatID
	repaired, err := database.DB.Collection(string(constants.UserColl)).UpdateMany(ctx,
		bson.M{
			"_id":                   bson.M{"$in": []bson.ObjectID{user.ID, targetUserID}},
			"chats." + chatID.Hex(): bson.M{"$exists": false},
		},
		bson.M{"$set": bson.M{"chats." + chatID.Hex(): 0}},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to repair chat entries: %w", err)
	}
	if repaired.ModifiedCount > 0 {
		AddMembersToLiveChat(ctx, chatID, user.ID, targetUserID)
	}

	return found.chat, nil
}

// directChatLookup is the result of findOrCreateDirectChat: the pair's chat, and
// whether this call created it (with both users' entries).
type directChatLookup struct {
	chat    *models.Chat
	created bool
}

// directChatAttempts bounds how often findOrCreateDirectChat starts over when
// the chat a concurrent call created is gone again: that call's entries failed
// and it removed the chat (#132).
const directChatAttempts = 3

// findOrCreateDirectChat returns the pair's direct chat, creating it if there is
// none. The keyed chat comes first: a pair with a duplicate from before #37 keeps
// using the one the backfill keyed. Then the oldest unkeyed one. A create that
// loses the race on directKey uses the winner's chat (#37).
func findOrCreateDirectChat(ctx context.Context, user models.LoginUserResponse, targetUserID bson.ObjectID) (directChatLookup, error) {
	chats := database.DB.Collection(string(constants.ChatColl))
	fields := bson.M{"_id": 1, "chatType": 1, "participants": 1}
	key := directChatKey(user.ID, targetUserID)
	legacy := bson.M{
		"chatType":                           "direct",
		"participants." + user.ID.Hex():      bson.M{"$exists": true},
		"participants." + targetUserID.Hex(): bson.M{"$exists": true},
	}

	for range directChatAttempts {
		var existing models.Chat
		err := chats.FindOne(ctx, bson.M{"directKey": key}, options.FindOne().SetProjection(fields)).Decode(&existing)
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = chats.FindOne(ctx, legacy,
				options.FindOne().SetProjection(fields).SetSort(bson.D{{Key: "_id", Value: 1}})).Decode(&existing)
		}
		if err == nil {
			return directChatLookup{chat: &existing}, nil
		}
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return directChatLookup{}, fmt.Errorf("failed to fetch existing chat: %w", err)
		}

		chat, err := insertDirectChat(ctx, user, targetUserID)
		if !errors.Is(err, errDirectChatTaken) {
			return directChatLookup{chat: chat, created: err == nil}, err
		}
		// A concurrent call inserted first: look again.
	}
	return directChatLookup{}, fmt.Errorf("failed to fetch existing chat: the concurrently created chat kept disappearing")
}

// directChatKey is the directKey of the direct chat between a and b: both IDs,
// smaller first, so either side gets the same key.
func directChatKey(a, b bson.ObjectID) string {
	x, y := a.Hex(), b.Hex()
	if y < x {
		x, y = y, x
	}
	return x + "_" + y
}

// errDirectChatTaken: the pair already has a keyed direct chat.
var errDirectChatTaken = errors.New("the pair already has a direct chat")

// insertDirectChat creates the direct chat between the user and the target, with
// both users' entries, and puts their connections in it. If the pair already
// has a keyed chat, it returns errDirectChatTaken.
func insertDirectChat(ctx context.Context, user models.LoginUserResponse, targetUserID bson.ObjectID) (*models.Chat, error) {
	targetUser, err := GetUserByID(ctx, targetUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get target user: %w", err)
	}

	chat := utils.NewChatWithDefaults(user.ID, "direct")
	chat.DirectKey = directChatKey(user.ID, targetUserID)
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
	// Only a clash on directKey_1 means the pair's chat exists; any other
	// duplicate key is a real failure.
	if mongo.IsDuplicateKeyError(err) && strings.Contains(err.Error(), "directKey_1") {
		return nil, errDirectChatTaken
	}
	if err != nil {
		return nil, fmt.Errorf("failed to insert chat: %w", err)
	}

	// The hub follows chats from these entries, so the chat is kept only when
	// both users got theirs (#132).
	if err := addDirectChatEntries(ctx, chat.ChatID, user.ID, targetUserID); err != nil {
		return nil, err
	}

	AddMembersToLiveChat(ctx, chat.ChatID, user.ID, targetUserID) // #49
	return &chat, nil
}

// addDirectChatEntries sets chats.<chatID> on both users. If either user
// document doesn't match, or the write fails, it removes the chat and any entry
// already written, and returns the error (ErrUserNotFound for a missing user).
func addDirectChatEntries(ctx context.Context, chatID, a, b bson.ObjectID) error {
	users := database.DB.Collection(string(constants.UserColl))
	ids := bson.M{"_id": bson.M{"$in": []bson.ObjectID{a, b}}}
	key := "chats." + chatID.Hex()

	res, err := users.UpdateMany(ctx, ids, bson.M{"$set": bson.M{key: 0}})
	if err == nil && res.MatchedCount < 2 {
		err = ErrUserNotFound
	}
	if err == nil {
		return nil
	}
	if _, undoErr := users.UpdateMany(ctx, ids, bson.M{"$unset": bson.M{key: ""}}); undoErr != nil {
		return fmt.Errorf("failed to add direct chat to users: %w (and removing the entries failed: %v)", err, undoErr)
	}
	if _, delErr := database.DB.Collection(string(constants.ChatColl)).DeleteOne(ctx, bson.M{"_id": chatID}); delErr != nil {
		return fmt.Errorf("failed to add direct chat to users: %w (and removing the chat failed: %v)", err, delErr)
	}
	return fmt.Errorf("failed to add direct chat to users: %w", err)
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
	creator, err := WithCurrentDisplayInfo(ctx, creator) // copied into the group, so not the JWT's (#36)
	if err != nil {
		return nil, err
	}

	// The group, its members' chats entries and the invitations are written in
	// one transaction (#37): a failed invitation used to return 500 with the
	// group saved, and the client's retry made a second one.
	var chat models.Chat
	err = database.WithTransaction(ctx, func(ctx context.Context) error {
		var err error
		chat, err = createGroupChat(ctx, creator, chatName, description, participantIDs)
		return err
	})
	if err != nil {
		return nil, err
	}
	AddMembersToLiveChat(ctx, chat.ChatID, creator.ID) // #49, once it is saved
	return &chat, nil
}

// createGroupChat writes a new group inside CreateGroupChat's transaction. The
// chat is new, so it can't conflict; the users are then written through
// UpdateMany over _id, in ID order, as every group write does.
func createGroupChat(ctx context.Context, creator models.LoginUserResponse, chatName, description string, participantIDs []bson.ObjectID) (models.Chat, error) {
	users, err := GetUsersByIDs(ctx, participantIDs)
	if err != nil {
		return models.Chat{}, fmt.Errorf("failed to get users: %w", err)
	}

	participants := newGroupParticipants(creator, users)
	if len(participants) > constants.DefaultMaxGroupParticipants {
		return models.Chat{}, fmt.Errorf("%w: %d participants", ErrGroupFull, len(participants))
	}

	chat := utils.NewChatWithDefaults(creator.ID, string(constants.ChatTypeGroup))
	chat.Participants = participants
	chat.Name = chatName
	chat.Description = description
	chat.Settings.MaxParticipants = constants.DefaultMaxGroupParticipants

	// Create invite code
	invite, ok := utils.NewInviteCodeWithDefaults(time.Now().Add(24 * time.Hour))
	if !ok {
		return models.Chat{}, fmt.Errorf("failed to create invite code")
	}
	chat.InviteCode = append(chat.InviteCode, invite)

	if _, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, chat); err != nil {
		return models.Chat{}, fmt.Errorf("failed to create group chat: %w", err)
	}
	// Only for the reply: the count isn't stored, reads compute it (#110).
	chat.Stats.ParticipantCount = len(participants)

	invitees := make([]bson.ObjectID, 0, len(users))
	for _, u := range users {
		if u.ID != creator.ID {
			invitees = append(invitees, u.ID)
		}
	}

	// Every member's chats entry, the creator's included. The hub delivers live
	// messages from it, so a creator without one fails the create (#112). The
	// invitees were read in this transaction, so they all exist.
	members := append([]bson.ObjectID{creator.ID}, invitees...)
	matched, err := giveChatEntry(ctx, chat.ChatID, members) // #5
	if err != nil {
		return models.Chat{}, err
	}
	if matched < int64(len(members)) {
		return models.Chat{}, fmt.Errorf("failed to add group chat to creator: %w", ErrUserNotFound)
	}

	// Each invitee's invitation for the new code.
	if len(invitees) > 0 {
		res, err := database.DB.Collection(string(constants.UserColl)).UpdateMany(ctx,
			bson.M{"_id": bson.M{"$in": invitees}}, addInvitationUpdate(invite.InviteCode))
		if err != nil {
			return models.Chat{}, fmt.Errorf("failed to send the invitations: %w", err)
		}
		if res.MatchedCount < int64(len(invitees)) {
			return models.Chat{}, fmt.Errorf("send the invitations: %w", ErrUserNotFound)
		}
	}
	return chat, nil
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

	if message.Mentions, err = resolveMentions(ctx, chatID, mentions); err != nil {
		return nil, err
	}

	// The message and the chat's lastMessageId and messageCount are written
	// together (#37). The reads above stay outside: every message in a chat
	// writes the chat, so concurrent sends conflict and rerun, and a rerun
	// should be only these two writes.
	message.ID = bson.NewObjectID()
	err = database.WithTransaction(ctx, func(ctx context.Context) error {
		if err := updateChatLastMessage(ctx, chatID, message.ID, now); err != nil {
			return err
		}
		if _, err := database.DB.Collection(string(constants.MessageColl)).InsertOne(ctx, message); err != nil {
			return fmt.Errorf("failed to save message: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Broadcast message in real-time
	broadcastNewMessage(chatID, message)

	return &message, nil
}

// resolveMentions turns the mentioned user IDs of a message into its mentions: each
// participant of the chat once, in the order sent, with the username beside it.
// Invalid IDs and users who aren't participants are left out, so a message can't show
// the username of any account. It reads the chat and the users once each, not once
// per mention (#40).
func resolveMentions(ctx context.Context, chatID bson.ObjectID, mentions []string) (models.MentionsEmbed, error) {
	ids := make([]bson.ObjectID, 0, len(mentions))
	seen := make(map[bson.ObjectID]bool, len(mentions))
	for _, mention := range mentions {
		id, err := bson.ObjectIDFromHex(mention)
		if err != nil || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return models.MentionsEmbed{}, nil
	}

	projection := bson.M{"_id": 1}
	for _, id := range ids {
		projection[participantField(id)] = 1
	}
	var chat models.Chat
	err := database.DB.Collection(string(constants.ChatColl)).FindOne(ctx, bson.M{"_id": chatID},
		options.FindOne().SetProjection(projection)).Decode(&chat)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return models.MentionsEmbed{}, fmt.Errorf("%w", ErrChatNotFound)
	}
	if err != nil {
		return models.MentionsEmbed{}, fmt.Errorf("failed to read the mentioned participants: %w", err)
	}
	members := make([]bson.ObjectID, 0, len(ids))
	for _, id := range ids {
		if _, ok := chat.Participants[id]; ok {
			members = append(members, id)
		}
	}
	if len(members) == 0 {
		return models.MentionsEmbed{}, nil
	}

	cursor, err := database.DB.Collection(string(constants.UserColl)).Find(ctx,
		bson.M{"_id": bson.M{"$in": members}}, options.Find().SetProjection(bson.M{"username": 1}))
	if err != nil {
		return models.MentionsEmbed{}, fmt.Errorf("failed to read the mentioned users: %w", err)
	}
	var users []struct {
		ID       bson.ObjectID `bson:"_id"`
		Username string        `bson:"username"`
	}
	if err := cursor.All(ctx, &users); err != nil {
		return models.MentionsEmbed{}, fmt.Errorf("failed to read the mentioned users: %w", err)
	}
	names := make(map[bson.ObjectID]string, len(users))
	for _, u := range users {
		names[u.ID] = u.Username
	}

	var result models.MentionsEmbed
	for _, id := range members {
		if name, ok := names[id]; ok {
			result.UserIDs = append(result.UserIDs, id)
			result.UserNames = append(result.UserNames, name)
		}
	}
	return result, nil
}

// UpdateTypingStatus updates typing indicator for a user in a chat
func UpdateTypingStatus(ctx context.Context, chatID, userID bson.ObjectID, isTyping bool) error {
	if err := RequireChatAccess(ctx, chatID, userID); err != nil {
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

	// Typing is only broadcast, never stored: a MongoDB write per event cost
	// too much, and nothing reads it back (#40).
	BroadcastToChat(chatID.Hex(), typingMsg)

	return nil
}

// MarkMessagesAsRead marks messages as read by a user. IDs that aren't messages
// of this chat are skipped; it returns ErrMessageNotFound when none are left.
func MarkMessagesAsRead(ctx context.Context, chatID, userID bson.ObjectID, messageIDs []bson.ObjectID) error {
	now := time.Now()

	if err := RequireChatAccess(ctx, chatID, userID); err != nil {
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

	// The chat keeps when each member last read. Without it the read isn't saved
	// in full, so the chat isn't told; marking the same messages again is safe.
	if err := updateChatReadReceipts(ctx, chatID, userID, now); err != nil {
		return err
	}

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

// LiveMemberRemover takes users' open connections out of a chat's room on every
// instance. The realtime hub implements it next to Broadcaster.
type LiveMemberRemover interface {
	RemoveMembersFromChat(chatID string, userIDs []string) error
}

// RemoveMembersFromLiveChat stops the chat's live events reaching the users'
// open connections, after they were removed from participants (#14). Call it
// after broadcasting the event that announces the removal, which they still get.
func RemoveMembersFromLiveChat(chatID string, userIDs []string) error {
	remover, ok := broadcaster.(LiveMemberRemover)
	if !ok {
		return fmt.Errorf("websocket hub not initialized")
	}
	return remover.RemoveMembersFromChat(chatID, userIDs)
}

// LiveMemberAdder puts users' open connections in a chat's room on every
// instance. The realtime hub implements it next to Broadcaster.
type LiveMemberAdder interface {
	AddMembersToChat(chatID string, userIDs []string) error
}

// AddMembersToLiveChat makes the users' open connections follow a chat they just
// gained, so its live events reach them without a reconnect (#49). Call it after
// the write that made them members succeeded. A failure is only logged: the
// chat exists, and a reconnect follows it.
func AddMembersToLiveChat(ctx context.Context, chatID bson.ObjectID, userIDs ...bson.ObjectID) {
	adder, ok := broadcaster.(LiveMemberAdder)
	if !ok {
		return // no hub in this process, as in tests without one
	}
	ids := make([]string, len(userIDs))
	for i, id := range userIDs {
		ids[i] = id.Hex()
	}
	if err := adder.AddMembersToChat(chatID.Hex(), ids); err != nil {
		logger.WithContext(ctx).WithError(err).Warn("Open connections may not follow a new chat until they reconnect")
	}
}

// ============ HELPER FUNCTIONS ============

// updateChatLastMessage counts a new message in the chat and makes it the last
// one. $max keeps the newest: message IDs start with their second, so a send
// that commits after a newer one (a rerun) can't set an older message (#37).
// Two sends in the same second from different servers compare by their random
// part, so either may win: as arbitrary as with $set. updatedAt uses $set, as
// every chat write does: $max would stick to a time from a server whose clock
// runs ahead.
func updateChatLastMessage(ctx context.Context, chatID, messageID bson.ObjectID, timestamp time.Time) error {
	res, err := database.DB.Collection(string(constants.ChatColl)).UpdateOne(ctx,
		bson.M{"_id": chatID},
		bson.M{
			"$max": bson.M{"lastMessageId": messageID},
			"$set": bson.M{"updatedAt": timestamp},
			"$inc": bson.M{"stats.messageCount": 1},
		},
	)
	if err != nil {
		return fmt.Errorf("failed to update the chat's last message: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("%w", ErrChatNotFound)
	}
	return nil
}

// updateChatReadReceipts saves when userID last read the chat.
func updateChatReadReceipts(ctx context.Context, chatID, userID bson.ObjectID, timestamp time.Time) error {
	filter := bson.M{"_id": chatID}
	update := bson.M{
		"$set": bson.M{
			"readReceipts." + userID.Hex(): timestamp,
		},
	}

	res, err := database.DB.Collection(string(constants.ChatColl)).UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update the chat's read receipts: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("%w", ErrChatNotFound)
	}
	return nil
}

// broadcastNewMessage broadcasts a new message to chat participants
func broadcastNewMessage(chatID bson.ObjectID, message models.Message) {
	// The sender and mentions saved with the message, so clients can show it
	// without looking the sender up (#50).
	sender := message.Sender
	mentions := make([]string, len(message.Mentions.UserIDs))
	for i, id := range message.Mentions.UserIDs {
		mentions[i] = id.Hex()
	}
	wsMessage := models.WebSocketMessage{
		Type:     models.WSMessageTypeChat,
		ChatID:   chatID.Hex(),
		UserID:   message.SenderID.Hex(),
		Username: sender.Username,
		Avatar:   sender.Avatar,
		Data: models.ChatMessage{
			ID:          message.ID.Hex(),
			ChatID:      chatID.Hex(),
			SenderID:    message.SenderID.Hex(),
			Sender:      &sender,
			Content:     message.Content,
			MessageType: message.MessageType,
			Attachments: message.Attachments,
			Mentions:    mentions,
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

// AddMessageReaction adds userID's emoji reaction to a message. It reports
// whether anything changed: false when the user had already reacted with that
// emoji, so the caller doesn't announce a reaction that was already there.
func AddMessageReaction(ctx context.Context, chatID, messageID, userID bson.ObjectID, emoji string) (bool, error) {
	if err := validateEmoji(emoji); err != nil {
		return false, err
	}

	if err := RequireChatAccess(ctx, chatID, userID); err != nil {
		return false, err
	}

	// details maps each user to their username, so history can show who reacted
	// without a lookup per ID (#75). Like the sender copied into each message, it
	// is a copy and goes stale if the username changes (#36).
	user, err := GetUserByID(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("failed to get reacting user: %w", err)
	}

	// Only a user who hasn't reacted yet matches, so repeating an add changes
	// nothing (#25). The count is set from the users list in the same write,
	// which also corrects a count that drifted before the fix. Being a pipeline,
	// the update can also create the emoji inside reactions: null, which every
	// message saved before #72 has.
	path := "reactions." + emoji
	filter := bson.M{
		"_id":           messageID,
		"chatId":        chatID,
		path + ".users": bson.M{"$ne": userID},
	}
	update := mongo.Pipeline{
		{{Key: "$set", Value: bson.D{
			{Key: path + ".users", Value: bson.M{"$concatArrays": bson.A{
				bson.M{"$ifNull": bson.A{"$" + path + ".users", bson.A{}}},
				bson.A{userID},
			}}},
			{Key: path + ".details." + userID.Hex(), Value: bson.M{"$literal": user.Username}},
			{Key: "updatedAt", Value: time.Now()},
		}}},
		reactionCountStage(path),
	}

	return applyReactionChange(ctx, chatID, messageID, filter, update, "failed to add reaction")
}

// RemoveMessageReaction removes userID's emoji reaction from a message. It
// reports whether anything changed: false when the user hadn't reacted with
// that emoji. The emoji is removed from the message with its last user.
func RemoveMessageReaction(ctx context.Context, chatID, messageID, userID bson.ObjectID, emoji string) (bool, error) {
	if err := validateEmoji(emoji); err != nil {
		return false, err
	}

	if err := RequireChatAccess(ctx, chatID, userID); err != nil {
		return false, err
	}

	// Only a user who has reacted matches, so a remove by anyone else can't
	// lower the count or delete other users' reactions (#25).
	path := "reactions." + emoji
	filter := bson.M{
		"_id":           messageID,
		"chatId":        chatID,
		path + ".users": userID,
	}
	update := mongo.Pipeline{
		{{Key: "$set", Value: bson.D{
			{Key: path + ".users", Value: bson.M{"$filter": bson.M{
				"input": "$" + path + ".users",
				"cond":  bson.M{"$ne": bson.A{"$$this", userID}},
			}}},
			{Key: "updatedAt", Value: time.Now()},
		}}},
		reactionCountStage(path),
		{{Key: "$unset", Value: path + ".details." + userID.Hex()}},
		// The emoji goes with its last user, in the same write, so nobody reads
		// an entry with no users. It checks users, not a count that may have
		// drifted, so it can't delete reactions that still have users.
		{{Key: "$set", Value: bson.D{
			{Key: path, Value: bson.M{"$cond": bson.A{
				bson.M{"$eq": bson.A{bson.M{"$size": "$" + path + ".users"}, 0}},
				"$$REMOVE",
				"$" + path,
			}}},
		}}},
	}

	return applyReactionChange(ctx, chatID, messageID, filter, update, "failed to remove reaction")
}

// reactionCountStage sets an emoji's count from its users list, so the count
// can't drift from who reacted (#25), and one that drifted before is corrected.
func reactionCountStage(path string) bson.D {
	return bson.D{{Key: "$set", Value: bson.D{
		{Key: path + ".count", Value: bson.M{"$size": "$" + path + ".users"}},
	}}}
}

// applyReactionChange runs a reaction update whose filter matches only when the
// change applies. When it matches nothing, it tells a message that doesn't exist
// (ErrMessageNotFound) from one the change doesn't apply to (false, nil).
func applyReactionChange(ctx context.Context, chatID, messageID bson.ObjectID, filter bson.M, update mongo.Pipeline, failure string) (bool, error) {
	messages := database.DB.Collection(string(constants.MessageColl))

	result, err := messages.UpdateOne(ctx, filter, update)
	if err != nil {
		return false, fmt.Errorf("%s: %w", failure, err)
	}
	if result.MatchedCount > 0 {
		return true, nil
	}

	found, err := messages.CountDocuments(ctx, bson.M{"_id": messageID, "chatId": chatID}, options.Count().SetLimit(1))
	if err != nil {
		return false, fmt.Errorf("%s: %w", failure, err)
	}
	if found == 0 {
		return false, ErrMessageNotFound
	}
	return false, nil
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
		"participants":             1,
		"settings.maxParticipants": 1,
	})).Decode(&chat)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return fmt.Errorf("%w", ErrChatNotFound)
		}
		return fmt.Errorf("failed to get chat: %w", err)
	}
	chat.ChatID = chatID
	return addGroupMembers(ctx, userID, chat, participantIDs)
}

// AddGroupMemberToChat is AddGroupMember for a caller that has already read the
// chat with GetChatForMember, so the chat isn't read again (#106). The update
// still re-checks the role, the chat type and the size limit.
func AddGroupMemberToChat(ctx context.Context, userID bson.ObjectID, chat models.Chat, participantIDs []bson.ObjectID) error {
	if chat.ChatID.IsZero() {
		return fmt.Errorf("add group members: chat has no ID")
	}
	if chat.ChatType != string(constants.ChatTypeGroup) {
		return fmt.Errorf("%w", ErrChatNotFound)
	}
	return addGroupMembers(ctx, userID, chat, participantIDs)
}

// The chat and the new members' chats entries are written in one transaction
// (#37), the chat first. chat was read before it; the update's filter re-checks
// what matters, so a stale copy only makes the add fail, never apply wrongly.
func addGroupMembers(ctx context.Context, userID bson.ObjectID, chat models.Chat, participantIDs []bson.ObjectID) error {
	return database.WithTransaction(ctx, func(ctx context.Context) error {
		return addGroupMembersTx(ctx, userID, chat, participantIDs)
	})
}

func addGroupMembersTx(ctx context.Context, userID bson.ObjectID, chat models.Chat, participantIDs []bson.ObjectID) error {
	chatID := chat.ChatID

	// An outsider gets the same error as for a missing group, so group IDs
	// can't be probed (#59).
	actingParticipant, ok := chat.Participants[userID]
	if !ok {
		return fmt.Errorf("%w", ErrChatNotFound)
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
	if len(chat.Participants)+len(users) > groupSizeLimit(chat.Settings.MaxParticipants) {
		return fmt.Errorf("%w", ErrGroupFull)
	}

	// Re-check the acting user's role, the targets' absence and the size limit
	// in the update itself, so a concurrent demotion or add between the read
	// and the write can't be overridden.
	updateFilter := bson.M{
		"_id":      chatID,
		"chatType": string(constants.ChatTypeGroup),
		"participants." + userID.Hex() + ".role": bson.M{"$in": []string{
			string(constants.ChatRoleOwner), string(constants.ChatRoleAdmin),
		}},
		"$expr": roomForParticipants(len(users)),
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

	// The filter requires every user to be absent, so the update adds all of
	// them or nobody.
	res, err := database.DB.Collection(string(constants.ChatColl)).UpdateOne(ctx, updateFilter, bson.M{
		"$set": setEntries,
	})
	if err != nil {
		return fmt.Errorf("failed to update chat: %w", err)
	}
	addedIDs := make([]bson.ObjectID, 0, len(users))
	for _, u := range users {
		addedIDs = append(addedIDs, u.ID)
	}
	if res.MatchedCount == 0 {
		return addNotApplied(ctx, chatID, addedIDs)
	}
	// The users were read in this transaction, so all of them match; one that
	// doesn't would be a participant with no chats entry (#5, #112).
	matched, err := giveChatEntry(ctx, chatID, addedIDs) // #5
	if err != nil {
		return err
	}
	if matched < int64(len(addedIDs)) {
		return fmt.Errorf("add the group to new members' chats: %w", ErrUserNotFound)
	}
	return nil
}

// addNotApplied picks the error for an add whose update matched nothing, from the
// group as it is now: ErrChatNotFound if it is gone, ErrPermissionDenied (the chat
// changed; a retry may work) if one of addedIDs was added meanwhile or the actor's
// role changed, and ErrGroupFull only if addedIDs no longer fit (#68).
func addNotApplied(ctx context.Context, chatID bson.ObjectID, addedIDs []bson.ObjectID) error {
	participants, limit, err := loadGroupSize(ctx, chatID)
	if err != nil {
		return err
	}
	for _, id := range addedIDs {
		if _, ok := participants[id]; ok {
			return fmt.Errorf("%w: members added meanwhile", ErrPermissionDenied)
		}
	}
	if len(participants)+len(addedIDs) > limit {
		return fmt.Errorf("%w: filled while adding members", ErrGroupFull)
	}
	return fmt.Errorf("%w: chat changed while adding members", ErrPermissionDenied)
}

// groupSizeLimit is a group's size limit: the stored settings.maxParticipants, or
// the default for groups that store none (#68).
func groupSizeLimit(stored int) int {
	if stored > 0 {
		return stored
	}
	return constants.DefaultMaxGroupParticipants
}

// participantCountExpr counts the entries of a chat's participants map. A group's
// participantCount is computed with it when read, not stored, so no write can leave it
// wrong (#110).
var participantCountExpr = bson.M{"$size": bson.M{"$objectToArray": bson.M{"$ifNull": bson.A{"$participants", bson.M{}}}}}

// roomForParticipants is an update-filter $expr that matches a group only if n more
// participants fit within its size limit (#68). It counts the participants map itself.
func roomForParticipants(n int) bson.M {
	stored := bson.M{"$ifNull": bson.A{"$settings.maxParticipants", 0}}
	limit := bson.M{"$cond": bson.A{bson.M{"$gt": bson.A{stored, 0}}, stored, constants.DefaultMaxGroupParticipants}}
	return bson.M{"$lte": bson.A{bson.M{"$add": bson.A{participantCountExpr, n}}, limit}}
}

// loadGroupSize reads the group chatID's participants and its size limit. It
// returns ErrChatNotFound if there is no such group.
func loadGroupSize(ctx context.Context, chatID bson.ObjectID) (map[bson.ObjectID]models.ParticipantEmbed, int, error) {
	var chat models.Chat
	err := database.DB.Collection(string(constants.ChatColl)).FindOne(ctx,
		bson.M{"_id": chatID, "chatType": string(constants.ChatTypeGroup)},
		options.FindOne().SetProjection(bson.M{"participants": 1, "settings.maxParticipants": 1}),
	).Decode(&chat)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, 0, fmt.Errorf("%w", ErrChatNotFound)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("failed to check group size: %w", err)
	}
	return chat.Participants, groupSizeLimit(chat.Settings.MaxParticipants), nil
}

// RemoveGroupMember removes one or more participants from a group chat.
// The acting user must be an owner or admin of the chat, and only the owner
// can remove an owner. Targets not in the chat, and the actor, are skipped.
// It returns the IDs that were removed.
//
// The chat and the removed users change in one transaction (#37): the chat
// first, then the users (in ID order, as UpdateMany over _id goes), the order
// every group write uses so two of them can't keep aborting each other.
func RemoveGroupMember(ctx context.Context, actingUserID bson.ObjectID, chatID bson.ObjectID, targetUserIDs []bson.ObjectID) ([]bson.ObjectID, error) {
	var removed []bson.ObjectID
	err := database.WithTransaction(ctx, func(ctx context.Context) error {
		var err error
		removed, err = removeGroupMember(ctx, actingUserID, chatID, targetUserIDs)
		return err
	})
	if err != nil {
		return nil, err
	}
	return removed, nil
}

func removeGroupMember(ctx context.Context, actingUserID bson.ObjectID, chatID bson.ObjectID, targetUserIDs []bson.ObjectID) ([]bson.ObjectID, error) {

	// Verify chat exists and is a group
	chatFilter := bson.M{
		"_id":      chatID,
		"chatType": string(constants.ChatTypeGroup),
	}

	var chat models.Chat
	err := database.DB.Collection(string(constants.ChatColl)).FindOne(ctx, chatFilter, options.FindOne().SetProjection(bson.M{
		"participants": 1,
		"ownerId":      1,
	})).Decode(&chat)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("%w", ErrChatNotFound)
		}
		return nil, fmt.Errorf("failed to get chat: %w", err)
	}

	// An outsider gets the same error as for a missing group, so group IDs
	// can't be probed (#59).
	actingParticipant, ok := chat.Participants[actingUserID]
	if !ok {
		return nil, fmt.Errorf("%w", ErrChatNotFound)
	}

	// Must be owner or admin
	if actingParticipant.Role != string(constants.ChatRoleOwner) && actingParticipant.Role != string(constants.ChatRoleAdmin) {
		return nil, fmt.Errorf("%w: only owners and admins can remove members", ErrPermissionDenied)
	}

	// Only the owner may remove an owner (#28). A request that names an owner
	// is refused as a whole, so nobody is half-removed.
	actorIsOwner := actingParticipant.Role == string(constants.ChatRoleOwner)
	actorRoles := []string{string(constants.ChatRoleOwner), string(constants.ChatRoleAdmin)}
	if actorIsOwner {
		actorRoles = []string{string(constants.ChatRoleOwner)}
	}

	// The update filter repeats the checks made on the read above, so a
	// demotion or a removal between the read and the write can't be ignored.
	updateFilter := bson.M{
		"_id":      chatID,
		"chatType": string(constants.ChatTypeGroup),
		"participants." + actingUserID.Hex() + ".role": bson.M{"$in": actorRoles},
	}
	unsetEntries := bson.M{}
	var removedIDs []bson.ObjectID

	for _, targetID := range targetUserIDs {
		target, isParticipant := chat.Participants[targetID]
		if !isParticipant {
			continue // Skip users not in the chat
		}
		if targetID == actingUserID {
			continue // Cannot remove self
		}
		// ownerId also counts: groups from before #18 may not have the owner role
		// backfilled on their creator.
		isOwner := target.Role == string(constants.ChatRoleOwner) || targetID == chat.OwnerID
		if isOwner && !actorIsOwner {
			return nil, fmt.Errorf("%w: only the owner can remove an owner", ErrPermissionDenied)
		}
		key := "participants." + targetID.Hex()
		if _, seen := unsetEntries[key]; seen {
			continue // Listed twice
		}
		updateFilter[key] = bson.M{"$exists": true}
		if !actorIsOwner {
			updateFilter[key+".role"] = bson.M{"$ne": string(constants.ChatRoleOwner)}
		}
		unsetEntries[key] = ""
		removedIDs = append(removedIDs, targetID)
	}

	if len(unsetEntries) == 0 {
		return nil, fmt.Errorf("%w", ErrNoValidParticipants)
	}
	if !actorIsOwner {
		updateFilter["ownerId"] = bson.M{"$nin": removedIDs}
	}

	chatUpdate := bson.M{
		"$unset": unsetEntries,
		"$set": bson.M{
			"updatedAt": time.Now(),
		},
	}

	res, err := database.DB.Collection(string(constants.ChatColl)).UpdateOne(ctx, updateFilter, chatUpdate)
	if err != nil {
		return nil, fmt.Errorf("failed to update chat: %w", err)
	}
	if res.MatchedCount == 0 {
		return nil, fmt.Errorf("%w: chat changed while removing members", ErrPermissionDenied)
	}

	// Only the users actually removed lose their chat ref; the actor and
	// skipped targets keep theirs (#28).
	userFilter := bson.M{"_id": bson.M{"$in": removedIDs}}
	userUpdate := bson.M{"$unset": bson.M{"chats." + chatID.Hex(): ""}}

	// Remove chat ref from each removed user. MatchedCount isn't checked: a
	// member whose account is gone matches nothing and must still be removable
	// from the chat (as with contacts).
	_, err = database.DB.Collection(string(constants.UserColl)).UpdateMany(ctx, userFilter, userUpdate)
	if err != nil {
		return nil, fmt.Errorf("failed to update users: %w", err)
	}

	return removedIDs, nil
}

// ================ Invites ================

// groupManagerRoles are the roles that may manage a group's invite codes:
// create, list, delete and send them, and see who joined by one (#30).
var groupManagerRoles = []string{string(constants.ChatRoleOwner), string(constants.ChatRoleAdmin)}

// requireGroupManager checks that userID is an owner or admin of the group
// chatID. It returns ErrChatNotFound when there is no such group or userID
// isn't in it, so group IDs can't be probed, and ErrPermissionDenied for any
// other role. Writes to the chat still re-check the role with
// groupManagerFilter, so a demotion between this read and the write can't be
// overridden.
func requireGroupManager(ctx context.Context, chatID, userID bson.ObjectID) error {
	var chat struct {
		Participants map[string]struct {
			Role string `bson:"role"`
		} `bson:"participants"`
	}
	err := database.DB.Collection(string(constants.ChatColl)).FindOne(ctx,
		bson.M{"_id": chatID, "chatType": string(constants.ChatTypeGroup)},
		options.FindOne().SetProjection(bson.M{"participants." + userID.Hex() + ".role": 1}),
	).Decode(&chat)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return fmt.Errorf("%w", ErrChatNotFound)
	}
	if err != nil {
		return fmt.Errorf("failed to get chat: %w", err)
	}

	participant, ok := chat.Participants[userID.Hex()]
	if !ok {
		return fmt.Errorf("%w", ErrChatNotFound)
	}
	for _, role := range groupManagerRoles {
		if participant.Role == role {
			return nil
		}
	}
	return fmt.Errorf("%w: only owners and admins can manage invites", ErrPermissionDenied)
}

// groupManagerFilter matches the group chatID only while userID is an owner or
// admin of it.
func groupManagerFilter(chatID, userID bson.ObjectID) bson.M {
	return bson.M{
		"_id":                                    chatID,
		"chatType":                               string(constants.ChatTypeGroup),
		"participants." + userID.Hex() + ".role": bson.M{"$in": groupManagerRoles},
	}
}

// CreateInvite adds a new invite code to a group and returns it, as stored.
// Only its owner and admins may.
func CreateInvite(ctx context.Context, userID bson.ObjectID, chatID bson.ObjectID) (models.InviteCodeEmbed, error) {
	if err := requireGroupManager(ctx, chatID, userID); err != nil {
		return models.InviteCodeEmbed{}, err
	}

	invite, ok := utils.NewInviteCodeWithDefaults(time.Now().Add(24 * time.Hour))
	if !ok {
		return models.InviteCodeEmbed{}, fmt.Errorf("failed to create invite code")
	}

	res, err := database.DB.Collection(string(constants.ChatColl)).UpdateOne(ctx,
		groupManagerFilter(chatID, userID),
		bson.M{"$push": bson.M{"inviteCode": invite}},
	)
	if err != nil {
		return models.InviteCodeEmbed{}, fmt.Errorf("failed to create invite code: %w", err)
	}
	if res.MatchedCount == 0 {
		return models.InviteCodeEmbed{}, fmt.Errorf("%w: role changed while creating the invite", ErrPermissionDenied)
	}

	return invite, nil
}

// GetGroupInvites lists every invite code of a group, deleted and expired ones
// included (each has deleted and expiredDate). Only its owner and admins may.
func GetGroupInvites(ctx context.Context, userID bson.ObjectID, chatID bson.ObjectID) ([]models.InviteCodeEmbed, error) {
	if err := requireGroupManager(ctx, chatID, userID); err != nil {
		return nil, err
	}

	chat := models.Chat{}
	err := database.DB.Collection(string(constants.ChatColl)).FindOne(ctx,
		groupManagerFilter(chatID, userID),
		options.FindOne().SetProjection(bson.M{"inviteCode": 1}),
	).Decode(&chat)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, fmt.Errorf("%w: role changed while listing invites", ErrPermissionDenied)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get group invites: %w", err)
	}

	return chat.InviteCode, nil
}

// DeleteInvite marks an invite code deleted, so nobody can join with it. Only
// the group's owner and admins may. A code that isn't stored is ErrInvalidInvite.
func DeleteInvite(ctx context.Context, userID bson.ObjectID, inviteID string) error {
	chatID, _, err := findInvite(ctx, inviteID)
	if err != nil {
		return err
	}
	if err := requireGroupManager(ctx, chatID, userID); err != nil {
		return err
	}

	filter := groupManagerFilter(chatID, userID)
	filter["inviteCode.inviteCode"] = inviteID

	res, err := database.DB.Collection(string(constants.ChatColl)).UpdateOne(ctx, filter,
		bson.M{"$set": bson.M{"inviteCode.$.deleted": true}},
	)
	if err != nil {
		return fmt.Errorf("failed to delete invite: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("%w: role changed while deleting the invite", ErrPermissionDenied)
	}

	return nil
}

// GetJoinedUsersByInvite lists who joined the group by an invite code. Only the
// group's owner and admins may see it.
func GetJoinedUsersByInvite(ctx context.Context, userID bson.ObjectID, inviteID string) ([]bson.ObjectID, error) {
	chatID, _, err := findInvite(ctx, inviteID)
	if err != nil {
		return nil, err
	}
	if err := requireGroupManager(ctx, chatID, userID); err != nil {
		return nil, err
	}

	filter := groupManagerFilter(chatID, userID)
	filter["inviteCode.inviteCode"] = inviteID

	chat := models.Chat{}

	// "inviteCode.$" returns only the entry that matched inviteID, not the
	// chat's first invite (#29).
	err = database.DB.Collection(string(constants.ChatColl)).FindOne(ctx, filter, options.FindOne().SetProjection(bson.M{"inviteCode.$": 1})).Decode(&chat)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, fmt.Errorf("%w: role changed while reading the invite", ErrPermissionDenied)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get joined users by invite: %w", err)
	}

	if len(chat.InviteCode) == 0 || chat.InviteCode[0].UserIDs == nil {
		return []bson.ObjectID{}, nil
	}

	return chat.InviteCode[0].UserIDs, nil
}

// SendInviteToUser gives targetUserID a pending invitation for an invite code.
// Only the group's owner and admins may send one. The code must be live, and the
// target must exist and not be in the group yet. Sending the same code again adds
// nothing. The invitation is stored on the user, not the chat, so unlike the
// other invite writes its write can't re-check the sender's role; that needs a
// transaction (#37).
func SendInviteToUser(ctx context.Context, actingUserID bson.ObjectID, inviteID string, targetUserID bson.ObjectID) error {
	chatID, invite, err := findInvite(ctx, inviteID)
	if err != nil {
		return err
	}

	// A deleted or expired code can't be sent (#91).
	if invite.Deleted || invite.ExpiredDate.Before(time.Now()) {
		return fmt.Errorf("%w", ErrInvalidInvite)
	}

	if err := requireGroupManager(ctx, chatID, actingUserID); err != nil {
		return err
	}

	// A pending member can be sent a code: joining with it is how they accept (#103).
	inGroup, err := database.DB.Collection(string(constants.ChatColl)).CountDocuments(ctx, bson.M{
		"_id":                          chatID,
		participantField(targetUserID): bson.M{"$exists": true},
		participantField(targetUserID) + ".requestStatus": bson.M{"$ne": string(constants.StatusPending)},
	})
	if err != nil {
		return fmt.Errorf("failed to check the target user: %w", err)
	}
	if inGroup > 0 {
		return fmt.Errorf("%w", ErrAlreadyParticipant)
	}

	found, err := addInvitation(ctx, inviteID, targetUserID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w", ErrUserNotFound)
	}

	return nil
}

// addInvitation gives targetUserID a pending invitation for inviteID, with no
// permission checks, and reports whether the user exists. Sending the same code
// again adds nothing. Callers check who may send it.
func addInvitation(ctx context.Context, inviteID string, targetUserID bson.ObjectID) (bool, error) {
	res, err := database.DB.Collection(string(constants.UserColl)).UpdateOne(ctx,
		bson.M{"_id": targetUserID}, addInvitationUpdate(inviteID))
	if err != nil {
		return false, fmt.Errorf("failed to update target user: %w", err)
	}
	return res.MatchedCount > 0, nil
}

// addInvitationUpdate gives a user a pending invitation for inviteID; sent again,
// it adds nothing.
func addInvitationUpdate(inviteID string) bson.M {
	return bson.M{"$addToSet": bson.M{"chatInvitations": models.ChatInvitationEmbed{Token: inviteID, Status: "pending"}}}
}

// joinOutcome is what a join's transaction did, acted on once it has committed:
// an error inside the transaction would roll back the repair it made (#37).
type joinOutcome struct {
	chatID  bson.ObjectID
	follow  bool // the user's open connections should follow the chat (#49)
	already bool // they were a member already: the answer is ErrAlreadyParticipant
}

// finish acts on a committed join: open connections follow the chat, and a
// member who was already one gets ErrAlreadyParticipant.
func (o joinOutcome) finish(ctx context.Context, userID bson.ObjectID) error {
	if o.follow {
		AddMembersToLiveChat(ctx, o.chatID, userID) // #49
	}
	if o.already {
		return fmt.Errorf("%w", ErrAlreadyParticipant)
	}
	return nil
}

// runJoin runs join in a transaction and, once it commits, acts on its outcome.
func runJoin(ctx context.Context, userID bson.ObjectID, join func(ctx context.Context) (joinOutcome, error)) error {
	var out joinOutcome
	err := database.WithTransaction(ctx, func(ctx context.Context) error {
		var err error
		out, err = join(ctx)
		return err
	})
	if err != nil {
		return err
	}
	return out.finish(ctx, userID)
}

// JoinGroupByInvite adds userID to the group that stores the invite code. The
// code must not be deleted or past its stored expiry, and the chat must be a
// group. The group and the user change in one transaction (#37), the group
// first.
func JoinGroupByInvite(ctx context.Context, userID bson.ObjectID, inviteID string) error {
	return runJoin(ctx, userID, func(ctx context.Context) (joinOutcome, error) {
		return joinGroupByInvite(ctx, userID, inviteID)
	})
}

func joinGroupByInvite(ctx context.Context, userID bson.ObjectID, inviteID string) (joinOutcome, error) {
	chatObjectID, _, err := findInvite(ctx, inviteID)
	if err != nil {
		return joinOutcome{}, err
	}

	user, err := GetUserBasicInfo(ctx, userID)
	if err != nil {
		return joinOutcome{}, fmt.Errorf("failed to get user: %w", err)
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

	now := time.Now()
	joinFilter := usableInviteFilter(chatObjectID, inviteID, now)
	joinFilter[participantField(userID)] = bson.M{"$exists": false}
	joinFilter["$expr"] = roomForParticipants(1)
	res, err := database.DB.Collection(string(constants.ChatColl)).UpdateOne(ctx, joinFilter, updateInviteCodeUserIDs)
	if err != nil {
		return joinOutcome{}, fmt.Errorf("failed to update invite code user IDs: %w", err)
	}
	if res.MatchedCount == 0 {
		// Not a new member. A pending member (#5) accepts here (#103); a member
		// gets their chats entry repaired; anyone else gets the reason the join
		// didn't apply.
		out, err := resolveMissedJoin(ctx, chatObjectID, userID, inviteID, now)
		if err != nil || out.already {
			return out, err
		}
	}

	if err := finishGroupJoin(ctx, chatObjectID, userID); err != nil {
		return joinOutcome{}, err
	}
	return joinOutcome{chatID: chatObjectID, follow: true}, nil
}

// finishGroupJoin completes userID becoming an accepted member of the group chatID,
// by a code or by accepting their invite (#5): the chat goes on their chats list,
// their invitations to the group are dropped as used (#30, #103). $max adds a
// missing entry as 0 and keeps an unread count that is already there. It runs in
// the join's transaction; the caller makes their open connections follow the
// chat once it commits (#49, joinOutcome).
func finishGroupJoin(ctx context.Context, chatID, userID bson.ObjectID) error {
	codes, err := groupInviteCodes(ctx, chatID)
	if err != nil {
		return err
	}
	userRes, err := database.DB.Collection(string(constants.UserColl)).UpdateOne(ctx,
		bson.M{"_id": userID},
		bson.M{
			"$max":  bson.M{"chats." + chatID.Hex(): 0},
			"$pull": bson.M{"chatInvitations": bson.M{"token": bson.M{"$in": codes}}},
		},
	)
	if err != nil {
		return fmt.Errorf("failed to add chat to user: %w", err)
	}
	if userRes.MatchedCount == 0 {
		return fmt.Errorf("add chat to user: %w", ErrUserNotFound)
	}
	return nil
}

// acceptPendingMember makes userID, a pending member of the group chatID, an
// accepted one, if inviteID is a usable code of that group at now (#103). The
// member already counts toward the size limit and participantCount, so neither
// changes. It reports whether the member was accepted.
func acceptPendingMember(ctx context.Context, chatID, userID bson.ObjectID, inviteID string, now time.Time) (bool, error) {
	filter := usableInviteFilter(chatID, inviteID, now)
	filter[participantField(userID)+".requestStatus"] = string(constants.StatusPending)
	res, err := database.DB.Collection(string(constants.ChatColl)).UpdateOne(ctx, filter, bson.M{
		"$set":      acceptedFields(userID, now),
		"$addToSet": bson.M{"inviteCode.$.userIDs": userID},
	})
	if err != nil {
		return false, fmt.Errorf("failed to accept pending member: %w", err)
	}
	return res.MatchedCount == 1, nil
}

// usableInviteFilter matches the group chatID while it stores inviteID as a code
// that is neither deleted nor expired.
func usableInviteFilter(chatID bson.ObjectID, inviteID string, now time.Time) bson.M {
	return bson.M{
		"_id":      chatID,
		"chatType": string(constants.ChatTypeGroup),
		"inviteCode": bson.M{"$elemMatch": bson.M{
			"inviteCode":  inviteID,
			"deleted":     bson.M{"$ne": true},
			"expiredDate": bson.M{"$gt": now},
		}},
	}
}

// resolveMissedJoin handles a join whose update matched nothing, from one read of
// the group. A pending member with a code usable at now (the time the join's
// filter used) is accepted, and it returns no error (#103). A member already in
// the group gets an outcome with already set: a join from before the
// transactions (#37) could stop after adding them to participants but before
// adding the chat to their chats list, which the hub needs for live messages,
// so a missing entry is added here (an existing one keeps its unread count) and
// the caller answers ErrAlreadyParticipant after the commit. Otherwise it
// returns why:
//   - ErrInvalidInvite: there is no such group, or the code isn't usable.
//   - ErrGroupFull: a new member wouldn't fit (#68).
//   - a plain error: the read and the join's filter disagree. Inside the
//     transaction a concurrent change makes the write conflict and rerun, so
//     this means a bug, not a race, and a retry won't help.
func resolveMissedJoin(ctx context.Context, chatID, userID bson.ObjectID, inviteID string, now time.Time) (joinOutcome, error) {
	var chat models.Chat
	err := database.DB.Collection(string(constants.ChatColl)).FindOne(ctx,
		bson.M{"_id": chatID, "chatType": string(constants.ChatTypeGroup)},
		options.FindOne().SetProjection(bson.M{"participants": 1, "settings.maxParticipants": 1, "inviteCode": 1}),
	).Decode(&chat)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return joinOutcome{}, fmt.Errorf("%w", ErrInvalidInvite)
	}
	if err != nil {
		return joinOutcome{}, fmt.Errorf("failed to check the group: %w", err)
	}

	usable := slices.ContainsFunc(chat.InviteCode, func(c models.InviteCodeEmbed) bool {
		return c.InviteCode == inviteID && !c.Deleted && c.ExpiredDate.After(now)
	})
	participant, member := chat.Participants[userID]
	switch {
	case member && participant.RequestStatus == string(constants.StatusPending):
		if !usable {
			return joinOutcome{}, fmt.Errorf("%w", ErrInvalidInvite)
		}
		accepted, err := acceptPendingMember(ctx, chatID, userID, inviteID, now)
		if err != nil {
			return joinOutcome{}, err
		}
		if !accepted {
			return joinOutcome{}, fmt.Errorf("accept did not apply although the read found a pending member")
		}
		return joinOutcome{}, nil
	case member:
		chatRef := "chats." + chatID.Hex()
		if _, err := database.DB.Collection(string(constants.UserColl)).UpdateOne(ctx,
			bson.M{"_id": userID, chatRef: bson.M{"$exists": false}},
			bson.M{"$set": bson.M{chatRef: 0}},
		); err != nil {
			return joinOutcome{}, fmt.Errorf("failed to restore chat ref of user: %w", err)
		}
		// Their connections follow the chat again, even when the entry was
		// there: a join whose commit succeeded but answered with an error never
		// got to follow it (#49). Following twice changes nothing.
		return joinOutcome{chatID: chatID, follow: true, already: true}, nil
	case !usable:
		return joinOutcome{}, fmt.Errorf("%w", ErrInvalidInvite)
	case len(chat.Participants)+1 > groupSizeLimit(chat.Settings.MaxParticipants):
		return joinOutcome{}, fmt.Errorf("%w", ErrGroupFull)
	default:
		return joinOutcome{}, fmt.Errorf("join did not apply although the read found room and a usable code")
	}
}

// groupInviteCodes returns every invite code the group chatID stores, deleted and
// expired ones included, so a join can clear all of the user's invitations to it.
func groupInviteCodes(ctx context.Context, chatID bson.ObjectID) ([]string, error) {
	var chat struct {
		InviteCode []struct {
			InviteCode string `bson:"inviteCode"`
		} `bson:"inviteCode"`
	}
	err := database.DB.Collection(string(constants.ChatColl)).FindOne(ctx,
		bson.M{"_id": chatID},
		options.FindOne().SetProjection(bson.M{"inviteCode.inviteCode": 1}),
	).Decode(&chat)
	if err != nil {
		return nil, fmt.Errorf("failed to read the group's invite codes: %w", err)
	}
	codes := make([]string, 0, len(chat.InviteCode))
	for _, c := range chat.InviteCode {
		codes = append(codes, c.InviteCode)
	}
	return codes, nil
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
