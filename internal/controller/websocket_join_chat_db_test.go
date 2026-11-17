package controller

import (
	"context"
	"strings"
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/testutil"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// stubJoinChatRoom records the rooms handleJoinChat joins instead of using the hub.
func stubJoinChatRoom(t *testing.T) *[]string {
	t.Helper()
	var joined []string
	orig := joinChatRoom
	joinChatRoom = func(_ context.Context, _ *models.Client, chatID string) error {
		joined = append(joined, chatID)
		return nil
	}
	t.Cleanup(func() { joinChatRoom = orig })
	return &joined
}

// readReply returns the one reply handleClientRequest queued for the client.
func readReply(t *testing.T, client *models.Client) models.WebSocketMessage {
	t.Helper()
	select {
	case reply := <-client.Send:
		return reply
	default:
		t.Fatal("no reply sent")
		return models.WebSocketMessage{}
	}
}

// wantJoinResponse checks reply is a success response naming chatID.
func wantJoinResponse(t *testing.T, reply models.WebSocketMessage, chatID string) {
	t.Helper()
	resp, ok := reply.Data.(models.MessageResponse)
	if reply.Type != models.WSMessageTypeResponse || !ok || !resp.Success {
		t.Fatalf("reply = %+v, want a success response", reply)
	}
	if got := resp.Data["chatId"]; got != chatID {
		t.Errorf("response chatId = %v, want %s", got, chatID)
	}
	if reply.RequestID != "r1" {
		t.Errorf("requestId = %q, want r1", reply.RequestID)
	}
}

// #48: join_chat follows the chat named by chatId when the caller is a member,
// opens a direct chat by senderId as before, and answers both with a response
// that names the chat.
func TestJoinChat(t *testing.T) {
	ctx := testutil.UseTestDB(t)
	joined := stubJoinChatRoom(t)

	me, other := bson.NewObjectID(), bson.NewObjectID()
	for _, id := range []bson.ObjectID{me, other} {
		if _, err := database.DB.Collection(string(constants.UserColl)).InsertOne(ctx, bson.M{"_id": id, "username": id.Hex()}); err != nil {
			t.Fatalf("insert user: %v", err)
		}
	}
	groupID := bson.NewObjectID()
	if _, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{
		"_id": groupID, "chatType": string(constants.ChatTypeGroup),
		"participants": bson.M{me.Hex(): bson.M{"role": string(constants.ChatRoleOwner), "requestStatus": "accepted"}},
	}); err != nil {
		t.Fatalf("insert group: %v", err)
	}
	userCtx := context.WithValue(ctx, constants.UserDataKey, models.LoginUserResponse{ID: me, Username: me.Hex()})
	newClient := func() *models.Client {
		return &models.Client{ID: "c1", UserID: me, Send: make(chan models.WebSocketMessage, 4)}
	}

	t.Run("a group by chatId", func(t *testing.T) {
		*joined = nil
		client := newClient()
		// Upper case, as some clients send it; the room key is lower case.
		handleClientRequest(userCtx, client, models.MessageRequest{
			Type: models.WSRequestTypeJoinChat, ChatID: strings.ToUpper(groupID.Hex()), RequestID: "r1",
		})
		wantJoinResponse(t, readReply(t, client), groupID.Hex())
		if len(*joined) != 1 || (*joined)[0] != groupID.Hex() {
			t.Errorf("joined rooms %v, want [%s]", *joined, groupID.Hex())
		}
	})

	t.Run("a direct chat by senderId", func(t *testing.T) {
		*joined = nil
		client := newClient()
		handleClientRequest(userCtx, client, models.MessageRequest{
			Type: models.WSRequestTypeJoinChat, SenderID: other.Hex(), RequestID: "r1",
		})
		reply := readReply(t, client)
		if len(*joined) != 1 {
			t.Fatalf("joined rooms %v, want one (reply %+v)", *joined, reply)
		}
		wantJoinResponse(t, reply, (*joined)[0])
	})

	t.Run("neither chatId nor senderId", func(t *testing.T) {
		*joined = nil
		client := newClient()
		handleClientRequest(userCtx, client, models.MessageRequest{Type: models.WSRequestTypeJoinChat, RequestID: "r1"})
		errMsg, ok := readReply(t, client).Data.(models.ErrorMessage)
		if !ok || errMsg.Code != "INVALID_REQUEST" {
			t.Errorf("reply = %+v, want INVALID_REQUEST", errMsg)
		}
		if len(*joined) != 0 {
			t.Errorf("joined rooms %v, want none", *joined)
		}
	})
}
