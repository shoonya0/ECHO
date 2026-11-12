package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/realtime"
)

// A connection whose session ended must get a close frame that says why, and
// then be closed by the server (#76).
func TestRevokedClientGetsCloseFrameWithReason(t *testing.T) {
	ready := make(chan *models.Client, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		client := &models.Client{ID: "c1", UserID: bson.NewObjectID(), Connection: conn, Send: make(chan models.WebSocketMessage, 1)}
		ready <- client
		handleClientWrite(client)
	}))
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	client := <-ready
	client.Revoke(realtime.CloseCodeForbidden, realtime.ReasonAccountBanned)

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, err = conn.ReadMessage()
	var closeErr *websocket.CloseError
	if !errors.As(err, &closeErr) {
		t.Fatalf("read returned %v, want a close frame", err)
	}
	if closeErr.Code != realtime.CloseCodeForbidden || closeErr.Text != realtime.ReasonAccountBanned {
		t.Fatalf("close frame %d %q, want %d %q", closeErr.Code, closeErr.Text, realtime.CloseCodeForbidden, realtime.ReasonAccountBanned)
	}
}
