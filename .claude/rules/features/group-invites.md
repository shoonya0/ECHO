---
paths:
  - internal/services/group_invite*.go
  - internal/controller/group_invite*.go
---
# Feature: group-invites

Docs for this feature live in `_docs/features/group-invites/`.

- Before changing code here, read `_docs/features/group-invites/state.md` (where work
  stopped) and `tasks.md` (what's next), then follow `_docs/README.md` only as far as the
  task needs.
- After any piece of work here, save the state: `/feature-docs update`. Link facts; don't copy them.
- Until increment 2, pending members still pass membership checks; don't enforce
  `requestStatus` piecemeal (decisions.md, "Who counts as a chat member").
