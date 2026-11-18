# ECHO: current state

A one-page snapshot of where the project stands. Last checked on 2026-11-18 at `417cd55`
on `main`. To learn what the product does, read
[product-overview.md](./product-overview.md). For the code, read
[architecture-overview.md](./architecture-overview.md).

## Active work

<!-- Read by the SessionStart hook (session-context.mjs): keep this section first, short,
and with a link to exactly one feature state.md. Updated by /close-increment and
/feature-docs update. -->

- **Feature:** none · nothing in progress. `main` at `417cd55`: #35's account deletion
  (PR #142) is merged, so Render deploys it. #5 step 2 (enforce pending) waits for the
  Android invite screen ([group-invites](./features/group-invites/state.md))
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
  [decided](./decisions.md#how-login-and-signup-limits-find-the-client-40)
- **#72 on Render (paused 2026-11-15):** `/health` answers `200`, but no chat with a message
  saved before PR #74 (`reactions: null`) is left to react to. Continue later, for example
  by setting `reactions: null` on a test message in Atlas first
- **Still open on #40:** a per-account WebSocket limit, mention lookups, `CheckOrigin`.
  Proposed, not filed: hide which emails have accounts (signup `409`, login timing). Split
  `websocket-requests.md` and `websocket-events.md` (~199 lines) before adding to them
- **Next step:** pick from the open issues (`gh issue list`); the Render checks above are
  the user's
- **Updated:** 2026-11-18 on `main` at `417cd55` (not committed yet: rides on the next branch)

## Where things stand

The core product works: accounts, profiles, contacts, direct chats, group chats, live
messages, message history, typing, read receipts, reactions and invite codes. Many of
these still have open bugs. The full table is in
[product-overview.md](./product-overview.md#what-works-today).

Not built yet: editing or deleting messages, and changing chat settings. The full list is in
[product-overview.md](./product-overview.md#not-built-yet).

## Recently finished

- **#35, account deletion (PR #142):** deleting an account closes its sockets, locks its
  token, pulls the ID from every contact and block list, takes it out of groups and
  channels (an owner hands on: another owner, else the earliest admin, else the earliest
  accepted member; with nobody, the group's codes are deleted), shows its messages and
  direct-chat entry as "Deleted user", then deletes the document
  ([decided](./decisions.md#what-deleting-an-account-does),
  [app changes](../Android/_docs/08_SERVER_CHANGES/auth-and-connection.md)). Review: 6 of
  10 fixed; reactions and `readBy` keep the bare ID.
- **#5 step 1 + #17 (PR #141):** invitees accept or decline (`groups/:groupID/accept`,
  `decline`, `GET groups/pending`) and get live messages once they accept; a startup
  backfill accepted members left pending before, by a cutoff stored in `migrations`
  ([REST](./rest-api/groups.md#groups), [decided](./decisions.md#where-a-pending-group-invite-is-stored-5),
  [app changes](../Android/_docs/08_SERVER_CHANGES/groups-and-invites.md)). Left: enforcing
  pending (step 2), a group event on accept and decline.
- **#36, emails (PR #140):** emails are stored trimmed and lower case; signup and login
  match them ignoring case (`email_ci`), so legacy capitals work before the startup
  `NormalizeEmails` lowers them; `PUT profile/` no longer changes `email`
  ([decided](./decisions.md#changing-the-email-36), [REST](./rest-api/users.md#accounts),
  [app changes](../Android/_docs/08_SERVER_CHANGES/auth-and-connection.md)). Left on #36:
  unique usernames, stale display copies.
- **#40, login and signup (PR #139):** login attempts are counted per IP + account (10 in
  15 min) and per account (100 an hour), before bcrypt, by account ID, and cleared on
  success; 50 logins per IP in 10 min, 10 signups per IP an hour. Over that: `429`
  `RATE_LIMITED` with `Retry-After` ([REST](./rest-api/users.md#accounts),
  [decided](./decisions.md#how-login-and-signup-limits-find-the-client-40),
  [app changes](../Android/_docs/08_SERVER_CHANGES/auth-and-connection.md)). The client IP
  comes from `CLIENT_IP_HEADER`; `X-Forwarded-For` is no longer trusted. Code and security
  reviews fixed; left: case-sensitive emails (#36), ~2x at a window's edge.
- **#79 (PR #138):** `Hub.Restart` and the WebSocket controller's cancelled-context check
  are removed: nothing cancels the hub in production, and the path started duplicate loops
  and raced on `hub.Ctx`. Each background loop gets its context from `startLoops`.
- **#50 (PR #137):** `message` events carry `data.sender` and `data.mentions` from the
  saved message; `join` and `leave` carry the username and avatar the connection loaded
  at connect ([events](./websocket-events.md)). A rename shows on the next connect.
- **#48 (PR #136):** `join_chat` with `chatId` follows any chat you are in (block rule
  included), `senderId` still opens a direct chat, and both answer with a `response`
  holding `chatId`. A repeated join isn't announced again
  ([requests](./websocket-requests.md#join_chat)).
- **#132 (PR #135):** a new direct chat is kept only when both users got their `chats`
  entry (else `404` / `INVALID_USER_ID`, chat and entries removed), and opening an existing
  one writes back a missing entry ([REST](./rest-api/chats.md#chats)).
- **#131 (PR #134):** `GET users/suggestions` takes `page` 1 to 10000 (clamped, shared
  `services.MaxPage` with the contact lists), so a huge page no longer overflows the skip
  into a `500` ([REST](./rest-api/users.md#finding-people)).
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
Older work, from #106 (PR #115) back: [finished-work.md](./finished-work.md).

## How to build, run, test

Step-by-step commands are in [getting-started.md](./getting-started.md). The one command
to remember is `node scripts/verify.mjs`.

## Test baseline

Run on 2026-11-18 on `main` at `417cd55`, on Windows, with MongoDB and Redis running and
`TEST_DB_REQUIRED=1`, through `node scripts/verify.mjs --full` after `go clean -testcache`.
The lint row is from 2026-11-07 at `90b2f73`. Compare new runs against this table by test
name.

| Command | Duration | Result | Known failures (by name) |
|---|---|---|---|
| `go build ./...` | 2s | pass | — |
| `go vet ./...` | <1s | pass | — |
| `gofmt -l cmd internal` | <1s | pass (no files) | — |
| `go test ./...` | 8s | pass, none skipped | — |
| `go test -race ./...` | 35s | pass, none skipped | — |
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
only (#14, #17), deleting accounts (#35), transactions on a replica set (#37), how
blocks are stored, and who counts as a member.

## Known issues and risks

There are 13 open issues. Where each risk sits in the code: [architecture-overview.md](./architecture-overview.md#riskiest-areas).

| Area | The problem, in short | Issues |
|---|---|---|
| Live connections | The server can leak resources or stall under load, missed live messages can't be detected, and open connections miss new chats | #21, #22, #43, #49, #117 |
| Groups | Pending members still pass membership checks (#5 step 2), and the participant count is maintained by hand | #5, #110 |
| Invite codes | Entries left by forged joins before #9 was fixed are still stored | #69 |
| Accounts | Usernames aren't unique, and display copies go stale | #36 |
| Contacts and blocking | Contact lists aren't cut to `limit`, and blocks written during a deploy can be missed | #39, #63 |
| Online status and privacy | Online status is unreliable, and privacy settings aren't applied | #31, #32 |
| Messages and data | Writes can half-finish, and some lists grow without limit | #37, #42 |
| Limits and errors | WebSocket limits are per connection, not per user; mention lookups are one read each; `CheckOrigin` allows any origin | #40 |

## Next task

Pick the next issue from the table above. Commit this state update as the first commit on
that branch.
