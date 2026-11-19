package utils

import (
	"strings"
	"testing"
)

// New usernames are 3 to 30 letters, digits, _ or . so they work as @handles
// (#36). Spaces around the name are trimmed first.
func TestValidUsername(t *testing.T) {
	tests := []struct {
		name string
		ok   bool
	}{
		{"alice", true},
		{"Alice_Smith.99", true},
		{"abc", true},
		{strings.Repeat("a", 30), true},
		{"ab", false},
		{strings.Repeat("a", 31), false},
		{"", false},
		{"has space", false},
		{"dash-name", false},
		{"émile", false},
		{"@alice", false},
	}
	for _, tt := range tests {
		if got := ValidUsername(tt.name); got != tt.ok {
			t.Errorf("ValidUsername(%q) = %v, want %v", tt.name, got, tt.ok)
		}
	}
}

func TestNormalizeUsername(t *testing.T) {
	if got := NormalizeUsername("  Alice \t"); got != "Alice" {
		t.Fatalf("NormalizeUsername = %q, want %q", got, "Alice")
	}
}
