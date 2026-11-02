# ECHO: docs index

Linked from CLAUDE.md. This page is a table of contents: one line per doc or feature, no
detail. Read top-down and stop as soon as you have what the task needs.

## Start here

| Need | Read |
|---|---|
| Where the project stands, active work, test baseline | [current-state.md](./current-state.md) |
| Product overview: features, performance numbers, API summary | [product-overview.md](./product-overview.md) |
| Packages, flows, data model, target layers, riskiest areas | [architecture-overview.md](./architecture-overview.md) |
| Pub/sub and WebSocket hub internals | [PUBSUB_ARCHITECTURE.md](./PUBSUB_ARCHITECTURE.md) |
| WebSocket request and event contract | [WEBSOCKET_PROTOCOL.md](./WEBSOCKET_PROTOCOL.md) |
| Increment log, written by `/close-increment` (turn into a link once it exists) | `status/increments.md` |
| How the AI harness was set up, what's deferred, playbook version | [ai-harness-setup.md](./ai-harness-setup.md) |

## Change notes

| Note | Topic |
|---|---|
| [CONSOLIDATION_SUMMARY.md](./CHANGELOG/CONSOLIDATION_SUMMARY.md) | Model consolidation |
| [GROUP_MESSAGING_README.md](./CHANGELOG/GROUP_MESSAGING_README.md) | Group messaging |
| [INDEXES_CURSOR_PAGINATION.md](./CHANGELOG/INDEXES_CURSOR_PAGINATION.md) | MongoDB indexes and cursor pagination |

## Feature registry

One row per feature that has its own docs folder. Status: ⏳ planned · 🟡 in progress ·
✅ done · ⚠️ blocked · 💤 dormant.

| Feature | Docs index | Code | Status | One line |
|---|---|---|---|---|
| *(none yet: `/feature-docs start <feature> <folder>` adds a row)* | | | | |

## Rules for these docs

- One fact, one home. Other docs link to it instead of repeating it.
- Feature docs live next to the feature's code (`<folder>/_docs/`), or in
  `_docs/features/<feature>/` when the feature has no single folder.
- Over budget (index 80, state 120, other docs 200 lines)? Split it into a folder with
  its own `README.md` index. Run `node scripts/docs-check.mjs` after any docs change.
