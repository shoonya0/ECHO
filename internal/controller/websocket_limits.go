package controller

import (
	"encoding/json"
	"time"

	"github.com/shoonya0/ECHO/internal/models"
)

// Limits on what one WebSocket connection may send (#40).
const (
	// maxFrameBytes is the largest frame the server reads. A bigger frame closes
	// the connection with 1009 before it is buffered or decoded. A message's
	// content is at most 4000 bytes, so this leaves room for JSON escaping,
	// attachments, mentions and metadata.
	maxFrameBytes = 64 << 10
	// requestsPerSecond and requestBurst set each connection's token bucket.
	// A request over the limit is answered with RATE_LIMITED and not handled.
	requestsPerSecond = 20
	requestBurst      = 40
)

// requestLimiter is a token bucket. It is used only by its connection's read
// loop, so it needs no lock.
type requestLimiter struct {
	rate   float64 // tokens added per second
	burst  float64 // most tokens the bucket holds
	tokens float64
	last   time.Time
}

func newRequestLimiter(rate float64, burst int, now time.Time) *requestLimiter {
	return &requestLimiter{rate: rate, burst: float64(burst), tokens: float64(burst), last: now}
}

// allow takes a token if there is one at time now.
func (l *requestLimiter) allow(now time.Time) bool {
	if elapsed := now.Sub(l.last); elapsed > 0 {
		l.tokens = min(l.burst, l.tokens+elapsed.Seconds()*l.rate)
		l.last = now
	}
	if l.tokens < 1 {
		return false
	}
	l.tokens--
	return true
}

// refuseRateLimited answers a request that went over the limit, with its
// requestId when the frame has one, so the client can match the reply.
func refuseRateLimited(client *models.Client, frame []byte) {
	var request struct {
		RequestID string `json:"requestId"`
	}
	_ = json.Unmarshal(frame, &request) // best effort: an unreadable frame gets no requestId
	sendErrorResponse(client, request.RequestID, "RATE_LIMITED", "Too many requests; slow down and retry")
}
