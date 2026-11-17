package realtime

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// #117: a connection that entered a room after its user was removed from the
// chat (a reconnect racing remove_user, or a members-removed message lost to a
// Redis failure) is taken out by the next membership re-check.

// membersOf is a loadMembers stub: chat ID -> member user IDs. A chat missing
// from it doesn't exist.
func membersOf(chats map[string][]bson.ObjectID) func(context.Context, []bson.ObjectID) (map[string]map[string]struct{}, error) {
	return func(_ context.Context, chatIDs []bson.ObjectID) (map[string]map[string]struct{}, error) {
		result := make(map[string]map[string]struct{})
		for _, chatID := range chatIDs {
			users, ok := chats[chatID.Hex()]
			if !ok {
				continue
			}
			result[chatID.Hex()] = make(map[string]struct{})
			for _, userID := range users {
				result[chatID.Hex()][userID.Hex()] = struct{}{}
			}
		}
		return result, nil
	}
}

func TestMembershipRecheck(t *testing.T) {
	group, other := bson.NewObjectID().Hex(), bson.NewObjectID().Hex()
	stay, gone := bson.NewObjectID(), bson.NewObjectID()

	t.Run("removes connections of users who aren't members, and keeps the rest", func(t *testing.T) {
		hub := newTestHub(t)
		a1, b1, b2 := bufferedClient("a1", stay), bufferedClient("b1", gone), bufferedClient("b2", gone)
		addClient(hub, a1, group, other)
		addClient(hub, b1, group, other)
		addClient(hub, b2, group)
		hub.loadMembers = membersOf(map[string][]bson.ObjectID{
			group: {stay},
			other: {stay, gone},
		})

		hub.performMembershipRecheck()

		if !inRoom(hub, group, a1.ID) {
			t.Error("a member's connection left the room")
		}
		for _, c := range []string{b1.ID, b2.ID} {
			if inRoom(hub, group, c) {
				t.Errorf("%s is still in the room of a chat its user left", c)
			}
		}
		if !inRoom(hub, other, b1.ID) {
			t.Error("the removed user's connection left a chat they are still in")
		}
		if b1.IsClosed() || b2.IsClosed() {
			t.Error("a connection was closed; it must stay open for its other chats")
		}
	})

	t.Run("a deleted chat empties its room and releases the channel", func(t *testing.T) {
		hub := newTestHub(t)
		addClient(hub, bufferedClient("a1", stay), group)
		hub.loadMembers = membersOf(map[string][]bson.ObjectID{})
		var released []string
		hub.pubSubManager.afterEmptyCheck = func(chatID string) { released = append(released, chatID) }

		hub.performMembershipRecheck()

		if inRoom(hub, group, "a1") {
			t.Error("a connection is still in the room of a deleted chat")
		}
		if len(released) != 1 || released[0] != group {
			t.Errorf("released %v, want [%s]", released, group)
		}
	})

	t.Run("a failed read removes nobody", func(t *testing.T) {
		hub := newTestHub(t)
		addClient(hub, bufferedClient("b1", gone), group)
		hub.loadMembers = func(context.Context, []bson.ObjectID) (map[string]map[string]struct{}, error) {
			return nil, errors.New("mongo is down")
		}

		hub.performMembershipRecheck()

		if !inRoom(hub, group, "b1") {
			t.Error("an outage took a connection out of its room")
		}
	})

	t.Run("a connection that entered the room after the snapshot is kept", func(t *testing.T) {
		hub := newTestHub(t)
		b1, b2 := bufferedClient("b1", gone), bufferedClient("b2", gone)
		addClient(hub, b1, group)
		load := membersOf(map[string][]bson.ObjectID{group: {}})
		hub.loadMembers = func(ctx context.Context, chatIDs []bson.ObjectID) (map[string]map[string]struct{}, error) {
			addClient(hub, b2, group) // registered while the chats were read
			return load(ctx, chatIDs)
		}

		hub.performMembershipRecheck()

		if inRoom(hub, group, b1.ID) {
			t.Error("the snapshotted connection is still in the room")
		}
		if !inRoom(hub, group, b2.ID) {
			t.Error("a connection the pass never read about was removed; the next pass checks it")
		}
	})

	t.Run("chat IDs that aren't object IDs are left alone", func(t *testing.T) {
		hub := newTestHub(t)
		addClient(hub, bufferedClient("b1", gone), "not-an-id")
		var asked []bson.ObjectID
		hub.loadMembers = func(_ context.Context, chatIDs []bson.ObjectID) (map[string]map[string]struct{}, error) {
			asked = append(asked, chatIDs...)
			return map[string]map[string]struct{}{}, nil
		}

		hub.performMembershipRecheck()

		if len(asked) != 0 {
			t.Errorf("asked about %v", asked)
		}
		if !inRoom(hub, "not-an-id", "b1") {
			t.Error("a connection left a room that wasn't checked")
		}
	})
}

// Run with -race: the pass reads and changes the hub's maps while connections
// join and leave.
func TestMembershipRecheckRace(t *testing.T) {
	hub := newTestHub(t)
	group := bson.NewObjectID().Hex()
	gone := bson.NewObjectID()
	hub.loadMembers = membersOf(map[string][]bson.ObjectID{group: {}})

	runTogether(
		func(int) { hub.performMembershipRecheck() },
		func(i int) { addClient(hub, bufferedClient(fmt.Sprintf("c%d", i), gone), group) },
		func(int) {
			hub.Mutex.Lock()
			for _, client := range hub.ChatClients[group] {
				removeClientFromRoom(hub.Hub, client, group)
				break
			}
			hub.Mutex.Unlock()
		},
	)
}
