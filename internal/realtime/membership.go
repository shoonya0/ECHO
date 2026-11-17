package realtime

import (
	"context"

	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/services"
)

// A member removed from a group leaves its rooms at once through the
// members-removed message (#14). That can miss a connection: a reconnect whose
// registration read the user's chats before the removal enters the room after
// the message ran, and a Redis failure during remove_user leaves the other
// instances' connections in. So every SessionRecheckInterval each instance
// also checks every room against the chat's participants and takes out the
// connections of users who aren't members any more (#117). The sockets stay
// open for their other chats, as with remove_user.

// membershipBatchSize is how many chats one membership query reads.
const membershipBatchSize = 200

// MembersLoader returns the participants of each chat, keyed by the chat's hex
// ID. A chat that doesn't exist has no entry. services.ChatMembers is the one
// used outside tests.
type MembersLoader func(ctx context.Context, chatIDs []bson.ObjectID) (map[string]map[string]struct{}, error)

func (eh *Hub) membersLoader() MembersLoader {
	if eh.loadMembers != nil {
		return eh.loadMembers
	}
	return services.ChatMembers
}

// roomMember is one connection in a room, as the snapshot saw it.
type roomMember struct {
	clientID string
	userID   string
}

// room is a chat's room as the snapshot saw it, with the chat ID parsed once.
type room struct {
	chatID  string
	id      bson.ObjectID
	members []roomMember
}

// performMembershipRecheck takes connections out of the rooms of chats their
// user isn't a participant of. Only connections seen in the snapshot taken
// before the read are removed, so a connection that entered later waits for
// the next pass. A read that fails ends the pass and removes nobody: an outage
// must not empty every room, and the snapshot's random order checks other
// rooms first next time.
func (eh *Hub) performMembershipRecheck() {
	rooms := eh.snapshotRooms()

	load := eh.membersLoader()
	var emptied []string
	for start := 0; start < len(rooms); start += membershipBatchSize {
		batch := rooms[start:min(start+membershipBatchSize, len(rooms))]
		ids := make([]bson.ObjectID, len(batch))
		for i, r := range batch {
			ids[i] = r.id
		}

		ctx, cancel := context.WithTimeout(eh.Ctx, sessionCheckTimeout)
		members, err := load(ctx, ids)
		cancel()
		if err != nil {
			eh.logger.WithError(err).Warn("Membership re-check stopped: chat members couldn't be read; rooms are kept until the next pass")
			break
		}

		emptied = append(emptied, eh.removeNonMembers(batch, members)...)
	}

	if len(emptied) > 0 {
		eh.pubSubManager.releaseEmptyChats(emptied)
	}
}

// snapshotRooms copies every room whose chat ID is an object ID, under the
// hub's lock (#19). Other room names aren't chats and are left alone.
func (eh *Hub) snapshotRooms() []room {
	eh.Mutex.RLock()
	defer eh.Mutex.RUnlock()

	rooms := make([]room, 0, len(eh.ChatClients))
	for chatID, clients := range eh.ChatClients {
		id, err := bson.ObjectIDFromHex(chatID)
		if err != nil {
			continue
		}
		r := room{chatID: chatID, id: id, members: make([]roomMember, 0, len(clients))}
		for clientID, client := range clients {
			r.members = append(r.members, roomMember{clientID: clientID, userID: client.UserID.Hex()})
		}
		rooms = append(rooms, r)
	}
	return rooms
}

// removeNonMembers takes the snapshotted connections of non-members out of the
// batch's rooms and returns the rooms that are now empty.
func (eh *Hub) removeNonMembers(batch []room, members map[string]map[string]struct{}) (emptied []string) {
	eh.Mutex.Lock()
	defer eh.Mutex.Unlock()

	for _, r := range batch {
		participants := members[r.chatID] // nil for a chat that doesn't exist
		for _, member := range r.members {
			if _, ok := participants[member.userID]; ok {
				continue
			}
			client, inRoom := eh.ChatClients[r.chatID][member.clientID]
			if !inRoom {
				continue
			}
			eh.logger.WithFields(logrus.Fields{"clientID": member.clientID, "chatID": r.chatID}).
				Info("Taking a connection out of a chat its user isn't a member of")
			if removeClientFromRoom(eh.Hub, client, r.chatID) {
				emptied = append(emptied, r.chatID)
			}
		}
	}
	return emptied
}
