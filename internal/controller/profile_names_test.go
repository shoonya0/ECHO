package controller

import "testing"

// Only updates of a field live events show clear the name caches; a presence or
// settings change mustn't broadcast to every server (code review of #36).
func TestChangesShownName(t *testing.T) {
	tests := []struct {
		update map[string]interface{}
		want   bool
	}{
		{map[string]interface{}{"username": "a"}, true},
		{map[string]interface{}{"profile.displayName": "A"}, true},
		{map[string]interface{}{"profile.avatar": "a.png"}, true},
		{map[string]interface{}{"presence.status": "away", "settings.theme": "dark"}, false},
		{map[string]interface{}{"profile.bio": "hi"}, false},
		{map[string]interface{}{}, false},
	}
	for _, tt := range tests {
		if got := changesShownName(tt.update); got != tt.want {
			t.Errorf("changesShownName(%v) = %v, want %v", tt.update, got, tt.want)
		}
	}
}
