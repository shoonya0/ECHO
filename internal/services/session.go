package services

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Codes a client gets when its session may no longer be used: in REST replies and
// as the reason of a WebSocket close frame (#33, #76).
const (
	CodeTokenExpired    = "TOKEN_EXPIRED"
	CodeTokenRevoked    = "TOKEN_REVOKED"
	CodeAccountNotFound = "ACCOUNT_NOT_FOUND"
	CodeAccountBanned   = "ACCOUNT_BANNED"
	CodeAccountInactive = "ACCOUNT_INACTIVE"
)

// CheckSession makes the checks that every request and every open WebSocket needs
// after the token's signature is verified: the token hasn't expired (a zero
// expiresAt skips this) or been logged out, and the account may still use it.
// It returns nil, an error SessionEndCode maps to a code, or any other error when
// the check couldn't be made.
func CheckSession(ctx context.Context, userID bson.ObjectID, tokenID string, expiresAt time.Time) error {
	if !expiresAt.IsZero() && !time.Now().Before(expiresAt) {
		return ErrTokenExpired
	}
	revoked, err := IsTokenRevoked(ctx, tokenID)
	if err != nil {
		return err
	}
	if revoked {
		return ErrTokenRevoked
	}
	return CheckAccountAccess(ctx, userID)
}

// SessionEndCode maps a CheckSession error to its code. ok is false when the error
// says nothing about the session, such as an outage.
func SessionEndCode(err error) (code string, ok bool) {
	switch {
	case errors.Is(err, ErrTokenExpired):
		return CodeTokenExpired, true
	case errors.Is(err, ErrTokenRevoked):
		return CodeTokenRevoked, true
	case errors.Is(err, ErrUserNotFound):
		return CodeAccountNotFound, true
	case errors.Is(err, ErrAccountBanned):
		return CodeAccountBanned, true
	case errors.Is(err, ErrAccountInactive):
		return CodeAccountInactive, true
	default:
		return "", false
	}
}
