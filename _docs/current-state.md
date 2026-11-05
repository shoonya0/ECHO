# ECHO: current state

A one-page snapshot of where the project stands. Last checked on 2026-11-05 at `8ccec3c`
on `main`. To learn what the product does, read
[product-overview.md](./product-overview.md). For the code, read
[architecture-overview.md](./architecture-overview.md).

## Active work

<!-- Read by the SessionStart hook (session-context.mjs): keep this section first, short,
and with a link to exactly one feature state.md. Updated by /close-increment and
/feature-docs update. -->

- **Feature:** none yet (set by `/feature-docs start`) · hub data races and the cleanup deadlock
  fixed (#19), and two new hub bugs filed (#54, #55)
- **Next step:** fix #23 (membership checks in WebSocket handlers)
- **Updated:** 2026-11-05 at `8ccec3c`

## Where things stand

The core product works: accounts, profiles, contacts, direct chats, group chats, live
messages, message history, typing, read receipts, reactions and invite codes. Many of
these still have open bugs. The full table is in
[product-overview.md](./product-overview.md#what-works-today).

Not built yet: editing or deleting messages, changing chat settings, and accepting or
declining a group invite. The full list is in
[product-overview.md](./product-overview.md#not-built-yet).

## Recently finished

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

Run on 2026-11-05 at `ec2d260`, on Windows, without MongoDB or Redis running. Compare new
runs against this table by test name.

| Command | Duration | Result | Known failures (by name) |
|---|---|---|---|
| `go build ./...` | 2s | pass | — |
| `go vet ./...` | 1s | pass | — |
| `gofmt -l cmd internal` | <1s | pass (no files) | — |
| `go test ./...` | 10s | pass; skipped: `TestAddGroupMemberAuthorization`, `TestGetChatMessagesRequiresMembership`, `TestCreateGroupChatCreatorCanAddMembers`, `TestBackfillGroupOwnerRoles` (no DB) | — |
| `go test -race ./...` | 15s | pass (same skips) | — |
| `golangci-lint run ./...` (v2.14) | 2s (cached) | **28 issues**: errcheck 21, staticcheck 7 | not blocking: verify lints only new code (`--new-from-merge-base=origin/main`) |

- The four skipped tests need MongoDB. CI runs them.
- `internal/realtime/hub_race_test.go` uses the hub from several goroutines at once. It only
  catches races under `go test -race`.

## Open decisions

These need a choice before the linked issues can be fixed.

| # | Decision needed | Options | Blocking? |
|---|---|---|---|
| 1 | Where chat membership is stored | `participants` only / keep `users.chats` in sync | #14, #17 |
| 2 | How blocks are stored | one-sided `blocked` + `blockedBy` | #26 |
| 3 | What deleting an account does | soft delete + anonymize / hard delete + cleanup | #35 |
| 4 | MongoDB transactions | require a replica set (Atlas yes, local compose no) | #37 |

## Known issues and risks

There are 37 open issues. Fix these first: #23, #26, #34. Where each risk sits in the
code: [architecture-overview.md](./architecture-overview.md#riskiest-areas).

| Area | The problem, in short | Issues |
|---|---|---|
| Live connections | The server can leak resources or stall under load, missed live messages can't be detected, and open connections miss new chats | #20, #21, #22, #43, #49, #54, #55 |
| Chat access | Typing, read receipts and reactions don't check membership; hub stats show too much | #23, #24 |
| Groups | Members are never asked to accept, and live delivery doesn't match who is in the group | #5, #14, #16, #17, #28 |
| Invite codes | Codes can be forged, and the invite endpoints lack permission checks | #9, #15, #29, #30 |
| Accounts | Bans and deletions don't take effect at once; signup and delete have data problems | #33, #34, #35, #36 |
| Contacts and blocking | Blocks can be undone by the blocked person and aren't enforced in chats | #26, #27, #38, #39 |
| Online status and privacy | Online status is unreliable, and privacy settings aren't applied | #31, #32 |
| Messages and data | Reaction counts drift, writes can half-finish, and some lists grow without limit | #25, #37, #42 |
| Limits and errors | No rate or size limits on the WebSocket; some replies expose internal error text | #40, #41 |
| WebSocket replies and events | Replies and events are inconsistent or leave out details, which affects the Android plan | #47, #48, #50, #51 |

## Next task

Fix #23: write a failing test first, then check membership in every WebSocket handler that
takes a `chatId`.
