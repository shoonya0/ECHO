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
contact writes (request, accept or decline, remove, block) run this way; the group and
message writes are next. A transaction that finally fails in MongoDB (commit, retries run
out, context ended) is logged at error by `WithTransaction`; the callers' own errors aren't.

Left for step 2b (from the PR review of step 2a):
- **Order for more than two users.** A group write touches a chat and N users;
  `writeInIDOrder` takes two. Write the chat first, then the users sorted by ID, through
  one helper, or two group operations can livelock as A→B and B→A did.
- **No backoff.** The driver reruns a conflicting transaction at once, reads included, so
  many writes to one popular user or chat repeat reads until the first commits. Only worth
  our own retry loop with jitter if it shows up in load or logs.
- **Commit vs. client timeout.** A client that disconnects during the commit can get an
  error for a write that was saved, and its retry gets "already sent". True of every
  write, not only transactions; leave unless clients report it.

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
