package realtime

import (
	"testing"
	"time"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/testutil"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// #32: presence events follow settings.privacy. Needs Redis: run with
// TEST_DB_REQUIRED=1.

// showOnlineStatus "contacts": a chat member who is a contact gets the online
// and offline; one who isn't gets neither, on this server or another.
func TestContactsOnlyPresence(t *testing.T) {
	redisClient := testutil.UseTestRedis(t)
	hub1 := newPresenceHub(t, redisClient, "one")
	hub2 := newPresenceHub(t, redisClient, "two")
	chat := bson.NewObjectID().Hex()
	a1 := bufferedClient("a1", bson.NewObjectID())
	contact := bufferedClient("contact", bson.NewObjectID())
	stranger := bufferedClient("stranger", bson.NewObjectID())
	farContact := bufferedClient("far", bson.NewObjectID())
	addClient(hub1, contact, chat)
	addClient(hub1, stranger, chat)
	addClient(hub2, farContact, chat)

	connectWithPresence(t, hub1, a1, UserPresence{
		Status: UserStatus(constants.UserStatusOnline), ShowOnline: "contacts", ShowLastSeen: true,
		Contacts: []string{contact.UserID.Hex(), farContact.UserID.Hex()},
	}, chat)
	online := nextPresence(t, contact, a1.UserID)
	wantStatus(t, online, constants.UserStatusOnline)
	wantStatus(t, nextPresence(t, farContact, a1.UserID), constants.UserStatusOnline)
	for _, key := range []string{"chats", "contactsOnly", "audience"} {
		if _, ok := online[key]; ok {
			t.Fatalf("event sent to the client still has %q: %v", key, online)
		}
	}
	noPresence(t, stranger, a1.UserID, 300*time.Millisecond)

	unregisterAndWait(hub1, a1)
	wantStatus(t, nextPresence(t, contact, a1.UserID), constants.UserStatusOffline)
	wantStatus(t, nextPresence(t, farContact, a1.UserID), constants.UserStatusOffline)
	noPresence(t, stranger, a1.UserID, 300*time.Millisecond)
}

// showOnlineStatus "none", or "contacts" with no contacts: nobody gets
// either event, like an invisible user.
func TestPresenceShownToNobody(t *testing.T) {
	for name, presence := range map[string]UserPresence{
		"none":        {Status: "online", ShowOnline: "none", ShowLastSeen: true},
		"no contacts": {Status: "online", ShowOnline: "contacts", ShowLastSeen: true},
		"unknown":     {Status: "online", ShowOnline: "friends", ShowLastSeen: true},
	} {
		t.Run(name, func(t *testing.T) {
			hub := newPresenceHub(t, testutil.UseTestRedis(t), "one")
			chat := bson.NewObjectID().Hex()
			a1 := bufferedClient("a1", bson.NewObjectID())
			b1 := bufferedClient("b1", bson.NewObjectID())
			connectOnline(t, hub, b1, chat)
			connectWithPresence(t, hub, a1, presence, chat)
			noPresence(t, b1, a1.UserID, 300*time.Millisecond)
			unregisterAndWait(hub, a1)
			noPresence(t, b1, a1.UserID, 300*time.Millisecond)
		})
	}
}

// showLastSeen false: neither event carries lastSeen.
func TestPresenceEventsHideLastSeen(t *testing.T) {
	hub := newPresenceHub(t, testutil.UseTestRedis(t), "one")
	chat := bson.NewObjectID().Hex()
	a1 := bufferedClient("a1", bson.NewObjectID())
	b1 := bufferedClient("b1", bson.NewObjectID())
	connectOnline(t, hub, b1, chat)
	connectWithPresence(t, hub, a1, UserPresence{Status: "online", ShowOnline: "everyone"}, chat)
	online := nextPresence(t, b1, a1.UserID)
	unregisterAndWait(hub, a1)
	offline := nextPresence(t, b1, a1.UserID)
	wantStatus(t, offline, constants.UserStatusOffline)
	for _, event := range []map[string]any{online, offline} {
		if seen, ok := event["lastSeen"]; ok {
			t.Fatalf("%v event has lastSeen %v; the user hides it", event["status"], seen)
		}
	}
}
