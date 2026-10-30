package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLoginUserResponseOmitsPasswordHash(t *testing.T) {
	out, err := json.Marshal(LoginUserResponse{PasswordHash: "$2a$10$secret"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(out), "passwordHash") || strings.Contains(string(out), "secret") {
		t.Fatalf("password hash serialized: %s", out)
	}
}
