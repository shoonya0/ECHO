package controller

import (
	"slices"
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/services"
	"github.com/shoonya0/ECHO/internal/testutil"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// removingBroadcaster records events and live removals in the order they happen.
type removingBroadcaster struct{ calls []string }

func (b *removingBroadcaster) PublishToChat(chatID string, message *models.WebSocketMessage) error {
	b.calls = append(b.calls, "publish "+message.Type)
	return nil
}

func (b *removingBroadcaster) RemoveMembersFromChat(chatID string, userIDs []string) error {
	sorted := slices.Clone(userIDs)
	slices.Sort(sorted)
	call := "remove " + chatID
	for _, id := range sorted {
		call += " " + id
	}
	b.calls = append(b.calls, call)
	return nil
}

// #14: remove_user must cut the removed members' open connections out of the
// group, after the chat_updated event that tells them, and only them: the owner
// named in the request isn't removed, so isn't cut off.
func TestRemoveUserCutsRemovedMembersOffLive(t *testing.T) {
	ctx := testutil.UseTestDB(t)
	broadcaster := &removingBroadcaster{}
	services.SetBroadcaster(broadcaster)
	t.Cleanup(func() { services.SetBroadcaster(nil) })

	owner, gone := bson.NewObjectID(), bson.NewObjectID()
	chatID := bson.NewObjectID()
	member := bson.M{"role": string(constants.ChatRoleMember), "requestStatus": "accepted"}
	if _, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{
		"_id": chatID, "chatType": string(constants.ChatTypeGroup), "ownerId": owner,
		"participants": bson.M{
			owner.Hex(): bson.M{"role": string(constants.ChatRoleOwner), "requestStatus": "accepted"},
			gone.Hex():  member,
		},
		"stats": bson.M{"participantCount": 2},
	}); err != nil {
		t.Fatalf("insert chat: %v", err)
	}

	client := &models.Client{ID: "c1", UserID: owner, Send: make(chan models.WebSocketMessage, 4)}
	handleRemoveUser(ctx, client, models.MessageRequest{
		ChatID: chatID.Hex(), RequestID: "r1",
		Metadata: map[string]any{"userIds": []any{gone.Hex(), owner.Hex()}},
	})

	if reply := <-client.Send; reply.Type != models.WSMessageTypeResponse {
		t.Fatalf("reply = %+v, want a success response", reply.Data)
	}
	want := []string{"publish " + models.WSMessageTypeChatUpdated, "remove " + chatID.Hex() + " " + gone.Hex()}
	if !slices.Equal(broadcaster.calls, want) {
		t.Fatalf("calls = %q, want %q", broadcaster.calls, want)
	}
}
