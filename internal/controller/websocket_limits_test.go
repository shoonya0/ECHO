package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/models"
)

// serveReadLoop starts a server that runs both loops for each connection, as
// HandleWebSocketChat does, and dials it.
func serveReadLoop(t *testing.T) *websocket.Conn {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		client := &models.Client{ID: "c1", UserID: bson.NewObjectID(), Connection: conn, Send: make(chan models.WebSocketMessage, 10)}
		go handleClientWrite(client)
		handleClientRead(r.Context(), client)
	}))
	t.Cleanup(server.Close)

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// A frame over the size limit must close the connection with 1009 before the
// server buffers or decodes it (#40).
func TestOversizedFrameClosesTheConnection(t *testing.T) {
	conn := serveReadLoop(t)

	big := `{"type":"send_message","content":"` + strings.Repeat("a", maxFrameBytes) + `"}`
	if err := conn.WriteMessage(websocket.TextMessage, []byte(big)); err != nil {
		t.Fatalf("write: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var err error
	for err == nil {
		_, _, err = conn.ReadMessage()
	}
	var closeErr *websocket.CloseError
	if !errors.As(err, &closeErr) || closeErr.Code != websocket.CloseMessageTooBig {
		t.Fatalf("read ended with %v, want close frame %d", err, websocket.CloseMessageTooBig)
	}
}

// A frame at the limit is still read and answered.
func TestFrameAtTheLimitIsAnswered(t *testing.T) {
	conn := serveReadLoop(t)

	prefix, suffix := `{"type":"no_such_request","requestId":"r1","content":"`, `"}`
	frame := prefix + strings.Repeat("a", maxFrameBytes-len(prefix)-len(suffix)) + suffix
	if err := conn.WriteMessage(websocket.TextMessage, []byte(frame)); err != nil {
		t.Fatalf("write: %v", err)
	}

	reply := readError(t, conn)
	if reply.RequestID != "r1" || reply.Code != "UNKNOWN_REQUEST" {
		t.Fatalf("got %q %q, want r1 UNKNOWN_REQUEST", reply.RequestID, reply.Code)
	}
}

// Requests over the rate limit get RATE_LIMITED with their requestId and are
// not handled; the connection stays open (#40).
func TestRequestsOverTheRateLimitAreRefused(t *testing.T) {
	conn := serveReadLoop(t)

	// One request at a time, so the server's queue of 10 never drops a reply. The
	// round trips take a few milliseconds, which refills at most a token or two.
	const slack = 5
	firstLimited := -1
	for i := range requestBurst + slack + 1 {
		id := fmt.Sprintf("r%d", i)
		reply := roundTrip(t, conn, id)
		if reply.RequestID != id {
			t.Fatalf("request %s answered as %q", id, reply.RequestID)
		}
		if reply.Code == "RATE_LIMITED" {
			firstLimited = i
			break
		}
		if reply.Code != "UNKNOWN_REQUEST" {
			t.Fatalf("request %s got %q", id, reply.Code)
		}
	}
	if firstLimited < requestBurst {
		t.Fatalf("first RATE_LIMITED at request %d, want at least the burst of %d", firstLimited, requestBurst)
	}

	// Still open: after the bucket refills, a request is handled again.
	time.Sleep(200 * time.Millisecond)
	if reply := roundTrip(t, conn, "after"); reply.Code != "UNKNOWN_REQUEST" {
		t.Fatalf("after refill got %q, want UNKNOWN_REQUEST", reply.Code)
	}
}

// roundTrip sends a request the server doesn't know and reads the reply.
func roundTrip(t *testing.T, conn *websocket.Conn, requestID string) errorReply {
	t.Helper()
	frame := fmt.Sprintf(`{"type":"no_such_request","requestId":%q}`, requestID)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(frame)); err != nil {
		t.Fatalf("write %s: %v", requestID, err)
	}
	return readError(t, conn)
}

type errorReply struct {
	RequestID string
	Code      string
}

func readError(t *testing.T, conn *websocket.Conn) errorReply {
	t.Helper()
	reply, ok := tryReadError(conn)
	if !ok {
		t.Fatalf("no error reply")
	}
	return reply
}

// tryReadError reads one error event, waiting at most 500 ms.
func tryReadError(conn *websocket.Conn) (errorReply, bool) {
	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	var msg struct {
		Type      string `json:"type"`
		RequestID string `json:"requestId"`
		Data      struct {
			Code string `json:"code"`
		} `json:"data"`
	}
	if err := conn.ReadJSON(&msg); err != nil || msg.Type != models.WSMessageTypeError {
		return errorReply{}, false
	}
	return errorReply{RequestID: msg.RequestID, Code: msg.Data.Code}, true
}

// The bucket allows a burst, then one request per 1/rate seconds.
func TestRequestLimiterRefillsAtItsRate(t *testing.T) {
	start := time.Unix(0, 0)
	l := newRequestLimiter(10, 3, start)

	for i := range 3 {
		if !l.allow(start) {
			t.Fatalf("request %d of the burst refused", i)
		}
	}
	if l.allow(start) {
		t.Fatal("request over the burst allowed")
	}
	if l.allow(start.Add(50 * time.Millisecond)) {
		t.Fatal("allowed after half a token")
	}
	if !l.allow(start.Add(110 * time.Millisecond)) {
		t.Fatal("refused after a full token")
	}
	// A long pause refills to the burst, not beyond it.
	later := start.Add(time.Hour)
	for i := range 3 {
		if !l.allow(later) {
			t.Fatalf("request %d after the pause refused", i)
		}
	}
	if l.allow(later) {
		t.Fatal("bucket refilled beyond its burst")
	}
}
