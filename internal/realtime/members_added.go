package realtime

import (
	"errors"

	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/services"
)

// A connection reads its user's chats once, when it is registered. A chat the
// user gains later (a new direct chat, a new group, an invite join) must be
// followed by their open connections at once (#49), the reverse of the removal
// in members_removed.go. The add travels as a hub-internal message on the
// user's own Redis channel, which every instance with a connection of that
// user subscribes to. Clients never see it, and no join event is sent: nobody
// joined from the clients' point of view.

// chatAddedType is the hub-internal message type. Its ChatID is the chat the
// user gained. Services build it after their write succeeded; clients can't
// send it.
const chatAddedType = "internal.chat_added"

// The services reach AddMembersToChat through an interface, so a renamed
// method must fail the build instead of silently skipping the add.
var _ services.LiveMemberAdder = (*Hub)(nil)

func chatAddedMessage(chatID string) models.WebSocketMessage {
	return models.WebSocketMessage{Type: chatAddedType, ChatID: chatID}
}

// AddMembersToChat puts the users' open connections in the chat's room on every
// instance. If Redis can't be reached, this instance's connections are still
// added, and the error is returned.
func (eh *Hub) AddMembersToChat(chatID string, userIDs []string) error {
	var errs []error
	for _, userID := range userIDs {
		msg := chatAddedMessage(chatID)
		if err := eh.pubSubManager.publishMessage(eh.pubSubManager.getUserChannel(userID), &msg); err != nil {
			if eh.pubSubManager.joinUserConnections(userID, chatID) {
				eh.pubSubManager.subscribeChats([]string{chatID})
			}
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// handleChatAdded applies a chatAddedType message received on the user's
// channel.
func (pm *PubSubManager) handleChatAdded(userID string, message *models.WebSocketMessage) {
	if message.ChatID == "" {
		pm.logger.Warn("Ignoring a chat-added message without a chat ID")
		return
	}
	pm.addUserToRoom(userID, message.ChatID)
}

// addUserToRoom puts the user's open connections on this instance in the chat's
// room and subscribes to the chat's channel. The subscribe runs off the
// caller's goroutine, which is the reader of the user's channel.
func (pm *PubSubManager) addUserToRoom(userID, chatID string) {
	if pm.joinUserConnections(userID, chatID) {
		go pm.subscribeChats([]string{chatID})
	}
}

// joinUserConnections adds every registered, open connection of the user to the
// chat's room. A connection that closed or was unregistered is skipped: nothing
// would take it out of the room again. It reports whether any was added. All
// map access is under the hub's lock (#19).
func (pm *PubSubManager) joinUserConnections(userID, chatID string) (added bool) {
	pm.hub.Mutex.Lock()
	defer pm.hub.Mutex.Unlock()
	for clientID, client := range pm.hub.UserClients[userID] {
		if _, registered := pm.hub.Clients[clientID]; !registered || client.IsClosed() {
			continue
		}
		addClientToRoom(pm.hub, client, chatID)
		added = true
	}
	return added
}
