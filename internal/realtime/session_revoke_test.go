package realtime

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/services"
	"github.com/shoonya0/ECHO/internal/testutil"
)

// These tests guard #76: an open WebSocket must close after logout, a ban or a
// deleted profile, not only new connections.

func newTokenClient(id string, userID bson.ObjectID, tokenID string) *models.Client {
	c := newTestClient(id, userID)
	c.TokenID = tokenID
	return c
}

func assertClosedWith(t *testing.T, c *models.Client, code int, reason string) {
	t.Helper()
	if !c.IsClosed() {
		t.Fatalf("client %s: still open, want closed with %d %s", c.ID, code, reason)
	}
	gotCode, gotReason := c.CloseReason()
	if gotCode != code || gotReason != reason {
		t.Fatalf("client %s: closed with %d %q, want %d %q", c.ID, gotCode, gotReason, code, reason)
	}
}

func assertOpen(t *testing.T, c *models.Client) {
	t.Helper()
	if c.IsClosed() {
		code, reason := c.CloseReason()
		t.Fatalf("client %s: closed with %d %q, want it open", c.ID, code, reason)
	}
}

func TestLogoutClosesOnlyThatTokensConnections(t *testing.T) {
	hub := newTestHub(t)
	user, other := bson.NewObjectID(), bson.NewObjectID()
	phone := newTokenClient("phone", user, "tok-phone")
	tablet := newTokenClient("tablet", user, "tok-tablet")
	stranger := newTokenClient("stranger", other, "tok-phone") // same jti text, other user
	for _, c := range []*models.Client{phone, tablet, stranger} {
		addClient(hub, c, "chat1")
	}

	hub.pubSubManager.closeLocalSessions(sessionControl{UserID: user.Hex(), TokenID: "tok-phone", Reason: ReasonTokenRevoked})

	assertClosedWith(t, phone, CloseCodeUnauthorized, ReasonTokenRevoked)
	assertOpen(t, tablet)
	assertOpen(t, stranger)
}

func TestDeleteClosesEveryConnectionOfTheUser(t *testing.T) {
	hub := newTestHub(t)
	user, other := bson.NewObjectID(), bson.NewObjectID()
	phone := newTokenClient("phone", user, "tok-phone")
	tablet := newTokenClient("tablet", user, "tok-tablet")
	stranger := newTokenClient("stranger", other, "tok-other")
	for _, c := range []*models.Client{phone, tablet, stranger} {
		addClient(hub, c, "chat1")
	}

	hub.pubSubManager.closeLocalSessions(sessionControl{UserID: user.Hex(), Reason: ReasonAccountNotFound})

	assertClosedWith(t, phone, CloseCodeUnauthorized, ReasonAccountNotFound)
	assertClosedWith(t, tablet, CloseCodeUnauthorized, ReasonAccountNotFound)
	assertOpen(t, stranger)

	// The closed clients are queued for removal from the hub's maps, once: the
	// instance's own control message arriving later changes nothing.
	hub.pubSubManager.closeLocalSessions(sessionControl{UserID: user.Hex(), Reason: ReasonAccountNotFound})
	if got := len(hub.Unregister); got != 2 {
		t.Fatalf("queued %d clients for unregister, want 2", got)
	}
}

func TestRecheckClosesConnectionsTheAccountCheckRefuses(t *testing.T) {
	hub := newTestHub(t)
	banned, fine, unknown, revoked := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()

	bannedA := newTokenClient("bannedA", banned, "tok-b")
	bannedB := newTokenClient("bannedB", banned, "tok-b") // same token: checked once
	fineC := newTokenClient("fine", fine, "tok-f")
	unknownC := newTokenClient("unknown", unknown, "tok-u")
	revokedC := newTokenClient("revoked", revoked, "tok-r")
	for _, c := range []*models.Client{bannedA, bannedB, fineC, unknownC, revokedC} {
		addClient(hub, c)
	}

	var calls atomic.Int32
	hub.checkSession = func(_ context.Context, userID bson.ObjectID, _ string) error {
		calls.Add(1)
		switch userID {
		case banned:
			return services.ErrAccountBanned
		case unknown:
			return errors.New("mongo is down") // an outage must not drop anyone
		case revoked:
			return services.ErrTokenRevoked
		}
		return nil
	}

	hub.performSessionRecheck()

	assertClosedWith(t, bannedA, CloseCodeForbidden, ReasonAccountBanned)
	assertClosedWith(t, bannedB, CloseCodeForbidden, ReasonAccountBanned)
	assertClosedWith(t, revokedC, CloseCodeUnauthorized, ReasonTokenRevoked)
	assertOpen(t, fineC)
	assertOpen(t, unknownC)
	if got := calls.Load(); got != 4 {
		t.Fatalf("checked %d sessions, want 4 (one per user and token)", got)
	}
}

