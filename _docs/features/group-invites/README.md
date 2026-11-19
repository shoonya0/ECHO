---
kind: feature
scope: [internal/services/group_invite*.go, internal/controller/group_invite*.go]
status: active
---
# Group invites: docs index

People added to a group are invited (`pending`) and accept or decline before they get the
group's messages ([#5](https://github.com/shoonya0/ECHO/issues/5), [#17](https://github.com/shoonya0/ECHO/issues/17)).
Up: [project docs index](../../README.md) · Code: `internal/services/group_invite*.go`, `internal/controller/group_invite*.go`

## Read in this order (stop when you have enough)

| Need | File |
|---|---|
| Where work stopped | [state.md](./state.md) |
| What's done and what's next | [tasks.md](./tasks.md) |
| Why it's built this way | [decisions](../../decisions/chats-and-groups.md#who-counts-as-a-chat-member) |

## Entry points (where to start reading code; not a full map)

| Path | Why start here |
|---|---|
| `internal/services/group_invites.go` | accept, decline, pending list |
| `internal/services/chat.service.go` | `newGroupParticipants`, `addGroupMembers`, `GetUserChats`, `JoinGroupByInvite` (accept by code, #103) |
| `internal/services/migrations.go` | backfill of legacy pending members |
