package services

import (
	"context"
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
// each ID twice (#37). The lists now hold each ID at most once.
func TestConcurrentContactWritesStoreEachIDOnce(t *testing.T) {
	ctx := useTestDB(t)

	t.Run("send request", func(t *testing.T) {
		sender, target := bson.NewObjectID(), bson.NewObjectID()
		insertTestUsers(t, ctx, sender, target)

		concurrently(10, func() { _ = SendContactRequest(ctx, sender, target) }) // the losers get "already sent"

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

		concurrently(10, func() { _ = AddToFavorites(ctx, user, friend) })

		if n := countID(loadContactInfo(t, ctx, user).Favorites, friend); n != 1 {
			t.Errorf("favorites hold the friend %d times, want 1", n)
		}
	})
}
