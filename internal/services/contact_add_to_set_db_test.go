package services

import (
	"context"
	"errors"
	"sync"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"
)

// concurrently runs f n times at once and waits for all of them.
func concurrently(n int, f func()) {
	var start, done sync.WaitGroup
	start.Add(1)
	for range n {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			f()
		}()
	}
	start.Done()
	done.Wait()
}

// collectErrors runs f n times at once and returns what each call returned.
func collectErrors(n int, f func() error) []error {
	errs := make(chan error, n)
	concurrently(n, func() { errs <- f() })
	close(errs)
	var out []error
	for err := range errs {
		out = append(out, err)
	}
	return out
}

// checkOneSucceeded fails unless exactly one call returned nil and every other
// returned want.
func checkOneSucceeded(t *testing.T, errs []error, want error) {
	t.Helper()
	ok := 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case !errors.Is(err, want):
			t.Errorf("a call failed with %v, want %v", err, want)
		}
	}
	if ok != 1 {
		t.Errorf("%d calls succeeded, want 1", ok)
	}
}

func loadContactInfo(t *testing.T, ctx context.Context, id bson.ObjectID) models.ContactInfoEmbed {
	t.Helper()
	var u struct {
		ContactInfo models.ContactInfoEmbed `bson:"contactInfo"`
	}
	if err := database.DB.Collection(string(constants.UserColl)).FindOne(ctx, bson.M{"_id": id}).Decode(&u); err != nil {
		t.Fatalf("load user: %v", err)
	}
	return u.ContactInfo
}

func countID(ids []bson.ObjectID, id bson.ObjectID) int {
	n := 0
	for _, x := range ids {
		if x == id {
			n++
		}
	}
	return n
}

// A double tap sent two requests that both passed the checks, and $push stored
// each ID twice (#37). The writes are now conditional: one call succeeds, the
// others get the error a second call gets, and each ID is stored once.
func TestConcurrentContactWritesStoreEachIDOnce(t *testing.T) {
	ctx := useTestDB(t)

	t.Run("send request", func(t *testing.T) {
		sender, target := bson.NewObjectID(), bson.NewObjectID()
		insertTestUsers(t, ctx, sender, target)

		errs := collectErrors(10, func() error { return SendContactRequest(ctx, sender, target) })
		checkOneSucceeded(t, errs, ErrRequestAlreadySent)

		if n := countID(loadContactInfo(t, ctx, sender).PendingOut, target); n != 1 {
			t.Errorf("sender's pendingOut holds the target %d times, want 1", n)
		}
		if n := countID(loadContactInfo(t, ctx, target).PendingIn, sender); n != 1 {
			t.Errorf("target's pendingIn holds the sender %d times, want 1", n)
		}
	})

	t.Run("add favorite", func(t *testing.T) {
		user, friend := bson.NewObjectID(), bson.NewObjectID()
		insertTestUsers(t, ctx, user, friend)
		if _, err := database.DB.Collection(string(constants.UserColl)).UpdateOne(ctx,
			bson.M{"_id": user}, bson.M{"$set": bson.M{"contactInfo.contacts": []bson.ObjectID{friend}}}); err != nil {
			t.Fatalf("make contacts: %v", err)
		}

		errs := collectErrors(10, func() error { return AddToFavorites(ctx, user, friend) })
		checkOneSucceeded(t, errs, ErrAlreadyInFavorites)

		if n := countID(loadContactInfo(t, ctx, user).Favorites, friend); n != 1 {
			t.Errorf("favorites hold the friend %d times, want 1", n)
		}
	})
}
