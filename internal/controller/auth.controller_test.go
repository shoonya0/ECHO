package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shoonya0/ECHO/internal/database"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// useBrokenDB points database.DB at a disconnected client so every query
// fails with a driver error.
func useBrokenDB(t *testing.T) {
	t.Helper()
	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://127.0.0.1:1"))
	if err != nil {
		t.Fatalf("mongo.Connect: %v", err)
	}
	if err := client.Disconnect(context.Background()); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}

	prev := database.DB
	database.DB = client.Database("echo_test")
	t.Cleanup(func() { database.DB = prev })
}

func TestAuthServerErrorsHideInternals(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useBrokenDB(t)

	tests := []struct {
		name    string
		handler gin.HandlerFunc
		body    string
	}{
		{"signup", Signup, `{"email":"a@example.com","username":"a","password":"password123"}`},
		{"login", Login, `{"email":"a@example.com","password":"password123"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")

			tt.handler(c)

			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500 (body %s)", w.Code, w.Body)
			}
			var resp map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if detail, ok := resp["error"]; ok {
				t.Fatalf("500 response exposes internal error: %v", detail)
			}
		})
	}
}
