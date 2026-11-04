# ECHO: docs index

ECHO is the server for a real-time chat app. This page is the map of its docs. Find what
you need, read that page, and stop when you have your answer.

## Start here

| I want to | Read |
|---|---|
| Understand what ECHO is and what works today | [product-overview.md](./product-overview.md) |
| Run it on my computer | [getting-started.md](./getting-started.md) |
| See where the project stands, active work, test baseline | [current-state.md](./current-state.md) |
| Look up a word I don't know | [glossary.md](./glossary.md) |

## Build a client app

| I want to | Read |
|---|---|
| Call the REST API | [rest-api.md](./rest-api.md) |
| Learn how the WebSocket works, with all error codes | [WEBSOCKET_PROTOCOL.md](./WEBSOCKET_PROTOCOL.md) |
| See every request an app can send | [websocket-requests.md](./websocket-requests.md) |
| See every event the server sends | [websocket-events.md](./websocket-events.md) |
| Start from working code | [websocket-client-example.md](./websocket-client-example.md) |

## Work on the server

| I want to | Read |
|---|---|
| Learn the code layout, main flows, data model, target layers, riskiest areas | [architecture-overview.md](./architecture-overview.md) |
| Learn how live messages move between servers | [PUBSUB_ARCHITECTURE.md](./PUBSUB_ARCHITECTURE.md) |
| Read the increment log, written by `/close-increment` (turn into a link once it exists) | `status/increments.md` |
| See how the AI tooling was set up and what's deferred | [ai-harness-setup.md](./ai-harness-setup.md) |

Bugs are tracked as [GitHub issues](https://github.com/shoonya0/ECHO/issues). These docs
describe what the code does today, bugs included, and point to the issue (for example
`#23`). They don't describe fixes that haven't happened yet.

## Change notes

Short records of past changes. They explain why something changed, and each one says what
is no longer true.

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
- When a doc and the code disagree, the code wins. Fix the doc.
- Write for someone new to the project: short sentences, and explain each new word.
- Feature docs live next to the feature's code (`<folder>/_docs/`), or in
  `_docs/features/<feature>/` when the feature has no single folder.
- Size limits: index 80 lines, state 120, other docs 200. Over the limit? Split the doc.
  Run `node scripts/docs-check.mjs` after any docs change.
