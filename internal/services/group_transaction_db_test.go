package services

import (
	"context"
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/testutil"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// The group and message writes touch a chat and its users or messages; each
// now commits as a whole or not at all (#37). Every test here makes one write
// fail and checks that the ones before it were rolled back.

func asUser(ctx context.Context, id bson.ObjectID) context.Context {
	return context.WithValue(ctx, constants.UserDataKey, models.LoginUserResponse{ID: id, Username: id.Hex()})
}

func loadTestUser(t *testing.T, ctx context.Context, id bson.ObjectID) models.User {
	t.Helper()
	var u models.User
	if err := database.DB.Collection(string(constants.UserColl)).FindOne(ctx, bson.M{"_id": id}).Decode(&u); err != nil {
		t.Fatalf("load user: %v", err)
	}
	return u
}

func loadTestChat(t *testing.T, ctx context.Context, id bson.ObjectID) models.Chat {
	t.Helper()
	var c models.Chat
	if err := database.DB.Collection(string(constants.ChatColl)).FindOne(ctx, bson.M{"_id": id}).Decode(&c); err != nil {
		t.Fatalf("load chat: %v", err)
	}
	return c
}

// An invitation that failed after the group was saved returned 500 with the group
// left behind, so the client's retry made a second one.
func TestCreateGroupChatRollsBackOnFailedInvite(t *testing.T) {
	ctx := useTestDB(t)
	creator, invitee := bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, creator, invitee)

	testutil.FailCommand(t, ctx, "update", 1) // the first user write passes
	if _, err := CreateGroupChat(asUser(ctx, creator), "g", "", []bson.ObjectID{invitee}); err == nil {
		t.Fatal("CreateGroupChat succeeded, want the failed write's error")
	}

	n, err := database.DB.Collection(string(constants.ChatColl)).CountDocuments(ctx, bson.M{"ownerId": creator})
	if err != nil {
		t.Fatalf("count chats: %v", err)
	}
	if n != 0 {
		t.Errorf("groups left = %d, want 0", n)
	}
	for _, id := range []bson.ObjectID{creator, invitee} {
		u := loadTestUser(t, ctx, id)
		if len(u.Chats) != 0 || len(u.ChatInvitations) != 0 {
			t.Errorf("user %s keeps chats %v and invitations %v, want none", id.Hex(), u.Chats, u.ChatInvitations)
		}
	}
}

// A message was saved and reported as sent while the chat's lastMessageId and
// messageCount write failed unnoticed. The two now go together, whichever fails.
func TestSendMessageRollsBackTogether(t *testing.T) {
	ctx := useTestDB(t)
	sender := bson.NewObjectID()
	insertTestUsers(t, ctx, sender)
	chatID := bson.NewObjectID()
	if _, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{"_id": chatID}); err != nil {
		t.Fatalf("insert chat: %v", err)
	}

	t.Run("chat update fails", func(t *testing.T) {
		testutil.FailCommand(t, ctx, "update", 0)
		if _, err := SendMessage(ctx, chatID, sender, "hi", "text", nil, nil); err == nil {
			t.Fatal("SendMessage succeeded, want the failed chat update's error")
		}
		checkNoMessage(t, ctx, chatID)
	})
	// The chat write runs first, so this checks that it is rolled back.
	t.Run("message insert fails", func(t *testing.T) {
		testutil.FailCommand(t, ctx, "insert", 0)
		if _, err := SendMessage(ctx, chatID, sender, "hi", "text", nil, nil); err == nil {
			t.Fatal("SendMessage succeeded, want the failed insert's error")
		}
		checkNoMessage(t, ctx, chatID)
	})
}

// checkNoMessage fails if chatID has a message, counts one or points at one.
func checkNoMessage(t *testing.T, ctx context.Context, chatID bson.ObjectID) {
	t.Helper()
	n, err := database.DB.Collection(string(constants.MessageColl)).CountDocuments(ctx, bson.M{"chatId": chatID})
	if err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if n != 0 {
		t.Errorf("messages saved = %d, want 0", n)
	}
	chat := loadTestChat(t, ctx, chatID)
	if chat.Stats.MessageCount != 0 || !chat.LastMessageID.IsZero() {
		t.Errorf("chat counts %d messages, last %s; want 0 and none (rolled back)",
			chat.Stats.MessageCount, chat.LastMessageID.Hex())
	}
}

