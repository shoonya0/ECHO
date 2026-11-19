package services

import (
	"testing"
	"time"

	"github.com/shoonya0/ECHO/internal/models"
)

// #31: what a viewer sees of a user's presence, from the status the user chose
// and whether a server holds them now. The stored isOnline never counts.
func TestShowPresence(t *testing.T) {
	seen := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	active := seen.Add(-time.Minute)
	cases := []struct {
		name       string
		stored     string
		connected  bool
		self       bool
		wantStatus string
		wantOnline bool
		wantSeen   bool
	}{
		{"connected online", "online", true, false, "online", true, true},
		{"connected away", "away", true, false, "away", true, true},
		{"connected dnd", "dnd", true, false, "dnd", true, true},
		{"never chose a status", "", true, false, "online", true, true},
		{"not connected", "online", false, false, "offline", false, true},
		{"not connected dnd", "dnd", false, false, "offline", false, true},
		{"invisible and connected", "invisible", true, false, "offline", false, false},
		{"offline by choice and connected", "offline", true, false, "offline", false, false},
		{"self invisible", "invisible", true, true, "invisible", true, true},
		{"self not connected", "online", false, true, "online", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// isOnline is stored wrong on purpose: it must be ignored.
			p := models.PresenceEmbed{Status: tc.stored, IsOnline: !tc.connected, LastSeen: seen, LastActivity: active}
			ShowPresence(&p, tc.connected, tc.self)
			if p.Status != tc.wantStatus || p.IsOnline != tc.wantOnline {
				t.Fatalf("status %q isOnline %v, want %q %v", p.Status, p.IsOnline, tc.wantStatus, tc.wantOnline)
			}
			if got := !p.LastSeen.IsZero(); got != tc.wantSeen {
				t.Fatalf("lastSeen shown = %v, want %v", got, tc.wantSeen)
			}
			if !tc.wantOnline && !tc.self && !p.LastActivity.IsZero() {
				t.Fatalf("lastActivity shown for a user shown offline")
			}
		})
	}
}
