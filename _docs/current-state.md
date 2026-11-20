# ECHO: current state

A one-page snapshot of where the project stands. Last checked on 2026-11-19 at `06de366`
on `main`. To learn what the product does, read
[product-overview.md](./product-overview.md). For the code, read
[architecture-overview.md](./architecture-overview.md).

## Active work

<!-- Read by the SessionStart hook (session-context.mjs): keep this section first, short,
and with a link to exactly one feature state.md. Updated by /close-increment and
/feature-docs update. -->

- **Feature:** none · #42 last step on `fix/42-drop-readby` (PR open, closes #42): messages
  no longer store `readBy` (mark_read writes only the chat), and a one-time startup job
  unsets it on older messages. ECHO-app #9 reads `readSeqs` only (checked 2026-11-20).
  #5 step 2 waits for the Android invite screen ([group-invites](./features/group-invites/state.md))
- **#42 readBy on Render (not checked yet):** the startup log shows `Removed readBy from
  messages` once, `migrations` holds `message-readby-42`, and read receipts still show in the app
- **#168 on Render (not checked yet):** connecting still gets the welcome first, and
  messages arrive in every chat
- **#42 step 4 on Render (not checked yet):** the startup log shows `Removed read state of
  users who aren't participants` once (or nothing), and `migrations` holds
  `stale-read-state-42`
- **#42 step 3 on Render (not checked yet):** mark a message read in the app; the chat in
  Atlas gets `readSeqs.<userId>`, and the next history reply lists it
- **#42 step 1 on Render (not checked yet):** the startup log shows `Indexes ensured` with
  `chatInvitations.token_1`; the 21st unexpired invite code of a group gets `409`