// A removal whose user write failed had already taken the members out of the chat.
func TestRemoveGroupMemberRollsBackOnFailedUserUpdate(t *testing.T) {
	ctx := useTestDB(t)
	owner, member := bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, owner, member)
	chat, err := CreateGroupChat(asUser(ctx, owner), "g", "", []bson.ObjectID{member})
	if err != nil {
		t.Fatalf("CreateGroupChat: %v", err)
	}

	testutil.FailCommand(t, ctx, "update", 1) // the chat write passes
	if _, err := RemoveGroupMember(ctx, owner, chat.ChatID, []bson.ObjectID{member}); err == nil {
		t.Fatal("RemoveGroupMember succeeded, want the failed write's error")
	}

	got := loadTestChat(t, ctx, chat.ChatID)
	if _, ok := got.Participants[member]; !ok {
		t.Error("member was removed from the chat, want the removal rolled back")
	}
	if got.Stats.ParticipantCount != 2 {
		t.Errorf("participantCount = %d, want 2", got.Stats.ParticipantCount)
	}
}

// An add whose chats-entry write failed kept the new members, only logging it.
func TestAddGroupMemberRollsBackOnFailedUserUpdate(t *testing.T) {
	ctx := useTestDB(t)
	owner, added := bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, owner, added)
	chat, err := CreateGroupChat(asUser(ctx, owner), "g", "", nil)
	if err != nil {
		t.Fatalf("CreateGroupChat: %v", err)
	}

	testutil.FailCommand(t, ctx, "update", 1) // the chat write passes
	if err := AddGroupMember(ctx, owner, chat.ChatID, []bson.ObjectID{added}); err == nil {
		t.Fatal("AddGroupMember succeeded, want the failed write's error")
	}

	got := loadTestChat(t, ctx, chat.ChatID)
	if _, ok := got.Participants[added]; ok {
		t.Error("the user was added to the chat, want the add rolled back")
	}
	if got.Stats.ParticipantCount != 1 {
		t.Errorf("participantCount = %d, want 1", got.Stats.ParticipantCount)
	}
}

// Groups created at once with the same people, each by a different creator,
// write the same users. They must all finish, not abort each other (#37).
func TestConcurrentGroupCreatesWithSharedUsers(t *testing.T) {
	ctx := useTestDB(t)
	users := make([]bson.ObjectID, 6)
	for i := range users {
		users[i] = bson.NewObjectID()
	}
	insertTestUsers(t, ctx, users...)

	// A livelock shows as the test context's deadline (10s) in an error.
	errs := make(chan error, len(users))
	concurrently(1, func() {
		for i, creator := range users {
			others := append(append([]bson.ObjectID{}, users[i+1:]...), users[:i]...)
			go func() {
				_, err := CreateGroupChat(asUser(ctx, creator), "g", "", others)
				errs <- err
			}()
		}
	})
	for range users {
		if err := <-errs; err != nil {
			t.Errorf("CreateGroupChat: %v", err)
		}
	}
	for _, id := range users {
		u := loadTestUser(t, ctx, id)
		if len(u.Chats) != len(users) {
			t.Errorf("user %s is on %d groups, want %d", id.Hex(), len(u.Chats), len(users))
		}
	}
}

// Sends to one chat at once all write the chat. Each must count once, and the
// chat's last message must be the newest, even when an older send commits last.
func TestConcurrentSendsToOneChat(t *testing.T) {
	ctx := useTestDB(t)
	sender := bson.NewObjectID()
	insertTestUsers(t, ctx, sender)
	chatID := bson.NewObjectID()
	if _, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{"_id": chatID}); err != nil {
		t.Fatalf("insert chat: %v", err)
	}

	const n = 20
	errs := collectErrors(n, func() error {
		_, err := SendMessage(ctx, chatID, sender, "hi", "text", nil, nil)
		return err
	})
	for _, err := range errs {
		if err != nil {
			t.Fatalf("SendMessage: %v", err)
		}
	}

	chat := loadTestChat(t, ctx, chatID)
	if chat.Stats.MessageCount != n {
		t.Errorf("messageCount = %d, want %d", chat.Stats.MessageCount, n)
	}
	var newest models.Message
	if err := database.DB.Collection(string(constants.MessageColl)).FindOne(ctx, bson.M{"chatId": chatID},
		options.FindOne().SetSort(bson.M{"_id": -1})).Decode(&newest); err != nil {
		t.Fatalf("load newest message: %v", err)
	}
	if chat.LastMessageID != newest.ID {
		t.Errorf("lastMessageId = %s, want the newest message %s", chat.LastMessageID.Hex(), newest.ID.Hex())
	}
}
