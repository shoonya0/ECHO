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
