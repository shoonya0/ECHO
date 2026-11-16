# ECHO: current state

A one-page snapshot of where the project stands. Last checked on 2026-11-15 at `c914ce2`
on `main`. To learn what the product does, read
[product-overview.md](./product-overview.md). For the code, read
[architecture-overview.md](./architecture-overview.md).

## Active work

<!-- Read by the SessionStart hook (session-context.mjs): keep this section first, short,
and with a link to exactly one feature state.md. Updated by /close-increment and
/feature-docs update. -->

- **Feature:** none yet · #59 on `fix/59-hide-chat-existence`: `send_message`, `invite_user`
  and `remove_user` must answer a non-member with `PERMISSION_DENIED` whether or not the
  chat exists
- **#72 on Render (paused 2026-11-15):** `/health` answers `200`, but no chat with a message
  saved before PR #74 (`reactions: null`) is left to react to. Continue later, for example
  by setting `reactions: null` on a test message in Atlas first
- **Next step:** failing test first (unknown chat, direct chat), then check membership first
  in the three handlers
- **Updated:** 2026-11-16 on `fix/59-hide-chat-existence`

## Where things stand

The core product works: accounts, profiles, contacts, direct chats, group chats, live
messages, message history, typing, read receipts, reactions and invite codes. Many of
these still have open bugs. The full table is in
[product-overview.md](./product-overview.md#what-works-today).

Not built yet: editing or deleting messages, changing chat settings, and accepting or
declining a group invite. The full list is in
[product-overview.md](./product-overview.md#not-built-yet).

## Recently finished

- **#30 (PR #102):** only owners and admins create, list, delete or send invite codes or
  see who joined: other members get `403`, anyone not in the group `404`
  ([REST](./rest-api.md#groups), [app changes](../Android/_docs/08_SERVER_CHANGES.md)).
  Security review: nothing found. From the code review: #103, #104.
- **#67 (PR #101):** the contract test pages history with `?before=<nextCursor>` and checks
  that `offset` gets `400`; every step passes against a local server.
- **#60 (PR #100):** a `chatId` in any case is lowercased where WebSocket requests come
  in, so its events reach the chat ([requests](./websocket-requests.md)).
- **#47 (PR #99):** `send_message` replies have a top-level `requestId` ([events](./websocket-events.md)).
- **#75 (PR #98):** reaction `details` hold usernames; older entries keep `""`
  ([REST](./rest-api.md#messages), [app changes](../Android/_docs/08_SERVER_CHANGES.md)).
- **#25 (PR #97):** each user counts once per emoji; a no-op add or remove replies
  `changed: false` with no event, and `count` is set from `users`, healing drift
  ([requests](./websocket-requests.md#add_reaction-and-remove_reaction)). Not done from
  the review: odd old reaction shapes, a leftover `{count: 0}`, the no-op lookup.
- **#27 (PR #96):** a block either way closes the direct chat to both people. Opening it
  gets `403` / `PERMISSION_DENIED` `User is blocked`, and so do `send_message`,
  `set_typing`, `mark_read` and reactions; history still reads. Groups aren't affected
  ([requests](./websocket-requests.md), [app changes](../Android/_docs/08_SERVER_CHANGES.md)).
  Not done from the code review: contact requests still miss a half-written block.
- **#38 (PR #95):** a contact request or direct chat with yourself gets `400`
  (`INVALID_USER_ID` from `join_chat`) instead of storing broken data
  ([app changes](../Android/_docs/08_SERVER_CHANGES.md)).
- **#28 (PR #94):** only the owner can remove the owner (also when the owner is only
  stored as `ownerId`); a request naming the owner removes nobody. Listing yourself no
  longer drops the group from your chat list, `removedUsers` lists only the people removed,
  and the role is re-checked in the write ([requests](./websocket-requests.md#remove_user),
  [app changes](../Android/_docs/08_SERVER_CHANGES.md)). The code review's other points
  (a race reported as `PERMISSION_DENIED`, the second write not checked, #37) were left.
- **#29 (PR #93):** the joined-users route returns the list of the code that was asked
  for, not the group's first code.
- **#91 (PR #92):** sending a deleted invite code returns `400` instead of giving the
  target user a pending invitation for it.
- **#15 (PR #90):** invite codes are 16 random bytes (32 hex characters) with nothing
  inside; the AES key that was in the repo is gone. The invite routes find the group by
  the stored code (indexed), so old codes work until they expire, and a code that isn't
  stored gets `400` on every route ([REST](./rest-api.md#groups),
  [app changes](../Android/_docs/08_SERVER_CHANGES.md)). Security review: nothing found.
  The code review found #91 (a deleted code can still be sent).
- **#65, #84, #88 (PRs #82, #86, #89):** signup, the auth middleware and the WebSocket
  connect log name the user by ID only; no log line writes an email, username or hash.
- **#83 (PR #87):** `invite_user` answers a failed chat lookup (timeout, MongoDB down) with
  the retryable `INVITE_FAILED`; only a chat that doesn't exist is `INVALID_CHAT_ID`.
  `GetChat` now returns `ErrChatNotFound` for a missing chat.
- **#39 part 1 (PR #85):** `limit=0` no longer crashes the contact lists with a `500`;
  `limit` is kept in 1..100 ([REST](./rest-api.md#contacts)). Still open on #39: the lists
  aren't cut to `limit`, which needs a cursor first.
- **#41 (PR #81):** no REST or WebSocket reply carries internal error text. A `400`'s
  `error` names each field by its JSON path and rule (`password: min=8`), and
  `invite_user` answers with specific codes ([REST](./rest-api.md#the-basics),
  [app changes](../Android/_docs/08_SERVER_CHANGES.md)). Security review: nothing found.
- **#40 part 1 (PR #80):** a WebSocket frame over 64 KiB closes with `1009`, and each
  connection gets 20 requests a second (bursts of 40, pings included). A request over that
  isn't handled and gets `RATE_LIMITED` ([protocol](./WEBSOCKET_PROTOCOL.md#stay-connected),
  [app changes](../Android/_docs/08_SERVER_CHANGES.md)). Security review: nothing found.
  Still open on #40: a per-user limit (one account can open many sockets), `set_typing`'s
  MongoDB write, mention lookups, `CheckOrigin`, and login/signup limits.
- **Render (2026-11-12):** `STATS_TOKEN` is set (the Blueprint generated it), and
  `GET chats/hub/stats` with `X-Stats-Token` returns `200`. The MongoDB password,
  `JWT_SECRET` and `STATS_TOKEN` were rotated after being exposed, so every login before
  that was signed out.
- **#76 (PR #78):** open WebSockets now
  close when their session ends. Logout closes the connections opened with that token
  (other devices stay connected); deleting the profile closes all of the user's. Both act
  at once on every server, through the Redis channel `control:sessions`. Every open
  connection is also re-checked every 30 seconds, so a ban made in MongoDB closes it within
  about a minute (52 to 58 s in the live checks), and a connection closes when its token
  expires. If a check can't be made, the pass stops and the connections stay open until
  the next one. The close frame says why: `4401` `TOKEN_REVOKED`,
  `TOKEN_EXPIRED` or `ACCOUNT_NOT_FOUND`, `4403` `ACCOUNT_BANNED` or `ACCOUNT_INACTIVE`
  ([WEBSOCKET_PROTOCOL.md](./WEBSOCKET_PROTOCOL.md#stay-connected)). A closed connection's
  read loop finishes the close handshake within 5 seconds instead of at its 60-second
  deadline. The middleware and the re-check share one check (`services.CheckSession`). The
  code review found #79; the security review found nothing.
- **#33 (PR #77):** a token stops working when its account is banned, deactivated or
  deleted after login ([REST](./rest-api.md), [app changes](../Android/_docs/08_SERVER_CHANGES.md)).
- **#72 (PR #74):** reactions work on real messages. New messages store `reactions: {}` instead of
  `null`, which MongoDB refused to create reaction fields in. A message that still has
  `null` (every message saved before the fix, or one written by an older server copy
  during a deploy) is repaired when someone first reacts to it, so there is no startup
  migration. Old messages nobody has reacted to still show `reactions: null` in history
  ([rest-api.md](./rest-api.md#messages)). The live check found #75.

## How to build, run, test

Step-by-step commands are in [getting-started.md](./getting-started.md). The one command
to remember is `node scripts/verify.mjs`.

## Test baseline

Run on 2026-11-12 on `fix/76-close-sockets-on-revoke`, on Windows, with MongoDB and Redis running and
`TEST_DB_REQUIRED=1`, through `node scripts/verify.mjs --full` after `go clean -testcache`.
The lint row is from 2026-11-07 at `90b2f73`. Compare new runs against this table by test
name.

| Command | Duration | Result | Known failures (by name) |
|---|---|---|---|
| `go build ./...` | 2s | pass | — |
| `go vet ./...` | 1s | pass | — |
| `gofmt -l cmd internal` | <1s | pass (no files) | — |
| `go test ./...` | 3s | pass, none skipped | — |
| `go test -race ./...` | 16s | pass, none skipped | — |
| `golangci-lint run ./...` (v2.14) | 11s | **27 issues**: errcheck 20, staticcheck 7 | not blocking: verify lints only new code (`--new-from-merge-base=origin/main`) |

- Tests that need MongoDB skip when it isn't running, unless `TEST_DB_REQUIRED` is set. CI
  sets it.
- `internal/realtime/hub_race_test.go` uses the hub from several goroutines at once. It only
  catches races under `go test -race`.
- The contract test (`scripts/contract-test`) isn't part of verify; it needs a running
  server. All its steps passed on 2026-11-15 (#67).

## Open decisions

None right now. What has been decided, and what each choice unblocks, is in
[decisions.md](./decisions.md): invite permissions (#30), membership in `participants`
only (#14, #17), soft-deleting accounts (#35), transactions on a replica set (#37), how
blocks are stored, and who counts as a member.

## Known issues and risks

There are 30 open issues. Where each risk sits in the code: [architecture-overview.md](./architecture-overview.md#riskiest-areas).

| Area | The problem, in short | Issues |
|---|---|---|
| Live connections | The server can leak resources or stall under load, missed live messages can't be detected, and open connections miss new chats, and a restart of a running hub doubles its loops | #20, #21, #22, #43, #49, #54, #55, #79 |
| Chat access | Some replies reveal whether a chat exists | #59 |
| Groups | Members are never asked to accept, live delivery doesn't match who is in the group, and the size limit isn't enforced | #5, #14, #16, #17, #68 |
| Invite codes | Pending members can't join by code, creating a code doesn't return it, and entries left by forged joins before #9 was fixed are still stored | #69, #103, #104 |
| Accounts | Deleting an account leaves old references, emails are case-sensitive, and usernames aren't unique | #35, #36 |
| Contacts and blocking | Your contacts list includes people you blocked, contact lists aren't cut to `limit`, and blocks written during a deploy can be missed | #39, #62, #63 |
| Online status and privacy | Online status is unreliable, and privacy settings aren't applied | #31, #32 |
| Messages and data | Writes can half-finish, and some lists grow without limit | #37, #42 |
| Limits and errors | WebSocket limits are per connection, not per user, and `set_typing` still writes to MongoDB; no limits on login and signup | #40 |
| WebSocket replies and events | Replies and events are inconsistent or leave out details, which affects the Android plan | #48, #50, #51 |

## Next task

Pick the next issue from the table above. Commit this state update as the first commit on
that branch.
