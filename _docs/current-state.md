# ECHO: current state

A one-page snapshot of where the project stands. Last checked on 2026-11-17 at `8fd32a6`
on `main`. To learn what the product does, read
[product-overview.md](./product-overview.md). For the code, read
[architecture-overview.md](./architecture-overview.md).

## Active work

<!-- Read by the SessionStart hook (session-context.mjs): keep this section first, short,
and with a link to exactly one feature state.md. Updated by /close-increment and
/feature-docs update. -->

- **Feature:** none · **#79** on `fix/79-hub-restart-duplicate-loops`: `Hub.Restart` and
  the controller's cancelled-context check are removed (nothing cancels the hub in
  production; the code review found it raced on `hub.Ctx` and could run two `runHub`
  loops). `startLoops` gives each background loop its context. Verified (`verify.mjs --full`)
- **#72 on Render (paused 2026-11-15):** `/health` answers `200`, but no chat with a message
  saved before PR #74 (`reactions: null`) is left to react to. Continue later, for example
  by setting `reactions: null` on a test message in Atlas first
- **Next step:** PR for #79; ask before merging. Split `rest-api.md`,
  `websocket-requests.md`, `websocket-events.md` (~199 lines) before adding to them
- **Updated:** 2026-11-17 on `fix/79-hub-restart-duplicate-loops` from `8fd32a6`

## Where things stand

