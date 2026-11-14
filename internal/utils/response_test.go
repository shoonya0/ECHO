package utils

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// A limit of 0 or less must not crash the request with a divide by zero
// (#39); it reports no pages instead.
func TestPaginatedResponseWithoutAPositiveLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name         string
		limit, total int
		wantPages    int
	}{
		{"zero limit", 0, 3, 0},
		{"negative limit", -5, 3, 0},
		{"normal", 2, 3, 2},
		{"empty list", 10, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			PaginatedResponse(c, "x", []int{}, tc.limit, tc.total)

			var resp struct {
				Data struct {
					Pagination struct {
						TotalPages int `json:"total_pages"`
					} `json:"pagination"`
				} `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v (body %s)", err, w.Body)
			}
			if got := resp.Data.Pagination.TotalPages; got != tc.wantPages {
				t.Fatalf("total_pages = %d, want %d", got, tc.wantPages)
			}
		})
	}
}
