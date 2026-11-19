# ECHO decisions: data writes

Up: [decisions](./README.md)

## MongoDB transactions

Decided on 2026-11-15. Multi-document writes may use transactions, so MongoDB must run as a
replica set everywhere. Atlas already does. Local `docker compose` and CI run it as a
one-node replica set (`--replSet rs0`, initiated by the compose healthcheck). Set up on
2026-11-19: CI starts MongoDB from `docker/docker-compose.yml`, and
`TestTestDBSupportsTransactions` fails on a standalone server. The server refuses to start
on one (`database.RequireTransactions`).

How they are written (2026-11-19, #37 part 2): `database.WithTransaction` runs the reads and
checks too, not only the writes, because the driver runs the function again after a write
conflict and the retry must see what the other side committed. Writes to two users go
smaller ID first (`writeInIDOrder`): in the other order, a request A→B and B→A at once each
held one user and aborted the other thousands of times until the request timed out. The
contact writes (request, accept or decline, remove, block) run this way. A transaction that
finally fails in MongoDB (commit, retries run out, context ended) is logged at error by
`WithTransaction`; the callers' own errors aren't.

Group and message writes (step 2b, 2026-11-19): creating a group, adding and removing
members, and sending a message each commit as a whole. They write the chat first, then the
users through one `UpdateMany` over `_id` (`$in`), which goes through the `_id` index in ID
order, so they meet other writes in the same order
(`TestConcurrentGroupCreatesWithSharedUsers`). Work outside the database (joining the live
hub, broadcasting) runs after the commit. A send keeps its reads outside the transaction,
since every message writes its chat and concurrent sends rerun; `lastMessageId` is set with
`$max`, so a rerun can't set an older message. Kept on 2026-11-19 although concurrent sends
to one chat now conflict and rerun (20 at once: 0.26s locally); revisit if send latency or
conflicts show on Render. Tests make one write fail with `testutil.FailCommand`
(`enableTestCommands`, local and CI only) to check the rollback. Join by code and accept or
decline an invite are next (step 2b-2): their "already a member" path repairs a missing
chats entry and then returns an error, which a transaction would roll back.

Left open (from the PR review of step 2a):
- **No backoff.** The driver reruns a conflicting transaction at once, reads included, so
  many writes to one popular user or chat repeat reads until the first commits. Only worth
  our own retry loop with jitter if it shows up in load or logs.
- **Commit vs. client timeout.** A client that disconnects during the commit can get an
  error for a write that was saved, and its retry gets "already sent". True of every
  write, not only transactions; leave unless clients report it.
- **One failpoint per server.** `testutil.FailCommand` sets MongoDB's single `failCommand`
  failpoint; only `internal/services` may use it, or packages testing in parallel
  overwrite each other's setting. Make it a lock if a second package needs it.

## One direct chat per pair (#37)

Decided on 2026-11-19. A direct chat stores `directKey`, the two user IDs smaller first,
under a unique partial index (`directKey_1`), so concurrent opens can't make two chats; the
loser returns the winner's chat. No transaction needed. Opening looks up the keyed chat
first, then the oldest by participants. A backfill runs on every start before the indexes:
it takes the key off the newer of two chats keyed alike (made before the index existed,
which would block it), then keys unkeyed chats oldest first (an older server copy can still
create some during a deploy). A pair's later duplicates stay unkeyed and are logged
(`Second direct chats of a pair`): merging their messages is a person's call.

Favorite writes are conditional on the list not holding the ID yet and check
`MatchedCount`, so of two concurrent favorites one wins and the other gets the usual
"already" error. Contact requests and answers do the same through a transaction instead
(see above), so neither side keeps a half-written change.
