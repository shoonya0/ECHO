package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/services"
)

// Open connections are closed when their session ends: on logout, a deleted
// profile, or a ban or deactivation (#76). Logout and delete close them at once,
// on every instance, through SessionControlChannel. Every open connection is also
// re-checked every SessionRecheckInterval, which catches bans made in the
// database and any control message an instance missed.

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

// Close frame reasons, the same codes the REST API sends.
const (
	ReasonTokenRevoked    = "TOKEN_REVOKED"
	ReasonAccountNotFound = "ACCOUNT_NOT_FOUND"
	ReasonAccountBanned   = "ACCOUNT_BANNED"
	ReasonAccountInactive = "ACCOUNT_INACTIVE"
)

// sessionRecheckTimeout bounds one session's check, so a slow database can't stall
// the whole pass.
const sessionRecheckTimeout = 5 * time.Second

// sessionControl asks every instance to close a user's connections: those opened
// with TokenID, or all of them when TokenID is empty.
type sessionControl struct {
	UserID  string `json:"userId"`
	TokenID string `json:"tokenId,omitempty"`
	Reason  string `json:"reason"`
}

// SessionChecker says whether a connection's session may continue. It returns nil,
// an error that sessionEndReason maps to a reason, or any other error when the
// check couldn't be made.
type SessionChecker func(ctx context.Context, userID bson.ObjectID, tokenID string) error

// checkSessionWithServices is the SessionChecker the hub uses outside tests: the
// same checks AuthMiddleware makes on every request.
func checkSessionWithServices(ctx context.Context, userID bson.ObjectID, tokenID string) error {
	revoked, err := services.IsTokenRevoked(ctx, tokenID)
	if err != nil {
		return err
	}
	if revoked {
		return services.ErrTokenRevoked
	}
	return services.CheckAccountAccess(ctx, userID)
}

// sessionEndReason maps a SessionChecker error to a close reason. ok is false when
// the error says nothing about the session, such as an outage: the connection is
// kept and checked again on the next pass, so an outage doesn't drop every client.
func sessionEndReason(err error) (reason string, ok bool) {
	switch {
	case errors.Is(err, services.ErrTokenRevoked):
		return ReasonTokenRevoked, true
	case errors.Is(err, services.ErrUserNotFound):
		return ReasonAccountNotFound, true
	case errors.Is(err, services.ErrAccountBanned):
		return ReasonAccountBanned, true
	case errors.Is(err, services.ErrAccountInactive):
		return ReasonAccountInactive, true
	default:
		return "", false
	}
}

func closeCodeFor(reason string) int {
	switch reason {
	case ReasonTokenRevoked, ReasonAccountNotFound:
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
// SessionRecheckInterval.
func (eh *Hub) CloseSessions(userID bson.ObjectID, tokenID, reason string) error {
	msg := sessionControl{UserID: userID.Hex(), TokenID: tokenID, Reason: reason}
	eh.pubSubManager.closeLocalSessions(msg)
	return eh.pubSubManager.publishSessionControl(msg)
}

func (pm *PubSubManager) publishSessionControl(msg sessionControl) error {
	if pm.redisClient == nil {
		return errors.New("redis client is not initialized")
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal session control: %w", err)
	}
	if err := pm.redisClient.Publish(pm.ctx, SessionControlChannel, data).Err(); err != nil {
		return fmt.Errorf("failed to publish session control: %w", err)
	}
	return nil
}

func (pm *PubSubManager) handleSessionControl(payload string) {
	var msg sessionControl
	if err := json.Unmarshal([]byte(payload), &msg); err != nil || msg.UserID == "" || closeCodeFor(msg.Reason) == 0 {
		pm.logger.WithError(err).Warn("Ignoring a malformed session control message")
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

// performSessionRecheck checks every open connection once, one check per user and
// token, and closes those whose session ended.
func (eh *Hub) performSessionRecheck() {
	eh.Mutex.RLock()
	clients := slices.Collect(maps.Values(eh.Clients))
	eh.Mutex.RUnlock()

	type session struct {
		userID  bson.ObjectID
		tokenID string
	}
	results := make(map[session]error)
	check := eh.checkSession
	if check == nil {
		check = checkSessionWithServices
	}

	for _, client := range clients {
		if client.IsClosed() {
			continue
		}
		key := session{client.UserID, client.TokenID}
		err, checked := results[key]
		if !checked {
			ctx, cancel := context.WithTimeout(eh.Ctx, sessionRecheckTimeout)
			err = check(ctx, key.userID, key.tokenID)
			cancel()
			results[key] = err
		}
		if err == nil {
			continue
		}
		reason, ends := sessionEndReason(err)
		if !ends {
			if !checked {
				eh.logger.WithError(err).WithField("userID", key.userID.Hex()).Warn("Session re-check failed; keeping the connection")
			}
			continue
		}
		eh.pubSubManager.revokeClient(client, reason)
	}
}
