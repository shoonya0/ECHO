package realtime

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/testutil"
)

// cacheName puts a fresh cache entry for the user.
func cacheName(hub *Hub, userID bson.ObjectID, name string) {
	hub.Mutex.Lock()
	hub.UserInfoCache[userID.Hex()] = &models.UserDisplayInfo{Username: name}
	hub.CacheExpiry[userID.Hex()] = time.Now().Add(time.Hour)
	hub.Mutex.Unlock()
}

// After a profile change the next live event reads the new name instead of the
// cached one, which used to last 5 minutes (#36).
func TestProfileChangeForgetsTheCachedName(t *testing.T) {
	hub := newTestHub(t)
	userID := bson.NewObjectID()
	hub.loadUserInfo = func(bson.ObjectID) (*models.UserDisplayInfo, error) {
		return &models.UserDisplayInfo{Username: "new", Avatar: "new.png"}, nil
	}
	cacheName(hub, userID, "old")

	hub.pubSubManager.forgetUserInfo(userID.Hex())

	client := newTestClient("c1", userID)
	client.Username, client.Avatar = "from-connect", ""
	if name, avatar := hub.currentName(client); name != "new" || avatar != "new.png" {
		t.Fatalf("currentName = %q, %q; want new, new.png", name, avatar)
	}
}

// A read that began before a profile change must not cache what it read: it may
// be the old name, which would then show for 5 minutes (#36).
func TestReadOverlappingAProfileChangeIsNotCached(t *testing.T) {
	hub := newTestHub(t)
	userID := bson.NewObjectID()
	var loads atomic.Int32
	hub.loadUserInfo = func(bson.ObjectID) (*models.UserDisplayInfo, error) {
		if loads.Add(1) == 1 {
			hub.pubSubManager.forgetUserInfo(userID.Hex()) // the change lands mid-read
			return &models.UserDisplayInfo{Username: "old"}, nil
		}
		return &models.UserDisplayInfo{Username: "new"}, nil
	}

	if info, err := hub.GetUserInfo(userID); err != nil || info.Username != "old" {
		t.Fatalf("first read: %v, %v", info, err)
	}
	info, err := hub.GetUserInfo(userID)
	if err != nil || info.Username != "new" {
		t.Fatalf("second read: %v, %v; want new, read again", info, err)
	}
	if n := loads.Load(); n != 2 {
		t.Fatalf("loads = %d, want 2", n)
	}
}

// A profile change on one instance clears the cached name on another, through
// Redis (#36).
func TestProfileChangeReachesOtherInstances(t *testing.T) {
	redisClient := testutil.UseTestRedis(t)
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	hub := newTestHub(t)
	receiver := NewPubSubManager(redisClient, hub.Hub, logger, "receiver")
	t.Cleanup(receiver.cancel)
	if err := receiver.subscribeToChannel(ProfileControlChannel); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(func() { _ = receiver.unsubscribeFromChannel(ProfileControlChannel) })

	userID := bson.NewObjectID()
	cacheName(hub, userID, "old")

	sender := NewPubSubManager(redisClient, &models.Hub{}, logger, "sender")
	t.Cleanup(sender.cancel)
	if err := sender.publishProfileControl(context.Background(), profileControl{UserID: userID.Hex()}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	cached := func() bool {
		hub.Mutex.RLock()
		defer hub.Mutex.RUnlock()
		_, ok := hub.UserInfoCache[userID.Hex()]
		return ok
	}
	deadline := time.Now().Add(3 * time.Second)
	for cached() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if cached() {
		t.Fatalf("the other instance still caches the old name")
	}
}
