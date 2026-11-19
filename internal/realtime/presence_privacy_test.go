package realtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/services"
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

// The user's own other connections get their contacts-only events: they
// aren't in their own contacts list (#32 review).
func TestContactsOnlyPresenceReachesOwnDevices(t *testing.T) {
	hub := newPresenceHub(t, testutil.UseTestRedis(t), "one")
	chat := bson.NewObjectID().Hex()
	userA := bson.NewObjectID()
	tablet := bufferedClient("tablet", userA)
	phone := bufferedClient("phone", userA)
	addClient(hub, tablet, chat)
	connectWithPresence(t, hub, phone, UserPresence{
		Status: "online", ShowOnline: "contacts", ShowLastSeen: true, Contacts: []string{bson.NewObjectID().Hex()},
	}, chat)
	wantStatus(t, nextPresence(t, tablet, userA), constants.UserStatusOnline)
}

// The offline follows the settings at disconnect, not at connect (#32
// review): it still reaches everyone the online did, so nobody is left
// showing the user online, but carries lastSeen only if the current settings
// let all of them see it.
func TestOfflineFollowsCurrentSettings(t *testing.T) {
	contact, dropped := bson.NewObjectID(), bson.NewObjectID()
	settings := func(scope string, showLastSeen bool, contacts ...bson.ObjectID) func(context.Context, bson.ObjectID) (services.PresenceSettings, error) {
		return func(context.Context, bson.ObjectID) (services.PresenceSettings, error) {
			return services.PresenceSettings{Status: constants.UserStatusOnline, Contacts: contacts,
				Privacy: models.PrivacySettingsEmbed{ShowOnlineStatus: scope, ShowLastSeen: showLastSeen}}, nil
		}
	}
	cases := []struct {
		name     string
		current  func(context.Context, bson.ObjectID) (services.PresenceSettings, error)
		lastSeen bool
	}{
		{"unchanged", settings("contacts", true, contact, dropped), true},
		{"turned lastSeen off", settings("contacts", false, contact, dropped), false},
		{"now shown to nobody", settings("none", true), false},
		{"a contact removed", settings("contacts", true, contact), false},
		{"a contact added", settings("contacts", true, contact, dropped, bson.NewObjectID()), true},
		{"settings unreadable", func(context.Context, bson.ObjectID) (services.PresenceSettings, error) {
			return services.PresenceSettings{}, errors.New("mongo down")
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hub := newPresenceHub(t, testutil.UseTestRedis(t), "one")
			hub.loadPresence = tc.current
			chat := bson.NewObjectID().Hex()
			a1 := bufferedClient("a1", bson.NewObjectID())
			b1 := bufferedClient("b1", contact)
			addClient(hub, b1, chat)
			connectWithPresence(t, hub, a1, UserPresence{
				Status: "online", ShowOnline: "contacts", ShowLastSeen: true,
				Contacts: []string{contact.Hex(), dropped.Hex()},
			}, chat)
			wantStatus(t, nextPresence(t, b1, a1.UserID), constants.UserStatusOnline)

			unregisterAndWait(hub, a1)
			offline := nextPresence(t, b1, a1.UserID)
			wantStatus(t, offline, constants.UserStatusOffline)
			if _, has := offline["lastSeen"]; has != tc.lastSeen {
				t.Fatalf("offline has lastSeen = %v, want %v", has, tc.lastSeen)
			}
		})
	}
}
