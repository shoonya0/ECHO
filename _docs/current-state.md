# ECHO: current state

A one-page snapshot of where the project stands. Last checked on 2026-11-09 at `f384c47`
on `main`. To learn what the product does, read
[product-overview.md](./product-overview.md). For the code, read
[architecture-overview.md](./architecture-overview.md).

## Active work

<!-- Read by the SessionStart hook (session-context.mjs): keep this section first, short,
and with a link to exactly one feature state.md. Updated by /close-increment and
/feature-docs update. -->

- **Feature:** none yet (set by `/feature-docs start`) · #34 merged (PR #64), and #65 filed
  from its code review. The production audit for old takeovers hasn't been run yet (see #34
  under "Recently finished")
- **Next step:** fix #9 (forged invite codes)
- **Updated:** 2026-11-09 at `f384c47`

## Where things stand

The core product works: accounts, profiles, contacts, direct chats, group chats, live
messages, message history, typing, read receipts, reactions and invite codes. Many of
these still have open bugs. The full table is in
[product-overview.md](./product-overview.md#what-works-today).

Not built yet: editing or deleting messages, changing chat settings, and accepting or
declining a group invite. The full list is in
[product-overview.md](./product-overview.md#not-built-yet).

## Recently finished

- **#34 (PR #64):** signup now always generates the user ID on the server. An `_id` in the
  request is ignored, so nobody can sign up with a deleted user's ID and take over their
  chats. The reply now returns the stored ID instead of `000000000000000000000000`. The
  code review found #65. **Still to do:** accounts that took over an ID before the fix
  aren't found automatically. Run this once on the production database. A reused ID's
  timestamp is older than the account's `createdAt`. If it lists any accounts, file an issue
  to clean them up:
  `db.users.find({ $expr: { $lt: [ { $toDate: "$_id" }, { $subtract: ["$createdAt", 60000] } ] } }, { _id: 1, email: 1, createdAt: 1 })`
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
- **PR #44:** added the AI tooling, the `verify` script and CI. See
  [ai-harness-setup.md](./ai-harness-setup.md).

## How to build, run, test

Step-by-step commands are in [getting-started.md](./getting-started.md). The one command
to remember is `node scripts/verify.mjs`.

## Test baseline

Run on 2026-11-09 at `dff0e12` (the code merged as `f384c47`), on Windows, with MongoDB
running and `TEST_DB_REQUIRED=1`, through `node scripts/verify.mjs --full`. The lint row is
from 2026-11-07 at `90b2f73`. Compare new runs against this table by test name.

| Command | Duration | Result | Known failures (by name) |
|---|---|---|---|
| `go build ./...` | 2s | pass | — |
| `go vet ./...` | 1s | pass | — |
| `gofmt -l cmd internal` | <1s | pass (no files) | — |
| `go test ./...` | 3s | pass, none skipped | — |
| `go test -race ./...` | 9s | pass, none skipped | — |
| `golangci-lint run ./...` (v2.14) | 11s | **27 issues**: errcheck 20, staticcheck 7 | not blocking: verify lints only new code (`--new-from-merge-base=origin/main`) |

- Tests that need MongoDB skip when it isn't running, unless `TEST_DB_REQUIRED` is set. CI
  sets it.
- `internal/realtime/hub_race_test.go` uses the hub from several goroutines at once. It only
  catches races under `go test -race`.

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

There are 40 open issues. Fix these first: #9. Where each risk sits in the
code: [architecture-overview.md](./architecture-overview.md#riskiest-areas).

| Area | The problem, in short | Issues |
|---|---|---|
| Live connections | The server can leak resources or stall under load, missed live messages can't be detected, and open connections miss new chats | #20, #21, #22, #43, #49, #54, #55 |
| Chat access | Hub stats show too much, and some replies reveal whether a chat exists | #24, #59 |
| Groups | Members are never asked to accept, and live delivery doesn't match who is in the group | #5, #14, #16, #17, #28 |
| Invite codes | Codes can be forged, and the invite endpoints lack permission checks | #9, #15, #29, #30 |
| Accounts | Bans and deletions don't take effect at once, deleting an account leaves old references, emails are case-sensitive, usernames aren't unique, and signup logs personal data | #33, #35, #36, #65 |
| Contacts and blocking | Blocks aren't enforced in chats, your contacts list includes people you blocked, and blocks written during a deploy can be missed | #27, #38, #39, #62, #63 |
| Online status and privacy | Online status is unreliable, and privacy settings aren't applied | #31, #32 |
| Messages and data | Reaction counts drift, emoji values can corrupt reactions, writes can half-finish, and some lists grow without limit | #25, #37, #42, #58 |
| Limits and errors | No rate or size limits on the WebSocket; some replies expose internal error text | #40, #41 |
| WebSocket replies and events | Replies and events are inconsistent or leave out details, which affects the Android plan, and uppercase chat IDs lose live events | #47, #48, #50, #51, #60 |

## Next task

Fix #9: forged invite codes subscribe any user to a group's live messages. Write a failing
test first.