func TestSessionEndReason(t *testing.T) {
	cases := []struct {
		err    error
		code   int
		reason string
		ends   bool
	}{
		{services.ErrTokenRevoked, CloseCodeUnauthorized, ReasonTokenRevoked, true},
		{services.ErrUserNotFound, CloseCodeUnauthorized, ReasonAccountNotFound, true},
		{services.ErrAccountBanned, CloseCodeForbidden, ReasonAccountBanned, true},
		{services.ErrAccountInactive, CloseCodeForbidden, ReasonAccountInactive, true},
		{errors.New("timeout"), 0, "", false},
	}
	for _, tc := range cases {
		reason, ends := sessionEndReason(tc.err)
		if ends != tc.ends || reason != tc.reason || closeCodeFor(reason) != tc.code {
			t.Errorf("sessionEndReason(%v) = %q %v (code %d), want %q %v (code %d)",
				tc.err, reason, ends, closeCodeFor(reason), tc.reason, tc.ends, tc.code)
		}
	}
}

// A logout on one instance must close the socket held by another instance.
func TestSessionControlMessageReachesOtherInstances(t *testing.T) {
	redisClient := testutil.UseTestRedis(t)
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	hub := newTestHub(t)
	receiver := NewPubSubManager(redisClient, hub.Hub, logger, "receiver")
	t.Cleanup(receiver.cancel)
	if err := receiver.subscribeToChannel(SessionControlChannel); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(func() { _ = receiver.unsubscribeFromChannel(SessionControlChannel) })

	user := bson.NewObjectID()
	client := newTokenClient("remote", user, "tok-remote")
	addClient(hub, client)

	sender := NewPubSubManager(redisClient, &models.Hub{}, logger, "sender")
	t.Cleanup(sender.cancel)
	if err := sender.publishSessionControl(sessionControl{UserID: user.Hex(), TokenID: "tok-remote", Reason: ReasonTokenRevoked}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for !client.IsClosed() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	assertClosedWith(t, client, CloseCodeUnauthorized, ReasonTokenRevoked)
}

// Closing sessions reads the hub's maps while clients connect and disconnect.
// It only proves anything under `go test -race` (#19).
func TestCloseSessionsRacesWithRegistration(t *testing.T) {
	hub := newTestHub(t)
	user := bson.NewObjectID()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range raceIterations {
			c := newTokenClient("c"+string(rune('a'+i%26)), user, "tok")
			addClient(hub, c, "chat1")
			hub.handleClientUnregistration(c)
		}
	}()
	go func() {
		defer wg.Done()
		for range raceIterations {
			hub.pubSubManager.closeLocalSessions(sessionControl{UserID: user.Hex(), Reason: ReasonAccountBanned})
			for len(hub.Unregister) > 0 {
				<-hub.Unregister
			}
		}
	}()
	wg.Wait()
}

func TestClientKeepsTheFirstCloseReason(t *testing.T) {
	c := newTestClient("c", bson.NewObjectID())
	c.Revoke(CloseCodeForbidden, ReasonAccountBanned)
	c.Revoke(CloseCodeUnauthorized, ReasonTokenRevoked)
	c.CloseSend()
	assertClosedWith(t, c, CloseCodeForbidden, ReasonAccountBanned)
}
