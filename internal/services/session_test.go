package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// An open WebSocket outlives its token unless the re-check looks at expiry (#76).
// Expiry is checked first, so this needs neither Redis nor MongoDB.
func TestCheckSessionRefusesAnExpiredToken(t *testing.T) {
	err := CheckSession(context.Background(), bson.NewObjectID(), "jti", time.Now().Add(-time.Second))
	if !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("CheckSession with an expired token = %v, want ErrTokenExpired", err)
	}
	if code, ok := SessionEndCode(err); !ok || code != CodeTokenExpired {
		t.Fatalf("SessionEndCode = %q %v, want %q true", code, ok, CodeTokenExpired)
	}
}

func TestSessionEndCodeKeepsOutagesApart(t *testing.T) {
	if code, ok := SessionEndCode(errors.New("redis is down")); ok {
		t.Fatalf("an outage mapped to %q; it must not end the session", code)
	}
}
