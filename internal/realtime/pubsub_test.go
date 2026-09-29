package realtime

import (
	"io"
	"testing"

	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/models"
)

// A slow client used to make pub/sub close its Send channel, and the hub's
// unregister path then closed it again: "close of closed channel" crashed the
// whole server. Both paths must now be safe together.
func TestSlowClientThenUnregisterDoesNotPanic(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	userID := bson.NewObjectID()
	client := &models.Client{ID: "c1", UserID: userID, Send: make(chan models.WebSocketMessage, 1)}
	client.TrySend(models.WebSocketMessage{}) // fill the buffer

	hub := &models.Hub{
		Clients:     map[string]*models.Client{client.ID: client},
		ChatClients: map[string]map[string]*models.Client{"chat1": {client.ID: client}},
		UserClients: map[string]map[string]*models.Client{userID.Hex(): {client.ID: client}},
		Unregister:  make(chan *models.Client, 4),
	}
	pm := NewPubSubManager(nil, hub, logger, "test")

	pm.handleChatMessage("chat1", &models.WebSocketMessage{Type: "message"})
	pm.handleUserMessage(userID.Hex(), &models.WebSocketMessage{Type: "message"})

	if got := len(hub.Unregister); got != 2 {
		t.Fatalf("expected slow client queued for unregister twice, got %d", got)
	}

	// What the hub does on unregister, plus a late broadcast: neither may panic.
	client.CloseSend()
	pm.broadcastToAllClients(&models.WebSocketMessage{Type: "system"})
	pm.handleChatMessage("chat1", &models.WebSocketMessage{Type: "message"})
}
