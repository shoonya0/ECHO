package controller

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/shoonya0/ECHO/internal/models"
)

// #51: the content limit counts characters, as its error says, so text outside
// plain English isn't cut off at a third of the limit.
func TestMessageContentLimitCountsCharacters(t *testing.T) {
	const tooLong = "message content too long (max 4000 characters)"
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"4000 ASCII characters", strings.Repeat("a", 4000), ""},
		{"4001 ASCII characters", strings.Repeat("a", 4001), tooLong},
		{"1500 Hindi characters (4500 bytes)", strings.Repeat("ह", 1500), ""},
		{"4000 Hindi characters", strings.Repeat("ह", 4000), ""},
		{"4001 Hindi characters", strings.Repeat("ह", 4001), tooLong},
		{"4000 emoji (16000 bytes)", strings.Repeat("😀", 4000), ""},
		{"4001 emoji", strings.Repeat("😀", 4001), tooLong},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validateMessageContent(models.MessageRequest{Content: tc.content, MessageType: "text"})
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// The largest content allowed still fits in one frame, even when the client
// escapes every character in JSON (a surrogate pair, 12 bytes per emoji).
func TestLongestContentFitsInAFrame(t *testing.T) {
	var escaped strings.Builder
	for range 4000 {
		escaped.WriteString(`😀`)
	}
	frame := `{"type":"send_message","requestId":"r1","chatId":"0123456789abcdef01234567","content":"` + escaped.String() + `"}`
	if len(frame) > maxFrameBytes {
		t.Fatalf("frame is %d bytes, over maxFrameBytes (%d)", len(frame), maxFrameBytes)
	}
	var req models.MessageRequest
	if err := json.Unmarshal([]byte(frame), &req); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := validateMessageContent(req); got != "" {
		t.Fatalf("the longest allowed content was refused: %q", got)
	}
}
