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

// Internal error text must never reach a WebSocket client (#41). invite_user
// maps the service's known errors to their own codes; anything else, including
// a wrapped driver error, gets the fixed INVITE_FAILED.
func TestInviteErrorsNeverCarryInternalText(t *testing.T) {
	internal := errors.New("connection(10.0.3.7:27017) server selection timeout")
	cases := []struct {
		name     string
		err      error
		wantCode string
		wantMsg  string
	}{
		{"driver error", fmt.Errorf("failed to get users: %w", internal), "INVITE_FAILED", "Failed to invite users"},
		{"chat not found", fmt.Errorf("%w", services.ErrChatNotFound), "INVALID_CHAT_ID", "Chat not found"},
		{"not a member", fmt.Errorf("%w", services.ErrNotParticipant), "PERMISSION_DENIED", "You are not a participant of this chat"},
		{"already member", fmt.Errorf("%w", services.ErrAlreadyParticipant), "INVALID_REQUEST", "Users are already in the chat"},
		{"user not found", fmt.Errorf("%w", services.ErrUserNotFound), "INVALID_USER_ID", "User not found"},
		// The handler checked the role first, so this is the chat changing during
		// the add: retryable, not a permission problem.
		{"concurrent change", fmt.Errorf("%w: chat changed while adding members", services.ErrPermissionDenied), "INVITE_FAILED", "Failed to invite users"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &models.Client{ID: "c1", Send: make(chan models.WebSocketMessage, 1)}
			sendInviteError(client, "r1", tc.err)

			data := errorData(t, client)
			if data.Code != tc.wantCode || data.Message != tc.wantMsg {
				t.Fatalf("got %q %q, want %q %q", data.Code, data.Message, tc.wantCode, tc.wantMsg)
			}
			if strings.Contains(data.Message, "27017") || strings.Contains(data.Message, "changed") {
				t.Fatalf("message leaks internal text: %q", data.Message)
			}
		})
	}
}

// The invite mapping must not change what the other handlers send: a typing
// failure on a missing user stays TYPING_FAILED.
func TestChatErrorsKeepTheirHandlerCodes(t *testing.T) {
	client := &models.Client{ID: "c1", Send: make(chan models.WebSocketMessage, 1)}
	sendChatError(client, "r1", fmt.Errorf("%w", services.ErrUserNotFound), "TYPING_FAILED", "Failed to update typing status")

	if data := errorData(t, client); data.Code != "TYPING_FAILED" {
		t.Fatalf("got %q, want TYPING_FAILED", data.Code)
	}
}

// Content problems are fixed texts that don't echo the request (#41).
func TestMessageContentProblemsDontEchoTheRequest(t *testing.T) {
	problem := validateMessageContent(models.MessageRequest{Content: "hi", MessageType: "<script>x</script>"})
	if problem != "invalid message type" {
		t.Fatalf("problem %q, want %q", problem, "invalid message type")
	}
	if ok := validateMessageContent(models.MessageRequest{Content: "hi", MessageType: "text"}); ok != "" {
		t.Fatalf("valid message reported %q", ok)
	}
}

func errorData(t *testing.T, client *models.Client) models.ErrorMessage {
	t.Helper()
	reply := <-client.Send
	data, ok := reply.Data.(models.ErrorMessage)
	if !ok {
		t.Fatalf("reply data is %T, want models.ErrorMessage", reply.Data)
	}
	return data
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
	type settingsBody struct {
		Name string `json:"name" binding:"required"`
	}
	type signupBody struct {
		Email       string       `json:"email" binding:"required,email"`
		DisplayName string       `json:"displayName" binding:"required"`
		Password    string       `json:"password" binding:"omitempty,min=8"`
		Age         int          `json:"age"`
		Settings    settingsBody `json:"settings"`
	}
	valid := `"email":"a@b.co","displayName":"A","settings":{"name":"s"}`
	cases := []struct {
		name string
		body string
		want string
	}{
		{"missing and invalid fields", `{"email":"nope","settings":{"name":"s"}}`, "email: email; displayName: required"},
		{"rule with a value", `{` + valid + `,"password":"short"}`, "password: min=8"},
		{"nested field", `{"email":"a@b.co","displayName":"A"}`, "settings.name: required"},
		{"wrong type", `{` + valid + `,"age":"x"}`, "age: wrong type"},
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
