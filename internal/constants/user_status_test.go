package constants

import "testing"

// One rule for which statuses exist and which hide the user, shared by the
// REST replies and the hub's announcements (#31).
func TestUserStatusRules(t *testing.T) {
	cases := []struct {
		status        UserStatus
		valid, hidden bool
	}{
		{UserStatusOnline, true, false},
		{UserStatusAway, true, false},
		{UserStatusDND, true, false},
		{UserStatusInvisible, true, true},
		{UserStatusOffline, true, true},
		{"Invisible", false, false},
		{"busy", false, false},
	}
	for _, tc := range cases {
		if got := tc.status.Valid(); got != tc.valid {
			t.Errorf("%q.Valid() = %v, want %v", tc.status, got, tc.valid)
		}
		if got := tc.status.Hidden(); got != tc.hidden {
			t.Errorf("%q.Hidden() = %v, want %v", tc.status, got, tc.hidden)
		}
	}
}
