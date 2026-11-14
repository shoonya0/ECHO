package controller

import (
	"context"
	"testing"

	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/testutil"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestInviteChatLookupErrors covers #83. Only a chat that doesn't exist is
// INVALID_CHAT_ID; a failed lookup, such as a timeout or a canceled request,
// is the retryable INVITE_FAILED, not "Chat not found".
func TestInviteChatLookupErrors(t *testing.T) {
	ctx := testutil.UseTestDB(t)

	canceled, cancel := context.WithCancel(ctx)
	cancel()

	cases := []struct {
		name     string
		ctx      context.Context
		wantCode string
	}{
		{"missing chat", ctx, "INVALID_CHAT_ID"},
		{"lookup fails", canceled, "INVITE_FAILED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &models.Client{ID: "c1", UserID: bson.NewObjectID(), Send: make(chan models.WebSocketMessage, 1)}
			handleInviteUser(tc.ctx, client, models.MessageRequest{
				ChatID:    bson.NewObjectID().Hex(),
				RequestID: "r1",
				Metadata:  map[string]interface{}{"userIds": []interface{}{bson.NewObjectID().Hex()}},
			})

			if data := errorData(t, client); data.Code != tc.wantCode {
				t.Fatalf("code = %q (%q), want %q", data.Code, data.Message, tc.wantCode)
			}
		})
	}
}