- **#43 on Render (checked 2026-11-19):** `chatId_1_seq_1` built (15:14Z), no numbering
  failure, `message-seqs-43` recorded at 15:36Z. Left: no message lacks `seq` (count in
  Atlas; deleted chats' messages may), and the app shows `seq` in history
- **#36 on Render (not checked yet):** the startup log shows any `Renamed a username another
  account already had`, then `Indexes ensured` with `username_ci`; after a rename in the
  app, old messages show the new name, and so do typing and join events within seconds
- **#37 part 1 on Render (not checked yet):** the startup log shows `Backfilled direct chat
  keys` once (not on the next start), any `Second direct chats of a pair` (chat IDs to merge
  by hand), and `Indexes ensured` with `directKey_1`
- **#37 step 2 on Render (not checked yet):** the server starts (no `MongoDB can't run
  transactions`); contacts, groups (create, add, remove, join, accept, decline) and sends work;
  step 3: marking messages read and reacting still work
- **#21 on Render (not checked yet):** Redis `CLIENT LIST TYPE pubsub` shows one connection
  per running server
- **#31 on Render (not checked yet):** in a shared chat, A sees B's online and offline, and
  B's contacts list shows `isOnline` only while B is connected
- **#32 on Render (partly checked):** `migrations` holds `show-last-seen-default-32`
  (05:29Z, 2026-11-19), so the one-time switch ran. Left: with `allowContactBy: none`, a contact request gets `403`
- **Not filed, from #31:** a user held only by a server that crashes gets no offline (its
  entry expires in 2 min); two servers connecting and disconnecting one user within
  milliseconds can order online and offline wrongly; during a deploy, old and new servers
  miss each other's presence events
- **Proposed from the #35 review, not filed:** a password check before deleting an
  account; a delete rate limit plus an index on `messages.senderId`; events to members
  when someone accepts, declines, leaves or is deleted
- **#35 on Render (not checked yet):** delete a test account and check that its contacts,
  groups and messages are cleaned (`Deleted user`)
- **#5 on Render (not checked yet):** startup log shows `Accepted legacy pending group
  members` once, and `migrations` holds `accept-legacy-pending-5`
- **#36 on Render (not checked yet):** startup log shows `Indexes ensured` with `email_ci`,
  and any `Normalized emails` count or conflict warning (user IDs to merge by hand)
- **#40 on Render (not checked yet):** a login loop with a forged `True-Client-IP` and
  `X-Forwarded-For` must still get `429`, and two real clients mustn't share a count. If
  Render's edge doesn't set `True-Client-IP`, unset `CLIENT_IP_HEADER` and note it on #40.
  The user runs this check (it sends traffic to production). Details:
  [decided](./decisions/accounts-and-privacy.md#how-login-and-signup-limits-find-the-client-40)
- **#72 on Render (paused 2026-11-15):** `/health` answers `200`, but no chat with a message
  saved before PR #74 (`reactions: null`) is left to react to. Continue later, for example
  by setting `reactions: null` on a test message in Atlas first
- **#40 WebSocket on Render (not checked yet):** a handshake with `Origin:
  https://evil.example.com` gets `403`; a 6th connection of one account gets `429`; the
  Android app still connects (it sends no `Origin`). Every #40 item is merged: proposed to
  close #40. The connection cap is per server, not shared through Redis.
  Proposed, not filed: hide which emails have accounts (signup `409`, login timing). Split
  `websocket-requests.md` and `websocket-events.md` (~199 lines) before adding to them
- **#69 on Render (not checked yet):** the startup log shows `Removed chat entries of users
  who aren't participants` once (or nothing), and not on the next start
- **#22 on Render (not checked yet):** connecting gets the welcome, and messages and
  presence arrive; two devices of one account connect and disconnect without either
  losing messages
- **From #22:** filed as #168
- **Next step:** merge the `readBy` PR (closes #42). Then #5 step 2 once the app's invite
  screen exists; the app also doesn't send `mark_read` yet (ECHO-app side).
  The "Left open" review items of #37 stay in [data-writes](./decisions/data-writes.md#mongodb-transactions). Direct-chat gating and live status changes are decided against for now
  ([decided](./decisions/accounts-and-privacy.md#how-privacy-settings-apply-32)). The Render
  checks above are the user's
- **Updated:** 2026-11-20 on `fix/42-drop-readby` from `dfd2473`. Commits on `main` must keep the author `shoonya0`

## Where things stand

The core product works: accounts, profiles, contacts, direct chats, group chats, live
messages, message history, typing, read receipts, reactions and invite codes. Many of
these still have open bugs. The full table is in
[product-overview.md](./product-overview.md#what-works-today).

Not built yet: editing or deleting messages, and changing chat settings. The full list is in
[product-overview.md](./product-overview.md#not-built-yet).

## Recently finished

- **#42 readBy (`fix/42-drop-readby`):** messages don't store `readBy`; a one-time job
  (`message-readby-42`) removed it from older ones. Every #42 item is done.
- **#168 (PR #169):** the welcome is always first, and a chat gained while
  connecting is joined (the chats are read again once the user channel is subscribed).
- **#42 step 4 (PR #167):** leavers' `readReceipts`/`readSeqs` entries
  are cleared on remove, decline and delete, plus a one-time cleanup (`stale-read-state-42`).
- **#42 step 3 (PR #166):** each member's highest read `seq` is kept on the
  chat (`readSeqs`) and sent in history and `delivery`; `readBy` stays until the app switches.
- **#42 step 2 (PR #165):** a user's `chatInvitations` keeps the newest
  100 (`$push` with `$slice`); one more drops the oldest rather than being refused, so
  nobody can fill a user's list to block invites. `GET groups/pending` is unaffected.
- **#42 step 1 (PR #164):** a group holds at most 20 invite codes that haven't expired,
  deleted ones included (`409`); creating one removes expired codes and users' invitations
  for them (`chatInvitations.token_1`), in one transaction. Review: 8 findings, 3 fixed.
- **#43 (PR #163):** each message has a per-chat `seq` (send transaction, gap-free); live
  events and the `send_message` reply carry it; `?afterSeq=` catches up; old history keeps
  the lowest numbers ([decided](./decisions/data-writes.md#messages-are-numbered-per-chat-43)).
- **#36 live names (PR #162):** a name or avatar change, or a delete, clears the user's
  cached name on every server (`control:profiles`); join and leave read through that cache.
- **#36 current names (PR #161):** message history and chat replies read names and avatars
  when built; new chats and WebSocket connect copy them from the DB, not the JWT
  ([decided](./decisions/accounts-and-privacy.md#names-are-read-when-shown-36)).
- **#36 usernames (PR #160):** 3 to 30 letters, digits, `_` or `.`, unique ignoring case
  (`username_ci`); `409` on signup and `PUT profile/`; names shared before are renamed at
  startup ([decided](./decisions/accounts-and-privacy.md#unique-usernames-36)).
- **#40 per-account limit (PR #159):** one account holds up to 5 WebSocket connections per
  server; more get `429`, or `1008` when two connect at once ([connect](./WEBSOCKET_PROTOCOL.md#connect)).
- **#40 `CheckOrigin` (PR #158):** a WebSocket upgrade with an `Origin` other than the server's
  host or `WS_ALLOWED_ORIGINS` gets `403` ([connect](./WEBSOCKET_PROTOCOL.md#connect)).
- **#40 mentions (PR #157):** a message keeps only mentions of chat participants, each once,
  read in two queries instead of one per mention ([request rule](./websocket-requests.md#send_message)).
- **#110 (PR #156):** a group's `participantCount` is computed from `participants` when
  read, not stored; the `$inc` writes and the startup backfill are gone
  ([decided](./decisions/chats-and-groups.md#how-a-groups-participant-count-is-kept-110)).
- **#37 step 3 (PR #155):** read receipts and reactions run on the request's context, and a
  failed chat read receipt answers `READ_FAILED` without telling the chat
  ([decided](./decisions/data-writes.md#mongodb-transactions)).
- **#37 part 2 (PRs #151 to #154):** MongoDB runs as a one-node replica set in compose and
  CI, and the server won't start on a standalone one. Contact, group, join, invite and
  message writes each commit as a whole (`database.WithTransaction`, chat first, users in
  ID order); a request A→B and B→A at once no longer leaves both pending. Rollback tests
  use `testutil.FailCommand` ([decided](./decisions/data-writes.md#mongodb-transactions)).
  Four code reviews; open items are in that decision. Step 3 followed in PR #155.

Older work: [finished-work/](./finished-work/README.md).

## How to build, run, test

Step-by-step commands are in [getting-started.md](./getting-started.md). The one command
to remember is `node scripts/verify.mjs`.

## Test baseline

Run on 2026-11-19 on `main` at `03a04d2`, on Windows, with MongoDB and Redis running and
`TEST_DB_REQUIRED=1`, through `node scripts/verify.mjs --full` after `go clean -testcache`.
The lint row is from 2026-11-07 at `90b2f73`. Compare new runs against this table by test
name.

| Command | Duration | Result | Known failures (by name) |
|---|---|---|---|
| `go build ./...` | 2s | pass | — |
| `go vet ./...` | <1s | pass | — |
| `gofmt -l cmd internal` | <1s | pass (no files) | — |
| `go test ./...` | 18s | pass, none skipped | — |
| `go test -race ./...` | 40s | pass, none skipped | — |
| `golangci-lint run ./...` (v2.14) | 11s | **27 issues**: errcheck 20, staticcheck 7 | not blocking: verify lints only new code (`--new-from-merge-base=origin/main`) |

- Tests that need MongoDB skip when it isn't running, unless `TEST_DB_REQUIRED` is set. CI
  sets it. MongoDB must be the compose replica set with `enableTestCommands`: the rollback
  tests (`testutil.FailCommand`) skip without it.
- `internal/realtime/hub_race_test.go` uses the hub from several goroutines at once. It only
  catches races under `go test -race`.
- The contract test (`scripts/contract-test`) isn't part of verify; it needs a running
  server. All its steps passed on 2026-11-15 (#67).

## Open decisions

None right now. What has been decided, and what each choice unblocks, is in
[decisions.md](./decisions/): invite permissions (#30), the message length limit (#51), accepting a group invite (#103, #5), membership in `participants`
only (#14, #17), deleting accounts (#35), transactions on a replica set (#37), how
blocks are stored, and who counts as a member.

## Known issues and risks

There are 2 open issues. Where each risk sits in the code: [architecture-overview.md](./architecture-overview.md#riskiest-areas).

| Area | The problem, in short | Issues |
|---|---|---|
| Groups | Pending members still pass membership checks (#5 step 2) | #5 |
| Online status and privacy | A status or privacy change reaches live events only at the next connect (decided) | none |
| Messages and data | None left from #42 once the `readBy` PR merges | #42 |
| Limits and errors | The per-account WebSocket cap is per server, not across servers | #40 |

## Next task

Pick the next issue from the table above. Commit this state update as the first commit on
that branch.
