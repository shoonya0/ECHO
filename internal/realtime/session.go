package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/services"
)

// Open connections are closed when their session ends: on logout, a deleted
// profile, a ban or deactivation, or when the token expires (#76). Logout and
// delete close them at once, on every instance, through SessionControlChannel.
// Every open connection is also re-checked every SessionRecheckInterval, which
// catches bans made in the database, expired tokens and any control message an
// instance missed. A new connection is checked once more right after it is
// registered, which catches a logout that ran while it was being registered.

// SessionControlChannel carries sessionControl messages. Every instance
// subscribes to it.
const SessionControlChannel = "control:sessions"

// SessionRecheckInterval is how often open connections are re-checked. It matches
// the account cache, so a ban made in MongoDB closes sockets within about twice
// this time.
const SessionRecheckInterval = services.AccountAccessCacheTTL

// Close frame codes: 4000 plus the HTTP status the REST API answers with.
const (
	CloseCodeUnauthorized = 4401
	CloseCodeForbidden    = 4403
)

// Close frame reasons: the codes the REST API sends.
const (
	ReasonTokenExpired    = services.CodeTokenExpired
	ReasonTokenRevoked    = services.CodeTokenRevoked
	ReasonAccountNotFound = services.CodeAccountNotFound
	ReasonAccountBanned   = services.CodeAccountBanned
	ReasonAccountInactive = services.CodeAccountInactive
)

const (
	// sessionCheckTimeout bounds one session's check, so a slow database can't
	// stall a whole pass.
	sessionCheckTimeout = 5 * time.Second
	// sessionCheckWorkers is how many sessions a pass checks at once.
	sessionCheckWorkers = 8
	// sessionPublishTimeout bounds telling the other instances, so a stalled
	// Redis can't hold up a logout reply.
	sessionPublishTimeout = 2 * time.Second
)

// sessionControl asks every instance to close a user's connections: those opened
// with TokenID, or all of them when TokenID is empty.
type sessionControl struct {
	UserID  string `json:"userId"`
	TokenID string `json:"tokenId,omitempty"`
	Reason  string `json:"reason"`
}

// SessionChecker says whether a connection's session may continue, with the
// contract of services.CheckSession, which the hub uses outside tests.
type SessionChecker func(ctx context.Context, userID bson.ObjectID, tokenID string, expiresAt time.Time) error

func (eh *Hub) sessionChecker() SessionChecker {
	if eh.checkSession != nil {
		return eh.checkSession
	}
	return services.CheckSession
}

// sessionEndReason maps a SessionChecker error to a close reason. ok is false when
// the error says nothing about the session, such as an outage: the connection is
// kept and checked again on the next pass, so an outage doesn't drop every client.
func sessionEndReason(err error) (reason string, ok bool) {
	reason, ok = services.SessionEndCode(err)
	if ok && closeCodeFor(reason) == 0 {
		return "", false // a code this file doesn't know yet; never close without a code
	}
	return reason, ok
}

func closeCodeFor(reason string) int {
	switch reason {
	case ReasonTokenExpired, ReasonTokenRevoked, ReasonAccountNotFound:
		return CloseCodeUnauthorized
	case ReasonAccountBanned, ReasonAccountInactive:
		return CloseCodeForbidden
	default:
		return 0
	}
}

// CloseSessions closes a user's connections on every instance: those opened with
// tokenID, or all of them when tokenID is empty. Connections on this instance
// close before it returns, even if Redis is down. An error means the other
// instances weren't told; their re-check closes those within
// SessionRecheckInterval. ctx may be a request's: it isn't cut short when the
// caller hangs up.
func (eh *Hub) CloseSessions(ctx context.Context, userID bson.ObjectID, tokenID, reason string) error {
	msg := sessionControl{UserID: userID.Hex(), TokenID: tokenID, Reason: reason}
	eh.pubSubManager.closeLocalSessions(msg)

	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sessionPublishTimeout)
	defer cancel()
	return eh.pubSubManager.publishSessionControl(ctx, msg)
}

func (pm *PubSubManager) publishSessionControl(ctx context.Context, msg sessionControl) error {
	if pm.redisClient == nil {
		return errors.New("redis client is not initialized")
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal session control: %w", err)
	}
	if err := pm.redisClient.Publish(ctx, SessionControlChannel, data).Err(); err != nil {
		return fmt.Errorf("failed to publish session control: %w", err)
	}
	return nil
}

