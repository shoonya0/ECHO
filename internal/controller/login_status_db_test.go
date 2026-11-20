package controller

import (
	"encoding/json"
	"net/http"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/services"
)

// Login's 403 for a banned or inactive account carries the same code the auth
// middleware sends for those states, and keeps its success/message fields (#171).
func TestLoginRefusalCarriesAccountCode(t *testing.T) {
	tests := []struct {
		name    string
		status  bson.M
		message string
		code    string
	}{
		{"banned", bson.M{"accountStatus.isBanned": true}, "account is banned", services.CodeAccountBanned},
		{"inactive", bson.M{"accountStatus.isActive": false}, "account is not active", services.CodeAccountInactive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupVictim(t, 100, 100)
			res, err := database.DB.Collection(string(constants.UserColl)).UpdateOne(t.Context(),
				bson.M{"email": "victim@example.com"}, bson.M{"$set": tt.status})
			if err != nil {
				t.Fatalf("set account status: %v", err)
			}
			if res.MatchedCount != 1 {
				t.Fatalf("set account status: matched %d, want 1", res.MatchedCount)
			}

			w := loginFrom("192.0.2.20", victimRight)
			if w.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403 (body %s)", w.Code, w.Body)
			}
			var body map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["code"] != tt.code || body["message"] != tt.message || body["success"] != false {
				t.Fatalf("body = %s, want success false, message %q, code %q", w.Body, tt.message, tt.code)
			}
		})
	}
}
