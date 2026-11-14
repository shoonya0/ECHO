package utils

import (
	"encoding/hex"
	"testing"
	"time"
)

// Invite codes are opaque random tokens (#15). Before, a code was the
// AES-GCM encryption of "<chatID>_<userID>_<displayName>_<expiry>" under a key
// in this public repo, so anyone could read who made it and for which group.
func TestInviteCodeIsAnOpaqueRandomToken(t *testing.T) {
	expiry := time.Now().Add(time.Hour)

	first, ok := NewInviteCodeWithDefaults(expiry)
	if !ok {
		t.Fatal("NewInviteCodeWithDefaults failed")
	}
	second, ok := NewInviteCodeWithDefaults(expiry)
	if !ok {
		t.Fatal("NewInviteCodeWithDefaults failed")
	}

	raw, err := hex.DecodeString(first.InviteCode)
	if err != nil {
		t.Fatalf("code %q is not hex: %v", first.InviteCode, err)
	}
	if len(raw) != inviteCodeBytes {
		t.Fatalf("code holds %d bytes, want %d random bytes", len(raw), inviteCodeBytes)
	}
	if first.InviteCode == second.InviteCode {
		t.Fatal("two codes with the same expiry are equal")
	}
	if !first.ExpiredDate.Equal(expiry) || first.Deleted || first.Status != "active" {
		t.Fatalf("stored fields = %+v, want active, not deleted, expiring at %v", first, expiry)
	}
}
