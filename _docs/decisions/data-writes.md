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
(`enableTestCommands`, local and CI only) to check the rollback. Joining by code and accepting or
declining an invite (step 2b-2) do the same. Their "already a member" path repairs a
missing chats entry and answers 409; an error inside the transaction would roll that repair
back, so the transaction returns what it did (`joinOutcome`) and the 409 and the live-hub
join come after the commit; a 409 always follows the chat again, as a commit that succeeded
but answered with an error never got to. A decline whose user cleanup fails now fails as a whole.

Step 3 (2026-11-19): read receipts (`MarkMessagesAsRead`) and reactions take the request's
context instead of `context.Background()`. The chat's read receipt (`updateChatReadReceipts`)
returns its error, which it used to discard. A failed one answers `READ_FAILED` and the chat
isn't told; the messages' `readBy` may already be saved, and marking them again is safe. Each
write repeats safely, so they run without a transaction.

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
- **Join and accept read the invite codes twice** (from the step 2b-2 review). One extra read
  per transaction; pass the codes in from the first read if join latency shows up.
- **`joinOutcome` is join-only.** If a second write needs "commit, then answer an error",
  make it a generic result of `WithTransaction` instead of another outcome type.

## Messages are numbered per chat (#43)

Decided on 2026-11-19. Live delivery is Redis pub/sub, at most once, so a client needs a
way to see a gap. Each message gets `seq`, taken from the chat's `stats.lastSeq` with
`$inc` inside the send transaction: sends to one chat write the same document, so they
commit one after another and an aborted send takes its number back. Message IDs were
rejected as the cursor: two servers can make IDs in the same second out of order, so a
client could pass one without noticing. `GET messages/:chatID/messages?afterSeq=` reads
`chatId_1_seq_1` (unique, partial on `seq` being set). A backfill on every start numbers
messages without a `seq` (from before #43, or saved by an older copy during a deploy)
oldest first after the chat's newest, in 500-message transactions; finding them reads
every message, which is fine at this size. Left: no replay buffer (Redis Streams), the
client catches up over REST.

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
