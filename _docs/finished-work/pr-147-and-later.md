# ECHO: finished work, PRs #147 and later

Part of [finished work](./README.md). Newest first.

- **#37 part 1 (PR #150):** a pair has one direct chat: `directKey` under a unique index,
  the race loser returns the winner's chat, and a startup backfill keys older chats and
  logs duplicates ([decided](../decisions/data-writes.md#one-direct-chat-per-pair-37)). Contact list
  writes are conditional and check `MatchedCount`, so double submits get "already" errors.
  Code review: 10 findings, all fixed. Left: transactions (part 2).
- **#21 (PR #149):** each server listens to all its Redis channels over one shared
  connection and one reader; a subscribe returns once Redis confirms it, and one that isn't
  confirmed is undone ([pub/sub](../PUBSUB_ARCHITECTURE.md#redis-channels)). Code review: 9
  findings, 7 fixed, 2 documented.
- **#32 (PR #148):** `showOnlineStatus` (`contacts`, `none`) and `showLastSeen` apply to
  contact lists and `presence` events, `allowContactBy` gates contact requests (`403`), and
  `showLastSeen` is on by default (one-time migration). Code review found `PUT profile/`
  resetting unsent settings, fixed ([decided](../decisions/accounts-and-privacy.md#how-privacy-settings-apply-32)).
- **#31 part 2 (PR #147):** REST presence is live from `online:<id>`, `status` is the
  user's choice, invisible users stay offline, `lastSeen` is saved on offline, and the hub
  reads the status from MongoDB ([decided](../decisions/accounts-and-privacy.md#where-presence-comes-from-31)).
  Code review: 10 findings, 6 fixed.
