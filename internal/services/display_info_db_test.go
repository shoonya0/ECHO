package services

import (
	"context"
	"errors"
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func insertProfile(t *testing.T, ctx context.Context, username, displayName, avatar string) bson.ObjectID {
	t.Helper()
	id := bson.NewObjectID()
	if _, err := database.DB.Collection(string(constants.UserColl)).InsertOne(ctx, bson.M{
		"_id": id, "username": username,
		"profile": bson.M{"displayName": displayName, "avatar": avatar},
	}); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

// Message history shows each sender's current name and avatar, not the copy
// saved with the message (#36). A sender whose account is gone shows as
// "Deleted user".
func TestGetChatMessagesShowsCurrentSender(t *testing.T) {
	ctx := useTestDB(t)
	alice := insertProfile(t, ctx, "alice_new", "Alice New", "new.png")
	gone := bson.NewObjectID()
	chatID := bson.NewObjectID()
	if _, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{
		"_id":          chatID,
		"participants": bson.M{alice.Hex(): bson.M{"status": "accepted"}},
	}); err != nil {
		t.Fatalf("insert chat: %v", err)
	}
	stale := bson.M{"username": "alice", "displayName": "Alice Old", "avatar": "old.png"}
	for _, sender := range []bson.ObjectID{alice, alice, gone} {
		if _, err := database.DB.Collection(string(constants.MessageColl)).InsertOne(ctx, bson.M{
			"chatId": chatID, "senderId": sender, "sender": stale, "content": "hi",
		}); err != nil {
			t.Fatalf("insert message: %v", err)
		}
	}

	// Reactions and mentions keep usernames too.
	if _, err := database.DB.Collection(string(constants.MessageColl)).InsertOne(ctx, bson.M{
		"chatId": chatID, "senderId": alice, "sender": stale, "content": "@alice",
		"reactions": bson.M{"x": bson.M{"count": 2, "users": bson.A{alice, gone},
			"details": bson.M{alice.Hex(): "alice", gone.Hex(): "ghost"}}},
		"mentions": bson.M{"userIds": bson.A{alice}, "userNames": bson.A{"alice"}},
	}); err != nil {
		t.Fatalf("insert message: %v", err)
	}

	msgs, _, err := GetChatMessages(ctx, alice, chatID, 10, nil)
	if err != nil {
		t.Fatalf("GetChatMessages: %v", err)
	}
	if len(msgs) != 4 {
		t.Fatalf("got %d messages, want 4", len(msgs))
	}
	// Newest first: the one with reactions and mentions.
	if got := msgs[0].Reactions["x"].Details; len(got) != 2 || got[alice.Hex()] != "alice_new" || got[gone.Hex()] != "" {
		t.Errorf("reaction details %v, want alice_new and an empty name for the deleted user", got)
	}
	if got := msgs[0].Mentions.UserNames; len(got) != 1 || got[0] != "alice_new" {
		t.Errorf("mention names %v, want [alice_new]", got)
	}
	want := map[bson.ObjectID]models.MessageSenderEmbed{
		alice: {UserID: alice, Username: "alice_new", DisplayName: "Alice New", Avatar: "new.png"},
		gone:  {UserID: gone, DisplayName: DeletedUserName},
	}
	for _, m := range msgs {
		if m.Sender != want[m.SenderID] {
			t.Errorf("message %s: sender %+v, want %+v", m.ID.Hex(), m.Sender, want[m.SenderID])
		}
	}
}

// A chat sent to a client carries its participants' current names (#36).
func TestFillParticipantInfo(t *testing.T) {
	ctx := useTestDB(t)
	bob := insertProfile(t, ctx, "bob_new", "Bob New", "bob.png")
	gone := bson.NewObjectID()
	chat := &models.Chat{Participants: map[bson.ObjectID]models.ParticipantEmbed{
		bob:  {Role: "member", UserInfo: models.ContactUserInfo{UserID: bob, Username: "bob", DisplayName: "Bob Old"}},
		gone: {Role: "member", UserInfo: models.ContactUserInfo{UserID: gone, Username: "ghost"}},
	}}

	if err := FillParticipantInfo(ctx, chat); err != nil {
		t.Fatalf("FillParticipantInfo: %v", err)
	}
	if got := chat.Participants[bob]; got.Role != "member" ||
		got.UserInfo != (models.ContactUserInfo{UserID: bob, Username: "bob_new", DisplayName: "Bob New", Avatar: "bob.png"}) {
		t.Errorf("bob: %+v", got)
	}
	if got := chat.Participants[gone].UserInfo; got != (models.ContactUserInfo{UserID: gone, DisplayName: DeletedUserName}) {
		t.Errorf("deleted participant: %+v", got)
	}
}

// The creator's entry in a new group or direct chat is copied from the database,
// not from the JWT, which has the name from login and no avatar (#36).
func TestNewChatsStoreTheCreatorsCurrentName(t *testing.T) {
	ctx := useTestDB(t)
	creator := insertProfile(t, ctx, "carol_new", "Carol New", "carol.png")
	other := insertProfile(t, ctx, "dave", "Dave", "")
	asStaleToken := context.WithValue(ctx, constants.UserDataKey,
		models.LoginUserResponse{ID: creator, Username: "carol", Profile: models.UserProfileEmbed{DisplayName: "Carol Old"}})
	want := models.ContactUserInfo{UserID: creator, Username: "carol_new", DisplayName: "Carol New", Avatar: "carol.png"}

	stored := func(t *testing.T, chatID bson.ObjectID) models.ContactUserInfo {
		t.Helper()
		var chat models.Chat
		if err := database.DB.Collection(string(constants.ChatColl)).FindOne(ctx, bson.M{"_id": chatID}).Decode(&chat); err != nil {
			t.Fatalf("read chat: %v", err)
		}
		return chat.Participants[creator].UserInfo
	}

	group, err := CreateGroupChat(asStaleToken, "g", "", []bson.ObjectID{other})
	if err != nil {
		t.Fatalf("CreateGroupChat: %v", err)
	}
	if got := stored(t, group.ChatID); got != want {
		t.Errorf("group creator entry %+v, want %+v", got, want)
	}

	direct, err := CreateDirectChat(asStaleToken, other)
	if err != nil {
		t.Fatalf("CreateDirectChat: %v", err)
	}
	if got := stored(t, direct.ChatID); got != want {
		t.Errorf("direct chat creator entry %+v, want %+v", got, want)
	}
}

// An account that is gone has no name to copy: creating a chat as it fails
// instead of storing a blank "Deleted user" creator (code review of #36).
func TestWithCurrentDisplayInfoMissingAccount(t *testing.T) {
	ctx := useTestDB(t)
	gone := bson.NewObjectID()
	other := insertProfile(t, ctx, "erin", "Erin", "")

	if _, err := WithCurrentDisplayInfo(ctx, models.LoginUserResponse{ID: gone, Username: "ghost"}); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("WithCurrentDisplayInfo: err %v, want ErrUserNotFound", err)
	}
	asGone := context.WithValue(ctx, constants.UserDataKey, models.LoginUserResponse{ID: gone, Username: "ghost"})
	if _, err := CreateDirectChat(asGone, other); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("CreateDirectChat: err %v, want ErrUserNotFound", err)
	}
	if n, err := database.DB.Collection(string(constants.ChatColl)).CountDocuments(ctx, bson.M{}); err != nil || n != 0 {
		t.Fatalf("chats stored: %d (err %v), want 0", n, err)
	}
}
