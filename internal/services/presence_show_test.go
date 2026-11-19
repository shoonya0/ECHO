package services

import (
	"testing"
	"time"

	"github.com/shoonya0/ECHO/internal/models"
)

// What a viewer sees of a user's presence, from the status the user chose,
// whether a server holds them now (#31) and their privacy settings (#32). The
// stored isOnline never counts.
func TestShowPresence(t *testing.T) {
	seen := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	active := seen.Add(-time.Minute)
	open := models.PrivacySettingsEmbed{ShowOnlineStatus: "everyone", ShowLastSeen: true}
	noLastSeen := models.PrivacySettingsEmbed{ShowOnlineStatus: "everyone", ShowLastSeen: false}
	contactsOnly := models.PrivacySettingsEmbed{ShowOnlineStatus: "contacts", ShowLastSeen: true}
	nobody := models.PrivacySettingsEmbed{ShowOnlineStatus: "none", ShowLastSeen: true}
	unknown := models.PrivacySettingsEmbed{ShowOnlineStatus: "friends", ShowLastSeen: true}
	unset := models.PrivacySettingsEmbed{ShowLastSeen: true}

	cases := []struct {
		name       string
		stored     string
		privacy    models.PrivacySettingsEmbed
		connected  bool
		viewer     Viewer
		wantStatus string
		wantOnline bool
		wantSeen   bool
	}{
		// #31: status and connection.
		{"connected online", "online", open, true, ViewerOther, "online", true, true},
		{"connected away", "away", open, true, ViewerOther, "away", true, true},
		{"connected dnd", "dnd", open, true, ViewerOther, "dnd", true, true},
		{"never chose a status", "", open, true, ViewerOther, "online", true, true},
		{"not connected", "online", open, false, ViewerOther, "offline", false, true},
		{"not connected dnd", "dnd", open, false, ViewerOther, "offline", false, true},
		{"invisible and connected", "invisible", open, true, ViewerContact, "offline", false, false},
		{"offline by choice and connected", "offline", open, true, ViewerContact, "offline", false, false},
		{"self invisible", "invisible", nobody, true, ViewerSelf, "invisible", true, true},
		{"self not connected", "online", noLastSeen, false, ViewerSelf, "online", false, true},
		// #32: showLastSeen.
		{"lastSeen hidden, connected", "online", noLastSeen, true, ViewerContact, "online", true, false},
		{"lastSeen hidden, not connected", "online", noLastSeen, false, ViewerContact, "offline", false, false},
		// #32: showOnlineStatus.
		{"contacts only, contact", "online", contactsOnly, true, ViewerContact, "online", true, true},
		{"contacts only, other", "online", contactsOnly, true, ViewerOther, "offline", false, false},
		{"nobody, contact", "online", nobody, true, ViewerContact, "offline", false, false},
		{"unknown scope hides", "online", unknown, true, ViewerContact, "offline", false, false},
		{"scope never set shows", "online", unset, true, ViewerOther, "online", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// isOnline is stored wrong on purpose: it must be ignored.
			p := models.PresenceEmbed{Status: tc.stored, IsOnline: !tc.connected, LastSeen: seen, LastActivity: active}
			ShowPresence(&p, tc.privacy, tc.connected, tc.viewer)
			if p.Status != tc.wantStatus || p.IsOnline != tc.wantOnline {
				t.Fatalf("status %q isOnline %v, want %q %v", p.Status, p.IsOnline, tc.wantStatus, tc.wantOnline)
			}
			if got := !p.LastSeen.IsZero(); got != tc.wantSeen {
				t.Fatalf("lastSeen shown = %v, want %v", got, tc.wantSeen)
			}
			if tc.viewer != ViewerSelf && (!tc.wantOnline || !tc.wantSeen) && !p.LastActivity.IsZero() {
				t.Fatalf("lastActivity shown to a viewer who may see neither online nor lastSeen")
			}
		})
	}
}
