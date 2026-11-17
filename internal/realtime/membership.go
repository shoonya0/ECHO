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

// MembersLoader returns the participants of each chat, keyed by chat ID. A chat
// that doesn't exist has no entry. services.ChatMembers is the one used
// outside tests.
type MembersLoader func(ctx context.Context, chatIDs []string) (map[string]map[string]struct{}, error)

func (eh *Hub) membersLoader() MembersLoader {
	if eh.loadMembers != nil {
		return eh.loadMembers
	}
	return loadChatMembers
}

func loadChatMembers(ctx context.Context, chatIDs []string) (map[string]map[string]struct{}, error) {
	ids := make([]bson.ObjectID, 0, len(chatIDs))
	for _, chatID := range chatIDs {
		if id, err := bson.ObjectIDFromHex(chatID); err == nil {
			ids = append(ids, id)
		}
	}
	return services.ChatMembers(ctx, ids)
}

// roomMember is one connection in a room, as the snapshot saw it.
type roomMember struct {
	clientID string
	userID   string
}

// performMembershipRecheck takes connections out of the rooms of chats their
// user isn't a participant of. Only connections seen in the snapshot taken
// before the read are removed, so a connection that entered later waits for
// the next pass. A read that fails ends the pass and removes nobody: an outage
// must not empty every room.
func (eh *Hub) performMembershipRecheck() {
	rooms := eh.snapshotRooms()
	if len(rooms) == 0 {
		return
	}

	chatIDs := make([]string, 0, len(rooms))
	for chatID := range rooms {
		chatIDs = append(chatIDs, chatID)
	}

	load := eh.membersLoader()
	var emptied []string
	for start := 0; start < len(chatIDs); start += membershipBatchSize {
		batch := chatIDs[start:min(start+membershipBatchSize, len(chatIDs))]

		ctx, cancel := context.WithTimeout(eh.Ctx, sessionCheckTimeout)
		members, err := load(ctx, batch)
		cancel()
		if err != nil {
			eh.logger.WithError(err).Warn("Membership re-check stopped: chat members couldn't be read; rooms are kept until the next pass")
			break
		}

		emptied = append(emptied, eh.removeNonMembers(batch, rooms, members)...)
	}

	if len(emptied) > 0 {
		eh.pubSubManager.releaseEmptyChats(emptied)
	}
}

// snapshotRooms copies every room whose chat ID is an object ID, under the
// hub's lock (#19). Other room names aren't chats and are left alone.
func (eh *Hub) snapshotRooms() map[string][]roomMember {
	eh.Mutex.RLock()
	defer eh.Mutex.RUnlock()

	rooms := make(map[string][]roomMember, len(eh.ChatClients))
	for chatID, clients := range eh.ChatClients {
		if _, err := bson.ObjectIDFromHex(chatID); err != nil {
			continue
		}
		room := make([]roomMember, 0, len(clients))
		for clientID, client := range clients {
			room = append(room, roomMember{clientID: clientID, userID: client.UserID.Hex()})
		}
		rooms[chatID] = room
	}
	return rooms
}

// removeNonMembers takes the snapshotted connections of non-members out of the
// batch's rooms and returns the rooms that are now empty.
func (eh *Hub) removeNonMembers(batch []string, rooms map[string][]roomMember, members map[string]map[string]struct{}) (emptied []string) {
	eh.Mutex.Lock()
	defer eh.Mutex.Unlock()

	for _, chatID := range batch {
		participants := members[chatID] // nil for a chat that doesn't exist
		for _, member := range rooms[chatID] {
			if _, ok := participants[member.userID]; ok {
				continue
			}
			client, inRoom := eh.ChatClients[chatID][member.clientID]
			if !inRoom {
				continue
			}
			eh.logger.WithFields(logrus.Fields{"clientID": member.clientID, "chatID": chatID}).
				Info("Taking a connection out of a chat its user isn't a member of")
			if removeClientFromRoom(eh.Hub, client, chatID) {
				emptied = append(emptied, chatID)
			}
		}
	}
	return emptied
}
