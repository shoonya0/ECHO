# ECHO: current state

A one-page snapshot of where the project stands. Last checked on 2026-11-14 at `e081f99`
on `main`. To learn what the product does, read
[product-overview.md](./product-overview.md). For the code, read
[architecture-overview.md](./architecture-overview.md).

## Active work

<!-- Read by the SessionStart hook (session-context.mjs): keep this section first, short,
and with a link to exactly one feature state.md. Updated by /close-increment and
/feature-docs update. -->

- **Feature:** none yet · #29 on `fix/29-joined-users-by-invite`: the joined-users route
  returns the list of the invite that was asked for. Still to do: confirm #72 on Render
  with a message saved before PR #74
- **Next step:** verify, push, open the PR, then ask before merging (it deploys to Render)
- **Updated:** 2026-11-14 on `fix/29-joined-users-by-invite`

## Where things stand

The core product works: accounts, profiles, contacts, direct chats, group chats, live
messages, message history, typing, read receipts, reactions and invite codes. Many of
these still have open bugs. The full table is in
[product-overview.md](./product-overview.md#what-works-today).

Not built yet: editing or deleting messages, changing chat settings, and accepting or
declining a group invite. The full list is in
[product-overview.md](./product-overview.md#not-built-yet).

## Recently finished

- **#91 (PR #92):** sending a deleted invite code returns `400` instead of giving the
  target user a pending invitation for it.
- **#15 (PR #90):** invite codes are 16 random bytes (32 hex characters) with nothing
  inside; the AES key that was in the repo is gone. The invite routes find the group by
  the stored code (indexed), so old codes work until they expire, and a code that isn't
  stored gets `400` on every route ([REST](./rest-api.md#groups),
  [app changes](../Android/_docs/08_SERVER_CHANGES.md)). Security review: nothing found.
  The code review found #91 (a deleted code can still be sent).
- **#88 (PR #89):** the WebSocket connect log names the user by ID only. With #65 and
  #84, no log line writes an email or username any more.
- **#83 (PR #87):** `invite_user` answers a failed chat lookup (timeout, MongoDB down) with
  the retryable `INVITE_FAILED`; only a chat that doesn't exist is `INVALID_CHAT_ID`.
  `GetChat` now returns `ErrChatNotFound` for a missing chat.
- **#84 (PR #86):** the auth middleware logs the user's ID only, not the email and
  username it wrote at Debug on every request. The WebSocket connect log still holds the
  username at Info (#88).
- **#39 part 1 (PR #85):** `limit=0` no longer crashes the contact lists with a `500`;
  `limit` is kept in 1..100 ([REST](./rest-api.md#contacts)). Still open on #39: the lists
  aren't cut to `limit`, which needs a cursor first.
- **#65 (PR #82):** signup logs the new user's ID only. Before, it logged the whole user:
  the email on the server, and also the password hash under the text formatter. Checking
  the other log lines found #84 (email in a Debug line); the #41 review found #83.
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
- **#33 (PR #77):** a token stops working when its
  account is banned, deactivated or deleted after login. Before, the middleware trusted
  the account status copied into the 7-day token. Every request now reads the account
  (`services.CheckAccountAccess`), cached in Redis for 30 seconds, and refuses it with
  `403` `ACCOUNT_BANNED` or `ACCOUNT_INACTIVE` (as Login does) or `401`
  `ACCOUNT_NOT_FOUND`. Deleting a profile caches the account as deleted, so it applies at
  once, even for a request that was mid-check; a ban made in MongoDB applies within 30
  seconds. A stored `accountStatus: null` counts as inactive, as in Login. If Redis can't
  answer the logout list, the request gets `503` `AUTH_UNAVAILABLE` instead of going
  through; a cache failure reads MongoDB instead. Tokens without a `jti` are refused.
  `401` replies no longer include the parser's error text. The `else if` chain that
  skipped the ban check is gone. CI now runs Redis for the cache tests. What the Android
  app must change is in
  [08_SERVER_CHANGES.md](../Android/_docs/08_SERVER_CHANGES.md).
- **#72 (PR #74):** reactions work on real messages. New messages store `reactions: {}` instead of
  `null`, which MongoDB refused to create reaction fields in. A message that still has
  `null` (every message saved before the fix, or one written by an older server copy
  during a deploy) is repaired when someone first reacts to it, so there is no startup
  migration. Old messages nobody has reacted to still show `reactions: null` in history
  ([rest-api.md](./rest-api.md#messages)). The live check found #75.
- **#58 (PR #71):** `add_reaction` and `remove_reaction` refuse an emoji that has `.`,
  `$`, a NUL character or invalid UTF-8, isn't made of emoji characters, or is over 64
  bytes, with `INVALID_REQUEST`, before anything is written. Before, a member could send
  `ok.details` and wipe that part of a reaction for everyone. Every standard emoji passes,
  including joined ones such as 👩‍💻; the first 10-byte limit refused 1,615 of the 3,963.
  A live check found a separate bug, filed as #72: every message is saved with
  `reactions: null`, so adding a reaction to a real message always fails
  (`REACTION_FAILED`). The local database has no stored reactions at all, so nothing there
  was damaged. The code review moved the #34 audit script to PR #73.

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
- The contract test (`scripts/contract-test`) isn't part of verify. Its `chat messages` step
  fails on every build, because it still sends `offset` (#67).

## Open decisions

These need a choice before the linked issues can be fixed.

| # | Decision needed | Options | Blocking? |
|---|---|---|---|
| 1 | Where chat membership is stored | `participants` only / keep `users.chats` in sync | #14, #17 |
| 3 | What deleting an account does | soft delete + anonymize / hard delete + cleanup | #35 |
| 4 | MongoDB transactions | require a replica set (Atlas yes, local compose no) | #37 |

### Decided: how blocks are stored

Decided on 2026-11-08 (#26). A block is stored on both people, with direction:
`contactInfo.blocked` on the blocker and `contactInfo.blockedBy` on the blocked person.
`blockedBy` is never sent to clients. Old `blockedChats` data has no direction, so the
startup migration (`MigrateLegacyBlocks`) turns each pair into a mutual block and removes
the old field. Nobody is unblocked without choosing to be.

### Decided: who counts as a chat member

Decided on 2026-11-07.

| Participant | Counts as a member? | In the code today |
|---|---|---|
| Pending | No | **Not enforced yet:** pending members still pass every membership check |
| Blocked | Yes | Already true. `send_message` still refuses them, as a separate check |
| Removed | No | Already true: removal deletes the participant entry |

Enforcing the pending rule has to wait for #5. Everyone added to a group is `pending`,
except the creator and people who join by invite code, and nobody can accept yet, so
enforcing it now would lock those members out. The order is: build accept and decline,
backfill existing members, then enforce it in `chatMemberFilter`. Direct-chat participants
have no status, so the check must allow for that.

## Known issues and risks

There are 38 open issues. Where each risk sits in the code: [architecture-overview.md](./architecture-overview.md#riskiest-areas).

| Area | The problem, in short | Issues |
|---|---|---|
| Live connections | The server can leak resources or stall under load, missed live messages can't be detected, and open connections miss new chats, and a restart of a running hub doubles its loops | #20, #21, #22, #43, #49, #54, #55, #79 |
| Chat access | Some replies reveal whether a chat exists | #59 |
| Groups | Members are never asked to accept, live delivery doesn't match who is in the group, and the size limit isn't enforced | #5, #14, #16, #17, #28, #68 |
| Invite codes | The invite endpoints lack permission checks, and entries left by forged joins before #9 was fixed are still stored | #30, #69 |
| Accounts | Deleting an account leaves old references, emails are case-sensitive, and usernames aren't unique | #35, #36 |
| Contacts and blocking | Blocks aren't enforced in chats, your contacts list includes people you blocked, and blocks written during a deploy can be missed | #27, #38, #39, #62, #63 |
| Online status and privacy | Online status is unreliable, and privacy settings aren't applied | #31, #32 |
| Messages and data | Reaction counts drift, history doesn't show who reacted by name, writes can half-finish, and some lists grow without limit | #25, #37, #42, #75 |
| Limits and errors | WebSocket limits are per connection, not per user, and `set_typing` still writes to MongoDB; no limits on login and signup | #40 |
| WebSocket replies and events | Replies and events are inconsistent or leave out details, which affects the Android plan, and uppercase chat IDs lose live events | #47, #48, #50, #51, #60 |

## Next task

Pick the next issue from the table above. Commit this state update as the first commit on
that branch.