The core product works: accounts, profiles, contacts, direct chats, group chats, live
messages, message history, typing, read receipts, reactions and invite codes. Many of
these still have open bugs. The full table is in
[product-overview.md](./product-overview.md#what-works-today).

Not built yet: editing or deleting messages, changing chat settings, and accepting or
declining a group invite. The full list is in
[product-overview.md](./product-overview.md#not-built-yet).

## Recently finished

- **#50 (PR #137):** `message` events carry `data.sender` and `data.mentions` from the
  saved message; `join` and `leave` carry the username and avatar the connection loaded
  at connect ([events](./websocket-events.md)). A rename shows on the next connect.
- **#48 (PR #136):** `join_chat` with `chatId` follows any chat you are in (block rule
  included), `senderId` still opens a direct chat, and both answer with a `response`
  holding `chatId`. A repeated join isn't announced again
  ([requests](./websocket-requests.md#join_chat)).
- **#132 (PR #135):** a new direct chat is kept only when both users got their `chats`
  entry (else `404` / `INVALID_USER_ID`, chat and entries removed), and opening an existing
  one writes back a missing entry ([REST](./rest-api.md#chats)).
- **#131 (PR #134):** `GET users/suggestions` takes `page` 1 to 10000 (clamped, shared
  `services.MaxPage` with the contact lists), so a huge page no longer overflows the skip
  into a `500` ([REST](./rest-api.md#finding-people)).
- **#51 (PR #133):** a message `content` is limited to 4000 characters (code points), not
  bytes, so Hindi or emoji text is no longer cut at about a third
  ([decided](./decisions.md#how-the-message-length-limit-counts-51),
  [app changes](../Android/_docs/08_SERVER_CHANGES/messages.md)).
- **#49 (PR #130):** a chat a user gains while connected (a new direct chat, either side; a
  group they create; a code join, also a retry or a pending accept) reaches their open
  connections at once, through a hub-internal `internal.chat_added` on `user:<id>`
  ([pub/sub](./PUBSUB_ARCHITECTURE.md#connecting-and-disconnecting),
  [app changes](../Android/_docs/08_SERVER_CHANGES/groups-and-invites.md)). Not done from
  the review: a connection registering in the same ms misses the add; a removal racing an
  add is undone by the #117 re-check within 30 s.
- **#39 (PR #129):** the five contact lists come in pages: `page` 1 to 10000, cut to
  `limit`, sorted by username ignoring case, with the real `total`
  ([REST](./rest-api.md#contacts), [app changes](../Android/_docs/08_SERVER_CHANGES/contacts-and-blocking.md)).
  Not done from the review: offset pages can shift when the list changes; `users/suggestions`
  has the same unbounded `page`.
- **#117 (PR #128):** every 30 s each server reads the participants of its rooms' chats
  (`services.ChatMembers`, keys only) and takes out connections of users who aren't
  members any more ([pub/sub](./PUBSUB_ARCHITECTURE.md#background-jobs)). Not done from the
  code review: a member removed and re-added within one pass stays out until they
  reconnect (#49); evicted connections get no event.
- **#123 + #124 (PR #127):** a join or registration releases the chat channel again after
  subscribing if a removal emptied the room meanwhile, and no `join` is announced for a
  member removed during the join. The release test hooks are `PubSubManager` fields
  ([pub/sub](./PUBSUB_ARCHITECTURE.md#connecting-and-disconnecting)). Not done from the
  code review: a removal before the room entry (#117), one per-chat reconcile step.
- **#40, typing part (PR #126):** `set_typing` only broadcasts the `typing` event; it no
  longer writes `typingUsers` to MongoDB or starts a timer that cleared users still typing
  ([requests](./websocket-requests.md#set_typing),
  [app changes](../Android/_docs/08_SERVER_CHANGES/auth-and-connection.md)).
- **#122 (PR #125):** the server reads a connection's requests only after the hub has
  registered it (at most 10 s), so a `join_chat` sent right after connecting is no longer
  dropped ([pub/sub](./PUBSUB_ARCHITECTURE.md#connecting-and-disconnecting)).
- **#54 (PR #121):** a disconnect the hub handles before the connection's registration
  closes its send queue and drops the presence it stored; the registration then skips the
  closed connection ([pub/sub](./PUBSUB_ARCHITECTURE.md#connecting-and-disconnecting)).
  From the code review: #122, #123, #124 and a comment on #22.
- **#20 + #55 (PR #120):** rooms are tracked per connection (`ClientChats`). Every room
  removal (disconnect, `leave_chat`, `remove_user`) releases the chat channels left
  unused, a user's last connection releases `user:<id>`, and a join racing a release keeps
  its subscription ([pub/sub](./PUBSUB_ARCHITECTURE.md#connecting-and-disconnecting)).
- **#116 (PR #119):** a user's last connection closing leaves every chat and unsubscribes
  each of their chat channels (the loop used to skip some).
- **#14 (PR #118):** a member removed with `remove_user` gets the `chat_updated` and then
  no more live events from the group, on every server: a hub-internal message on the
  chat's Redis channel takes their connections out of the room
  ([events](./websocket-events.md), [app changes](../Android/_docs/08_SERVER_CHANGES/groups-and-invites.md)).
  The Android log is split into topic pages. Security review: nothing found; code review: #116, #117.
- **#106 (PR #115):** `send_message` and `invite_user` read the chat once, in the same
  query that checks membership (`services.GetChatForMember`); replies are unchanged.
  `remove_user` still reads it twice.
- **#112 (PR #114):** creating a group checks the write that adds it to the creator's chat
  list; if that write fails or finds no user, the new group is removed and the request
  fails (`404` or `500`) before any invitation goes out.
- **#103 (PR #113):** a `pending` group member accepts by joining with any usable code of
  the group; `send` works for pending members, and joining clears every invitation to the
  group. Live messages start at the next connection ([REST](./rest-api.md#groups),
  [app changes](../Android/_docs/08_SERVER_CHANGES/README.md)). A dedicated accept and decline
  action is left to #5 ([decisions](./decisions.md#how-a-pending-member-accepts-103-5)).
- **#68 (PR #111):** a group holds at most `settings.maxParticipants` (default 100). Create,
  add and join past it add nobody: REST `409`, WebSocket `GROUP_FULL`, checked in the
  update filter ([REST](./rest-api.md#groups), [app changes](../Android/_docs/08_SERVER_CHANGES/README.md)).
  From the code review: #112.
- **#16 (PR #109):** a group's `stats.participantCount` follows every add, join and
  removal, and a startup backfill resets drifted groups (skipping malformed ones). It
  counts pending members and deleted accounts ([app changes](../Android/_docs/08_SERVER_CHANGES/README.md)).
  From the code review: #110 (derive the count instead of maintaining it).
- **#62 (PR #108):** `GET users/contacts` leaves out people you blocked; `GET /blocked`
  lists them ([REST](./rest-api.md#contacts), [app changes](../Android/_docs/08_SERVER_CHANGES/README.md)).
- **#104 (PR #107):** creating an invite code returns it in `data`, shaped like an entry
  of the group's list ([REST](./rest-api.md#groups),
  [app changes](../Android/_docs/08_SERVER_CHANGES/README.md)).
- **#59 (PR #105):** a non-member gets the same answer whether or not a chat exists:
  `PERMISSION_DENIED` from `send_message`, `invite_user` and `remove_user`, `404` from
  REST `add-members` ([protocol](./WEBSOCKET_PROTOCOL.md#error-codes),
  [app changes](../Android/_docs/08_SERVER_CHANGES/README.md)). Security review: nothing found.
  From the code review: #106.
- **#30 (PR #102):** only owners and admins create, list, delete or send invite codes or
  see who joined: other members get `403`, anyone not in the group `404`
  ([REST](./rest-api.md#groups), [app changes](../Android/_docs/08_SERVER_CHANGES/README.md)).
  Security review: nothing found. From the code review: #103, #104.
Older work, from #67 (PR #101) back: [finished-work.md](./finished-work.md).

## How to build, run, test

Step-by-step commands are in [getting-started.md](./getting-started.md). The one command
to remember is `node scripts/verify.mjs`.

## Test baseline

Run on 2026-11-17 on `main` at `8fd32a6`, on Windows, with MongoDB and Redis running and
`TEST_DB_REQUIRED=1`, through `node scripts/verify.mjs --full` after `go clean -testcache`.
The lint row is from 2026-11-07 at `90b2f73`. Compare new runs against this table by test
name.

| Command | Duration | Result | Known failures (by name) |
|---|---|---|---|
| `go build ./...` | 2s | pass | — |
| `go vet ./...` | 1s | pass | — |
| `gofmt -l cmd internal` | <1s | pass (no files) | — |
| `go test ./...` | 5s | pass, none skipped | — |
| `go test -race ./...` | 14s | pass, none skipped | — |
| `golangci-lint run ./...` (v2.14) | 11s | **27 issues**: errcheck 20, staticcheck 7 | not blocking: verify lints only new code (`--new-from-merge-base=origin/main`) |

- Tests that need MongoDB skip when it isn't running, unless `TEST_DB_REQUIRED` is set. CI
  sets it.
- `internal/realtime/hub_race_test.go` uses the hub from several goroutines at once. It only
  catches races under `go test -race`.
- The contract test (`scripts/contract-test`) isn't part of verify; it needs a running
  server. All its steps passed on 2026-11-15 (#67).

## Open decisions

None right now. What has been decided, and what each choice unblocks, is in
[decisions.md](./decisions.md): invite permissions (#30), the message length limit (#51), accepting a group invite (#103, #5), membership in `participants`
only (#14, #17), soft-deleting accounts (#35), transactions on a replica set (#37), how
blocks are stored, and who counts as a member.

## Known issues and risks

There are 22 open issues. Where each risk sits in the code: [architecture-overview.md](./architecture-overview.md#riskiest-areas).

| Area | The problem, in short | Issues |
|---|---|---|
| Live connections | The server can leak resources or stall under load, missed live messages can't be detected, and open connections miss new chats, and a restart of a running hub doubles its loops | #21, #22, #43, #49, #79, #117 |
| Groups | Members are never asked to accept, pending members get no live messages, and the participant count is maintained by hand | #5, #17, #110 |
| Invite codes | Entries left by forged joins before #9 was fixed are still stored | #69 |
| Accounts | Deleting an account leaves old references, emails are case-sensitive, and usernames aren't unique | #35, #36 |
| Contacts and blocking | Contact lists aren't cut to `limit`, and blocks written during a deploy can be missed | #39, #63 |
| Online status and privacy | Online status is unreliable, and privacy settings aren't applied | #31, #32 |
| Messages and data | Writes can half-finish, and some lists grow without limit | #37, #42 |
| Limits and errors | WebSocket limits are per connection, not per user; no limits on login and signup | #40 |
| WebSocket replies and events | Replies and events are inconsistent or leave out details, which affects the Android plan | #50 |

## Next task

Pick the next issue from the table above. Commit this state update as the first commit on
that branch.