func (pm *PubSubManager) handleSessionControl(payload string) {
	var msg sessionControl
	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		pm.logger.WithError(err).Warn("Ignoring a session control message that isn't JSON")
		return
	}
	if msg.UserID == "" || closeCodeFor(msg.Reason) == 0 {
		pm.logger.WithFields(logrus.Fields{"userID": msg.UserID, "reason": msg.Reason}).
			Warn("Ignoring a session control message with no user or an unknown reason")
		return
	}
	pm.closeLocalSessions(msg)
}

// closeLocalSessions closes the matching connections on this instance. The
// clients are copied under the lock and closed after it (#19).
func (pm *PubSubManager) closeLocalSessions(msg sessionControl) {
	pm.hub.Mutex.RLock()
	clients := slices.Collect(maps.Values(pm.hub.UserClients[msg.UserID]))
	pm.hub.Mutex.RUnlock()

	// Skipping closed clients makes this a no-op when an instance receives its own
	// message after CloseSessions already closed them.
	for _, client := range clients {
		if !client.IsClosed() && (msg.TokenID == "" || client.TokenID == msg.TokenID) {
			pm.revokeClient(client, msg.Reason)
		}
	}
}

// revokeClient closes one connection with a reason and queues it for removal, so
// no more chat messages are routed to it. The read loop's own unregister is a
// no-op once the client is gone.
func (pm *PubSubManager) revokeClient(client *models.Client, reason string) {
	pm.logger.WithFields(logrus.Fields{"clientID": client.ID, "reason": reason}).Info("Closing a connection whose session ended")
	client.Revoke(closeCodeFor(reason), reason)
	select {
	case pm.hub.Unregister <- client:
	default:
		pm.logger.WithFields(logrus.Fields{"clientID": client.ID}).Warn("Unregister queue full; the read loop will remove the client")
	}
}

// checkNewSession checks a connection just added to the hub's maps. A logout
// that ran between the upgrade's auth check and the registration found nothing
// to close, but its blacklist entry is already written, so this sees it. Later
// logouts find the client in the maps.
func (eh *Hub) checkNewSession(client *models.Client) {
	ctx, cancel := context.WithTimeout(eh.Ctx, sessionCheckTimeout)
	defer cancel()
	err := eh.sessionChecker()(ctx, client.UserID, client.TokenID, client.TokenExpiresAt)
	if reason, ends := sessionEndReason(err); ends {
		eh.pubSubManager.revokeClient(client, reason)
	}
	// Any other error: the periodic re-check tries again.
}

// recheckSessions runs performSessionRecheck every SessionRecheckInterval.
func (eh *Hub) recheckSessions() {
	ticker := time.NewTicker(SessionRecheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-eh.Ctx.Done():
			return
		case <-ticker.C:
			eh.performSessionRecheck()
		}
	}
}

// performSessionRecheck checks every open connection, one check per user and
// token, sessionCheckWorkers at a time, and closes those whose session ended. The
// first check that can't be made (an outage) ends the pass with one log line;
// the next pass tries again.
func (eh *Hub) performSessionRecheck() {
	eh.Mutex.RLock()
	clients := slices.Collect(maps.Values(eh.Clients))
	eh.Mutex.RUnlock()

	type session struct {
		userID  bson.ObjectID
		tokenID string
	}
	bySession := make(map[session][]*models.Client)
	for _, client := range clients {
		if !client.IsClosed() {
			key := session{client.UserID, client.TokenID}
			bySession[key] = append(bySession[key], client)
		}
	}

	check := eh.sessionChecker()
	jobs := make(chan []*models.Client)
	var outage atomic.Pointer[error]
	var wg sync.WaitGroup
	for range min(sessionCheckWorkers, len(bySession)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for group := range jobs {
				if outage.Load() != nil {
					continue // drain without checking
				}
				first := group[0]
				ctx, cancel := context.WithTimeout(eh.Ctx, sessionCheckTimeout)
				err := check(ctx, first.UserID, first.TokenID, first.TokenExpiresAt)
				cancel()
				if err == nil {
					continue
				}
				reason, ends := sessionEndReason(err)
				if !ends {
					outage.CompareAndSwap(nil, &err)
					continue
				}
				for _, client := range group {
					eh.pubSubManager.revokeClient(client, reason)
				}
			}
		}()
	}
	for _, group := range bySession {
		jobs <- group
	}
	close(jobs)
	wg.Wait()

	if errp := outage.Load(); errp != nil {
		eh.logger.WithError(*errp).Warn("Session re-check stopped: a check couldn't be made; open connections are kept until the next pass")
	}
}
