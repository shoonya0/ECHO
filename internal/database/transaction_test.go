package database

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

// A transaction that finally fails in MongoDB is logged at error; the callers'
// own errors (not found, already sent) are not (#37).
func TestFailedInMongo(t *testing.T) {
	errCheck := errors.New("already sent")
	conflict := mongo.CommandError{Code: 112, Name: "WriteConflict", Labels: []string{"TransientTransactionError"}}
	commit := mongo.CommandError{Code: 50, Labels: []string{"UnknownTransactionCommitResult"}}
	wrappedConflict := fmt.Errorf("failed to update user: %w", conflict)
	wrappedDeadline := fmt.Errorf("failed to fetch user: %w", context.DeadlineExceeded)

	for _, tc := range []struct {
		name       string
		err, fnErr error
		want       bool
	}{
		{"caller's own check", errCheck, errCheck, false},
		{"caller's check, wrapped", fmt.Errorf("x: %w", errCheck), errCheck, false},
		{"commit failed after fn succeeded", commit, nil, true},
		{"retries ran out on a conflict", wrappedConflict, wrappedConflict, true},
		{"context ended inside fn", wrappedDeadline, wrappedDeadline, true},
	} {
		if got := failedInMongo(tc.err, tc.fnErr); got != tc.want {
			t.Errorf("%s: failedInMongo = %v, want %v", tc.name, got, tc.want)
		}
	}
}
