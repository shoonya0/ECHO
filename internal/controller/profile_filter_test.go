package controller

import (
	"slices"
	"testing"
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
