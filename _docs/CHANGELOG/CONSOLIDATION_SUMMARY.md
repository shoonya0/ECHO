# Change note: one chat model instead of two

> **This is history.** It records a past change and isn't a guide to today's code. Some of
> it is no longer true: see [what has changed since](#what-has-changed-since).

## The problem

The code had two models for the same thing: `Chat` for saved data and `ChatRoom` for live
connections. Both had to be kept in step, and the same user details were stored in many
places.

## What changed

- **One model:** `ChatRoom` and its related types were removed. `Chat` now covers both saved
  data and live tracking. The hub's `ChatRooms` became `ChatClients`.
- **Smaller `Client`:** repeated user data, such as `Username`, was removed from the
  connection model.
- **Less copied user data:** participants stored a reference (`ParticipantRef`) instead of
  the user's name and avatar.
- **User cache:** a new lookup service kept display names for 5 minutes, with batch lookups.
- **Helpers:** new functions covered display names, permission checks and active client
  counts.

## Why it helped

- There was one source of truth for a chat, so the two models could no longer disagree.
- The note claimed about 60% less repeated user data. That figure hasn't been measured
  again.
- Group chats with many people were supported, and existing APIs kept working with no
  database migration.

## What has changed since

- The hub moved from `internal/services/websocket.hub.go` to `internal/realtime/hub.go`.
- `user_lookup.service.go` and `chat_helper.service.go` no longer exist. The 5-minute cache
  now lives in the hub.
- `ParticipantRef` is gone. Participants are stored as `ParticipantEmbed`, which includes a
  copy of the user's details.
- `main.go` moved to `cmd/server/main.go`.
- "Single source of truth" held for chat state, but chat membership is still stored in two
  places. See [architecture-overview.md](../architecture-overview.md#data-model).
