package realtime

import (
	"errors"
	"testing"

	"github.com/shoonya0/ECHO/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestLeaveChat(t *testing.T) {
	t.Run("a client in the room is removed, whatever the database says", func(t *testing.T) {
		hub := newTestHub(t)
		client := &models.Client{ID: "in-room", UserID: bson.NewObjectID()}
		hub.ChatClients["chat1"] = map[string]*models.Client{client.ID: client}

		// newTestHub's Redis refuses every publish, so the leave event fails
		// after the client has been removed.
		if err := hub.LeaveChat(client, "chat1"); err == nil || errors.Is(err, ErrNotInChat) {
			t.Fatalf("err = %v, want the publish error", err)
		}
		if _, stillIn := hub.ChatClients["chat1"][client.ID]; stillIn {
			t.Fatal("client is still in the room")
		}
	})

	t.Run("a client outside the room publishes nothing", func(t *testing.T) {
		hub := newTestHub(t)
		outsider := &models.Client{ID: "outsider", UserID: bson.NewObjectID()}
		member := &models.Client{ID: "member", UserID: bson.NewObjectID()}
		hub.ChatClients["chat1"] = map[string]*models.Client{member.ID: member}

		if err := hub.LeaveChat(outsider, "chat1"); !errors.Is(err, ErrNotInChat) {
			t.Fatalf("err = %v, want ErrNotInChat", err)
		}
		if _, stillIn := hub.ChatClients["chat1"][member.ID]; !stillIn {
			t.Fatal("the member was removed from the room")
		}
	})
}
