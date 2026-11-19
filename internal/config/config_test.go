package config

import (
	"slices"
	"testing"
)

// WS_ALLOWED_ORIGINS items are trimmed, lose a trailing slash, and empty ones drop (#40).
func TestSplitList(t *testing.T) {
	got := splitList(" https://a.example.com/ ,, https://b.example.com:8443 ,")
	want := []string{"https://a.example.com", "https://b.example.com:8443"}
	if !slices.Equal(got, want) {
		t.Fatalf("splitList = %q, want %q", got, want)
	}
	if got := splitList(""); got != nil {
		t.Fatalf("splitList(\"\") = %q, want nil", got)
	}
}
