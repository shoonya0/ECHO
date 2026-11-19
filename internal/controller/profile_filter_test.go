package controller

import (
	"slices"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// PUT /profile drops "email" (#36): changing it needs a flow that checks the
// password and the new address, which doesn't exist yet.
func TestFilterProfileUpdateKeysDropsEmail(t *testing.T) {
	sanitized, dropped := FilterProfileUpdateKeys(map[string]interface{}{
		"email":    "new@example.com",
		"username": "alice",
	})
	if _, ok := sanitized["email"]; ok {
		t.Fatalf("email kept in the update: %v", sanitized)
	}
	if !slices.Contains(dropped, "email") {
		t.Fatalf("dropped = %v, want it to list email", dropped)
	}
	if sanitized["username"] != "alice" {
		t.Fatalf("username not kept: %v", sanitized)
	}
}

// PUT /profile drops presence.isOnline and presence.lastSeen (#31): the server
// sets them from the user's connections. The chosen status stays editable.
func TestFilterProfileUpdateKeysDropsServerPresence(t *testing.T) {
	sanitized, dropped := FilterProfileUpdateKeys(map[string]interface{}{
		"presence.isOnline": true,
		"presence.lastSeen": "2030-01-01T00:00:00Z",
		"presence.status":   "away",
	})
	for _, key := range []string{"presence.isOnline", "presence.lastSeen"} {
		if _, ok := sanitized[key]; ok || !slices.Contains(dropped, key) {
			t.Fatalf("%s kept in the update: sanitized %v, dropped %v", key, sanitized, dropped)
		}
	}
	if sanitized["presence.status"] != "away" {
		t.Fatalf("presence.status not kept: %v", sanitized)
	}
}

// PUT /profile keeps only fields that are both sent and declared by the
// request struct, with the struct's values (#32 review): keys the struct
// doesn't declare, such as MongoDB operators, never reach the update.
func TestSentFieldsOnly(t *testing.T) {
	structDoc := bson.M{"settings.privacy.showLastSeen": false, "settings.theme": "", "username": "alice"}
	sentDoc := bson.M{"settings.privacy.showLastSeen": "ignored", "username": "x", "$where": "1", "accountStatus.isBanned": false}
	got := sentFieldsOnly(structDoc, sentDoc)
	want := bson.M{"settings.privacy.showLastSeen": false, "username": "alice"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
