package services

import (
	"context"
	"math"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// GetUserSuggestions refuses a page or limit out of range before it queries,
// so a huge page can't overflow the skip (#131).
func TestGetUserSuggestionsRejectsOutOfRange(t *testing.T) {
	cases := []struct{ page, limit int }{
		{0, 20}, {-1, 20}, {MaxPage + 1, 20}, {math.MaxInt, 50}, {1, 0},
	}
	for _, tc := range cases {
		if _, _, err := GetUserSuggestions(context.Background(), bson.NewObjectID(), tc.page, tc.limit, nil); err == nil {
			t.Errorf("page %d, limit %d: accepted, want an error", tc.page, tc.limit)
		}
	}
}
