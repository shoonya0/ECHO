package services

import (
	"context"
	"errors"
	"sync"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// A→B and B→A sent at once both passed the checks, so each user ended up with
// the other in both pendingIn and pendingOut (#37). Each request now runs in a
// transaction: the two write the same documents, so one commits and the other
// retries, sees the first request and gets ErrRequestAlreadySent.
func TestContactRequestsBothWaysAtOnce(t *testing.T) {
	ctx := useTestDB(t)

	for round := range 20 {
		a, b := bson.NewObjectID(), bson.NewObjectID()
		insertTestUsers(t, ctx, a, b)

		var start sync.WaitGroup
		start.Add(1)
		errs := make(chan error, 2)
		for _, pair := range [][2]bson.ObjectID{{a, b}, {b, a}} {
			go func() {
				start.Wait()
				errs <- SendContactRequest(ctx, pair[0], pair[1])
			}()
		}
		start.Done()

		ok := 0
		for _, err := range []error{<-errs, <-errs} {
			switch {
			case err == nil:
				ok++
			case !errors.Is(err, ErrRequestAlreadySent):
				t.Fatalf("round %d: a request failed with %v, want %v", round, err, ErrRequestAlreadySent)
			}
		}
		if ok != 1 {
			t.Fatalf("round %d: %d requests succeeded, want 1", round, ok)
		}

		checkOneWayRequest(t, ctx, round, a, b)
	}
}

// checkOneWayRequest fails unless exactly one of a and b has sent the other a
// request, and the other has it as received.
func checkOneWayRequest(t *testing.T, ctx context.Context, round int, a, b bson.ObjectID) {
	t.Helper()
	ca, cb := loadContactInfo(t, ctx, a), loadContactInfo(t, ctx, b)
	aSent := countID(ca.PendingOut, b) == 1 && countID(cb.PendingIn, a) == 1
	bSent := countID(cb.PendingOut, a) == 1 && countID(ca.PendingIn, b) == 1
	if aSent == bSent {
		t.Fatalf("round %d: want exactly one direction; a: out %v in %v, b: out %v in %v",
			round, ca.PendingOut, ca.PendingIn, cb.PendingOut, cb.PendingIn)
	}
	if len(ca.PendingOut)+len(ca.PendingIn)+len(cb.PendingOut)+len(cb.PendingIn) != 2 {
		t.Fatalf("round %d: leftover entries; a: out %v in %v, b: out %v in %v",
			round, ca.PendingOut, ca.PendingIn, cb.PendingOut, cb.PendingIn)
	}
}
