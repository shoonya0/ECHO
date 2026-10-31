# Access Control and Error Leak Fixes

Three security fixes, merged October 2026. Each one has a regression test.

## Chat history requires membership ([#1](https://github.com/shoonya0/ECHO/issues/1), PR [#3](https://github.com/shoonya0/ECHO/pull/3))

`GET /echo/v1/messages/:chatID/messages` returned any chat's messages to any logged-in
user who knew the chat ID. `services.GetChatMessages` now matches the chat only when
the caller is in its `participants`, and returns `404 Chat not found` otherwise, the
same as for a chat that doesn't exist, so chat IDs can't be probed.

Test: `internal/services/chat_messages_db_test.go`.

## Auth errors no longer leak internals ([#4](https://github.com/shoonya0/ECHO/issues/4), PR [#6](https://github.com/shoonya0/ECHO/pull/6))

The `500` responses from signup and login included `err.Error()`, which could expose
database and driver details. They now return only the message, with no error text.
`LoginUserResponse.PasswordHash` is tagged `json:"-"`, so the hash can't be serialized
into a response.

Tests: `internal/controller/auth.controller_test.go`,
`internal/models/requestResponse.model_test.go`.

## Only owners and admins can add group members ([#8](https://github.com/shoonya0/ECHO/issues/8), PR [#11](https://github.com/shoonya0/ECHO/pull/11))

`POST /echo/v1/groups/:groupID/add-members` had no permission check, so anyone could add
themselves to any group and then read its history, getting around the #1 fix.
Re-adding an existing participant also reset them to a pending `member`, which could
demote the owner.

`services.AddGroupMember` now:

| Case | Before | After |
| ---- | ------ | ----- |
| Caller isn't in the group | Added the users | `403 You are not a participant of this chat` |
| Caller is a plain `member` | Added the users | `403 Permission denied` |
| A user is already in the group | Entry overwritten (role reset to `member`) | Skipped, role and status kept |
| Every user is already in the group | `200` | `409 Users are already in the chat` |

The role check is repeated in the update filter, so a demotion between the read and
the write can't be overridden. The WebSocket `invite_user` request uses the same
function.

Test: `internal/services/add_group_member_db_test.go`.

## Running the database tests

The tests above that touch MongoDB create a throwaway database and drop it afterwards.
They use `TEST_DB_URI` (default `mongodb://echo:echo@localhost:27017`, the
docker-compose instance) and are **skipped**, not failed, when MongoDB isn't reachable.
Run them with `go test -count=1 -v ./internal/services` and check for `SKIP`.
