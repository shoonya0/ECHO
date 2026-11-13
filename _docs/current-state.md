# ECHO: current state

A one-page snapshot of where the project stands. Last checked on 2026-11-12 at `d9f19ae`
on `main`. To learn what the product does, read
[product-overview.md](./product-overview.md). For the code, read
[architecture-overview.md](./architecture-overview.md).

## Active work

<!-- Read by the SessionStart hook (session-context.mjs): keep this section first, short,
and with a link to exactly one feature state.md. Updated by /close-increment and
/feature-docs update. -->

- **Feature:** none yet (set by `/feature-docs start`) · #40, part 1 on
  `fix/40-ws-frame-and-rate-limits` (verify `--full` passes, PR open): frame size and
  request rate limits ([protocol](./WEBSOCKET_PROTOCOL.md#stay-connected),
  [app changes](../Android/_docs/08_SERVER_CHANGES.md)). Code review fixes in; security review found nothing. Left
  on #40: a per-user limit (one account can open many sockets), `set_typing`'s MongoDB
  write, mention lookups, `CheckOrigin`, login/signup limits. Also to do: confirm #72 on
  Render with a message saved before PR #74
- **Next step:** CI green, then ask before merging the PR (it deploys to Render)
- **Updated:** 2026-11-13 on `fix/40-ws-frame-and-rate-limits`

## Where things stand

The core product works: accounts, profiles, contacts, direct chats, group chats, live
messages, message history, typing, read receipts, reactions and invite codes. Many of
these still have open bugs. The full table is in
[product-overview.md](./product-overview.md#what-works-today).

Not built yet: editing or deleting messages, changing chat settings, and accepting or
declining a group invite. The full list is in
[product-overview.md](./product-overview.md#not-built-yet).

## Recently finished

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
- **#24 (PR #70):** `GET chats/hub/stats` is for operators only. It needs
  an `X-Stats-Token` header that matches the new `STATS_TOKEN` setting, and it is off while
  that is unset. Every refusal is `404`. The reply no longer lists the Redis channels, which
  named every online user and active chat. `render.yaml` asks Render to generate the token.
- **#9 (PR #66):** joining a group needs an invite code that is stored on that group, not deleted
  and not expired. Before, any code that decrypted worked, and the key is public (#15). A
  connection now follows only chats that list the user in `participants`, so `chats`
  entries planted by forged codes before the fix give no live messages. Codes that can't
  be decoded return 400 on every invite route. Joining twice returns 409 instead of 500,
  and retrying a join that failed halfway finishes it. The code is carried over from
  PR #12, which branched before the docs rewrite. The code review found #67, #68 and #69.
- **#34 (PR #64):** signup now always generates the user ID on the server. An `_id` in the
  request is ignored, so nobody can sign up with a deleted user's ID and take over their
  chats. The reply now returns the stored ID instead of `000000000000000000000000`. The
  code review found #65. The audit
  [scripts/audits/34-reused-signup-ids.js](../scripts/audits/34-reused-signup-ids.js)
  (PR #73) ran on production on 2026-11-12: 1 user checked, 0 flagged, 0 it couldn't
  judge, so no account took over an ID.
- **#26 (PR #61):** blocks are one-sided. They are stored in `contactInfo.blocked` (shown to the
  blocker) and `contactInfo.blockedBy` (never sent to clients), so only the blocker can lift
  a block and the blocked person can't see it. Blocking twice, or unblocking someone you
  didn't block, now returns 409. Old blocks become mutual blocks when the server starts. A
  retried block or unblock repairs a half-finished one. The code review found #62 and #63.
- **#23 (PR #57):** typing, read receipts and reactions now refuse non-members with
  `PERMISSION_DENIED`, checked in the service layer. `leave_chat` is refused unless the
  connection is in the chat's room, and `mark_read` skips IDs from other chats.
- **#19:** the hub now uses its shared maps safely, so concurrent connections can no longer
  crash the server that way, and inactive-client cleanup can no longer deadlock.
- **PR #46:** rewrote these docs in plain language and corrected what didn't match the
  code. Checking the docs against the code found five new bugs: #47 to #51.
- **#18 (PR #45):** group creators now get the `owner` role, so adding and removing group
  members works again. Groups created before the fix are repaired when the server starts.
  CI now runs the database tests against a real MongoDB.

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

There are 41 open issues. Where each risk sits in the code: [architecture-overview.md](./architecture-overview.md#riskiest-areas).

| Area | The problem, in short | Issues |
|---|---|---|
| Live connections | The server can leak resources or stall under load, missed live messages can't be detected, and open connections miss new chats, and a restart of a running hub doubles its loops | #20, #21, #22, #43, #49, #54, #55, #79 |
| Chat access | Some replies reveal whether a chat exists | #59 |
| Groups | Members are never asked to accept, live delivery doesn't match who is in the group, and the size limit isn't enforced | #5, #14, #16, #17, #28, #68 |
| Invite codes | Anyone can decrypt a code, the invite endpoints lack permission checks, and entries left by forged joins before #9 was fixed are still stored | #15, #29, #30, #69 |
| Accounts | Deleting an account leaves old references, emails are case-sensitive, usernames aren't unique, and signup logs personal data | #35, #36, #65 |
| Contacts and blocking | Blocks aren't enforced in chats, your contacts list includes people you blocked, and blocks written during a deploy can be missed | #27, #38, #39, #62, #63 |
| Online status and privacy | Online status is unreliable, and privacy settings aren't applied | #31, #32 |
| Messages and data | Reaction counts drift, history doesn't show who reacted by name, writes can half-finish, and some lists grow without limit | #25, #37, #42, #75 |
| Limits and errors | No rate or size limits on the WebSocket; some replies expose internal error text | #40, #41 |
| WebSocket replies and events | Replies and events are inconsistent or leave out details, which affects the Android plan, and uppercase chat IDs lose live events | #47, #48, #50, #51, #60 |

## Next task

Pick the next issue from the table above. Commit this state update as the first commit on
that branch.
