package services

import (
	"testing"

	"github.com/shoonya0/ECHO/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestMessagePageFilter(t *testing.T) {
	chatID := bson.NewObjectID()

	t.Run("first page has no cursor bound", func(t *testing.T) {
		filter := messagePageFilter(chatID, nil)
		if filter["chatId"] != chatID {
			t.Fatalf("chatId = %v, want %v", filter["chatId"], chatID)
		}
		if _, ok := filter["_id"]; ok {
			t.Fatalf("unexpected _id bound on first page: %v", filter["_id"])
		}
	})

	t.Run("cursor bounds _id strictly below it", func(t *testing.T) {
		before := bson.NewObjectID()
		filter := messagePageFilter(chatID, &before)
		bound, ok := filter["_id"].(bson.M)
		if !ok {
			t.Fatalf("_id = %#v, want bson.M", filter["_id"])
		}
		if bound["$lt"] != before {
			t.Fatalf("_id.$lt = %v, want %v", bound["$lt"], before)
		}
	})
}

func TestTrimMessagePage(t *testing.T) {
	msgs := func(n int) []models.Message {
		out := make([]models.Message, n)
		for i := range out {
			out[i].ID = bson.NewObjectID()
		}
		return out
	}

	tests := []struct {
		name        string
		fetched     int
		limit       int
		wantLen     int
		wantHasMore bool
	}{
		{"empty chat", 0, 3, 0, false},
		{"partial last page", 2, 3, 2, false},
		{"exactly limit rows", 3, 3, 3, false},
		{"extra row means more", 4, 3, 3, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := msgs(tt.fetched)
			got, hasMore := trimMessagePage(in, tt.limit)
			if len(got) != tt.wantLen || hasMore != tt.wantHasMore {
				t.Fatalf("got len=%d hasMore=%v, want len=%d hasMore=%v",
					len(got), hasMore, tt.wantLen, tt.wantHasMore)
			}
			// The page must keep the newest rows (the head), dropping the probe row.
			for i := range got {
				if got[i].ID != in[i].ID {
					t.Fatalf("row %d changed after trim", i)
				}
			}
		})
	}
}
