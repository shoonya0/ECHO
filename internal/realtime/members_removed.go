package realtime

import (
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/services"

	"github.com/sirupsen/logrus"
)

// A member removed from a group must stop getting its live messages at once
// (#14). The removal travels as a hub-internal message on the chat's own Redis
// channel, right after the chat_updated event: Redis keeps a channel's order, so
// the removed users still get that event and nothing published after it, on
// every instance that has a connection in the room. Clients never see it.

// membersRemovedType is the hub-internal message type. Its Data is the list of
// removed user IDs. Handlers build every message, so clients can't send it.
const membersRemovedType = "internal.members_removed"

// The controller reaches RemoveMembersFromChat through services, so a renamed
// method must fail the build instead of silently skipping the removal.
var _ services.LiveMemberRemover = (*Hub)(nil)

func membersRemovedMessage(chatID string, userIDs []string) models.WebSocketMessage {
	return models.WebSocketMessage{Type: membersRemovedType, ChatID: chatID, Data: userIDs}
}

// RemoveMembersFromChat takes the users' open connections out of the chat's
// room on every instance. If Redis can't be reached, this instance's
// connections are still removed, and the error is returned.
func (eh *Hub) RemoveMembersFromChat(chatID string, userIDs []string) error {
	msg := membersRemovedMessage(chatID, userIDs)
	if err := eh.pubSubManager.PublishToChat(chatID, &msg); err != nil {
		if removeUsersFromRoom(eh.Hub, chatID, userIDs) {
			eh.pubSubManager.releaseEmptyChats([]string{chatID})
		}
		return err
	}
	return nil
}

// handleMembersRemoved applies a membersRemovedType message. Items that aren't
// strings are skipped.
func (pm *PubSubManager) handleMembersRemoved(chatID string, message *models.WebSocketMessage) {
	raw, ok := message.Data.([]any)
	if !ok {
		pm.logger.WithFields(logrus.Fields{"chatID": chatID}).Warn("Ignoring a members-removed message without a user list")
		return
	}
	userIDs := make([]string, 0, len(raw))
	for _, item := range raw {
		if id, ok := item.(string); ok && id != "" {
			userIDs = append(userIDs, id)
		}
	}
	if removeUsersFromRoom(pm.hub, chatID, userIDs) {
		// Not on this goroutine: it is the pub/sub reader, which every channel
		// waits on (#21).
		go pm.releaseEmptyChats([]string{chatID})
	}
}

// removeUsersFromRoom removes every connection of the users from the chat's
// room. Their sockets stay open for their other chats. It reports whether the
// room is now empty. All map access is under the hub's lock (#19).
func removeUsersFromRoom(h *models.Hub, chatID string, userIDs []string) (emptied bool) {
	h.Mutex.Lock()
	defer h.Mutex.Unlock()
	for _, userID := range userIDs {
		for _, client := range h.UserClients[userID] {
			if _, inRoom := h.ChatClients[chatID][client.ID]; inRoom && removeClientFromRoom(h, client, chatID) {
				emptied = true
			}
		}
	}
	return emptied
}
