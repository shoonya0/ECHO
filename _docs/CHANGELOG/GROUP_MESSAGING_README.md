# Change note: first version of group messaging

> **This is history.** It records an early version and isn't a guide to today's code. Most
> of the details below have since changed: see
> [what has changed since](#what-has-changed-since).

## What changed

This was the first working group chat over WebSocket. To keep it simple and predictable,
the early version:

- **Removed parallel processing.** The hub and chat service stopped using goroutines and Go
  channels, and called functions directly, one step at a time.
- **Moved WebSocket login to the URL.** Connections sent `?token=<jwt_token>`, because
  browsers can't easily set a WebSocket header.

## What it could do

- Create a group chat
- Send messages to all group members in real time
- Show join and leave notifications
- Show typing indicators

The note listed these as future work: editing and deleting messages, attachments,
reactions, roles and permissions, and message threads.

## The trade-off

Running one step at a time was easier to reason about, but slower under heavy traffic. The
note warned that parallel processing might have to come back for production use, and it did.

## What has changed since

- **Parallel processing is back.** The hub uses Go channels (`Register`, `Unregister`) and
  goroutines. See [PUBSUB_ARCHITECTURE.md](../PUBSUB_ARCHITECTURE.md).
- **Login moved back to the header.** The server only accepts `Authorization: Bearer <token>`.
  `?token=` no longer works.
- **Paths changed.** `/api/v1/` is now `/echo/v1/`, and `go run main.go` is now
  `go run ./cmd/server`.
- **Message history changed.** `GET /api/v1/chats/messages?chatId=<id>` is now
  `GET /echo/v1/messages/:chatID/messages`.
- **Reactions and roles were built.** Attachment details can be sent too, but there is no
  file upload. Editing, deleting and threads are still not built.
- **`join_chat` changed.** It now opens a direct chat and can't be used to enter a group. See
  [websocket-requests.md](../websocket-requests.md#join_chat).

For how things work today, read [WEBSOCKET_PROTOCOL.md](../WEBSOCKET_PROTOCOL.md).
