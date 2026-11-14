package controller

import (
	"fmt"
	"strings"
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// The "connection established" log line, written at Info on every connect,
// names the connection by user ID and client ID only: no username or email
// (#88).
func TestConnectionLogFieldsHoldNoPersonalData(t *testing.T) {
	user := models.LoginUserResponse{ID: bson.NewObjectID(), Email: "connect@example.com", Username: "Jane Doe"}
	client := &models.Client{ID: "client-1"}

	fields := connectionLogFields(user, client)

	if fields[string(constants.UserIDKey)] != user.ID.Hex() {
		t.Fatalf("user ID field = %#v, want %s", fields[string(constants.UserIDKey)], user.ID.Hex())
	}
	if fields[string(constants.ClientIDKey)] != "client-1" {
		t.Fatalf("client ID field = %#v, want client-1", fields[string(constants.ClientIDKey)])
	}
	for key, value := range fields {
		text := fmt.Sprintf("%v", value)
		if strings.Contains(text, "Jane Doe") || strings.Contains(text, "connect@example.com") {
			t.Fatalf("log field %q holds personal data: %s", key, text)
		}
	}
}
