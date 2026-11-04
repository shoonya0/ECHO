# ECHO: current state

A one-page snapshot of where the project stands. Last checked on 2026-11-04 at `5645775`
on `main`. To learn what the product does, read
[product-overview.md](./product-overview.md). For the code, read
[architecture-overview.md](./architecture-overview.md).

## Active work

<!-- Read by the SessionStart hook (session-context.mjs): keep this section first, short,
and with a link to exactly one feature state.md. Updated by /close-increment and
/feature-docs update. -->

- **Feature:** none yet (set by `/feature-docs start`) · docs rewritten in plain language (#46),
  and five new bugs filed (#47 to #51)
- **Next step:** fix #19 (hub map races)
- **Updated:** 2026-11-04 at `5645775`

## Where things stand

The core product works: accounts, profiles, contacts, direct chats, group chats, live
messages, message history, typing, read receipts, reactions and invite codes. Many of
these still have open bugs. The full table is in
[product-overview.md](./product-overview.md#what-works-today).

Not built yet: editing or deleting messages, changing chat settings, and accepting or
declining a group invite. The full list is in
[product-overview.md](./product-overview.md#not-built-yet).

## Recently finished

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

Run on 2026-11-04 at `17ec08b`, on Windows, without MongoDB or Redis running. Compare new
runs against this table by test name.

| Command | Duration | Result | Known failures (by name) |
|---|---|---|---|
| `go build ./...` | 13s | pass | — |
| `go vet ./...` | 3s | pass | — |
| `gofmt -l cmd internal` | <1s | pass (no files) | — |
| `go test ./...` | 8s | pass; skipped: `TestAddGroupMemberAuthorization`, `TestGetChatMessagesRequiresMembership`, `TestCreateGroupChatCreatorCanAddMembers`, `TestBackfillGroupOwnerRoles` (no DB) | — |
| `go test -race ./...` | 17s | pass (same skips) | — |
| `golangci-lint run ./...` (v2.14) | 2s (cached) | **28 issues**: errcheck 21, staticcheck 7 | not blocking: verify lints only new code (`--new-from-merge-base=origin/main`) |

- The four skipped tests need MongoDB. They ran and passed in CI at `17ec08b`.
- The race test passes only because no test uses the hub from several places at once.
  #19 explains how to reproduce the problem.

## Open decisions

These need a choice before the linked issues can be fixed.

| # | Decision needed | Options | Blocking? |
|---|---|---|---|
| 1 | Where chat membership is stored | `participants` only / keep `users.chats` in sync | #14, #17 |
| 2 | How blocks are stored | one-sided `blocked` + `blockedBy` | #26 |
| 3 | What deleting an account does | soft delete + anonymize / hard delete + cleanup | #35 |
| 4 | MongoDB transactions | require a replica set (Atlas yes, local compose no) | #37 |

## Known issues and risks

There are 36 open issues. Fix these first: #19, #23, #26, #34. Where each risk sits in the
code: [architecture-overview.md](./architecture-overview.md#riskiest-areas).

| Area | The problem, in short | Issues |
|---|---|---|
| Live connections | The server can crash or leak resources under load, missed live messages can't be detected, and open connections miss new chats | #19, #20, #21, #22, #43, #49 |
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

Fix #19: write a failing test under `go test -race` first, then protect the hub maps with
`hub.Mutex`.
