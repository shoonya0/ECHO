# ECHO: finished work, PRs #147 and later

Part of [finished work](./README.md). Newest first.

- **#32 (PR #148):** `showOnlineStatus` (`contacts`, `none`) and `showLastSeen` apply to
  contact lists and `presence` events, `allowContactBy` gates contact requests (`403`), and
  `showLastSeen` is on by default (one-time migration). Code review found `PUT profile/`
  resetting unsent settings, fixed ([decided](../decisions/accounts-and-privacy.md#how-privacy-settings-apply-32)).
- **#31 part 2 (PR #147):** REST presence is live from `online:<id>`, `status` is the
  user's choice, invisible users stay offline, `lastSeen` is saved on offline, and the hub
  reads the status from MongoDB ([decided](../decisions/accounts-and-privacy.md#where-presence-comes-from-31)).
  Code review: 10 findings, 6 fixed.
