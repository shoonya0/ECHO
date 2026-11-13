package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/services"
)

// Internal error text must never reach a WebSocket client (#41). A wrapped
// driver error gets the handler's fixed code and message; known service errors
// get their own.
func TestChatErrorsNeverCarryInternalText(t *testing.T) {
	internal := errors.New("connection(10.0.3.7:27017) server selection timeout")
	cases := []struct {
		name     string
		err      error
		wantCode string
		wantMsg  string
	}{
		{"driver error", fmt.Errorf("failed to get users: %w", internal), "INVITE_FAILED", "Failed to invite users"},
		{"chat not found", fmt.Errorf("%w", services.ErrChatNotFound), "INVALID_CHAT_ID", "Chat not found"},
		{"permission", fmt.Errorf("%w: only owners and admins can add members", services.ErrPermissionDenied), "PERMISSION_DENIED", "Permission denied"},
		{"already member", fmt.Errorf("%w", services.ErrAlreadyParticipant), "INVALID_REQUEST", "Users are already in the chat"},
		{"user not found", fmt.Errorf("%w", services.ErrUserNotFound), "INVALID_USER_ID", "User not found"},
		{"message not found", services.ErrMessageNotFound, "MESSAGE_NOT_FOUND", "Message not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &models.Client{ID: "c1", Send: make(chan models.WebSocketMessage, 1)}
			sendChatError(client, "r1", tc.err, "INVITE_FAILED", "Failed to invite users")

			reply := <-client.Send
			data, ok := reply.Data.(models.ErrorMessage)
			if !ok {
				t.Fatalf("reply data is %T, want models.ErrorMessage", reply.Data)
			}
			if data.Code != tc.wantCode || data.Message != tc.wantMsg {
				t.Fatalf("got %q %q, want %q %q", data.Code, data.Message, tc.wantCode, tc.wantMsg)
			}
			if strings.Contains(data.Message, "27017") || strings.Contains(data.Message, "only owners") {
				t.Fatalf("message leaks internal text: %q", data.Message)
			}
		})
	}
}

// A frame that isn't JSON gets a fixed PARSE_ERROR message, not the decoder's
// text (#41).
func TestParseErrorHidesTheDecoderText(t *testing.T) {
	conn := serveReadLoop(t)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":`)); err != nil {
		t.Fatalf("write: %v", err)
	}

	var msg struct {
		Type string `json:"type"`
		Data struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"data"`
	}
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("read: %v", err)
	}
	if msg.Data.Code != "PARSE_ERROR" || msg.Data.Message != "Message is not valid JSON" {
		t.Fatalf("got %q %q, want PARSE_ERROR with a fixed message", msg.Data.Code, msg.Data.Message)
	}
}

// Bind errors tell the client which fields failed and why, without Go struct
// or type names (#41).
func TestBindErrorDetailHidesGoNames(t *testing.T) {
	type signupBody struct {
		Email       string `json:"email" binding:"required,email"`
		DisplayName string `json:"displayName" binding:"required"`
		Age         int    `json:"age"`
	}
	cases := []struct {
		name string
		body string
		want string
	}{
		{"missing and invalid fields", `{"email":"nope"}`, "email: email; displayName: required"},
		{"wrong type", `{"email":"a@b.co","displayName":"A","age":"x"}`, "request body is not valid JSON for this endpoint"},
		{"not JSON", `{"email":`, "request body is not valid JSON for this endpoint"},
	}
	gin.SetMode(gin.TestMode)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")

			var body signupBody
			err := c.ShouldBindJSON(&body)
			if err == nil {
				t.Fatal("bind succeeded, want an error")
			}
			got := bindErrorDetail(err)
			if got != tc.want {
				t.Fatalf("detail %q, want %q", got, tc.want)
			}
			for _, leak := range []string{"signupBody", "Key:", "Go struct", "of type"} {
				if strings.Contains(got, leak) {
					t.Fatalf("detail %q leaks %q", got, leak)
				}
			}
		})
	}
}
