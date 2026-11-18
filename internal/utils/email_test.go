package utils

import "testing"

func TestNormalizeEmail(t *testing.T) {
	tests := map[string]string{
		"alice@x.com":        "alice@x.com",
		"Alice@X.COM":        "alice@x.com",
		"  bob@example.org ": "bob@example.org",
		"":                   "",
	}
	for in, want := range tests {
		if got := NormalizeEmail(in); got != want {
			t.Errorf("NormalizeEmail(%q) = %q, want %q", in, got, want)
		}
	}
}
