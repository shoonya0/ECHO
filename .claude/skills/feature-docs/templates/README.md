---
kind: feature
scope: [<folder, file or glob>, …]
status: active
---
# <Feature>: docs index

<One or two sentences: what this feature does for the user.>
Up: [project docs index](<relative path to _docs/README.md>) · Code: `<code folder or files>`

<!-- Copy to <feature folder>/_docs/README.md (co-located) or _docs/features/<feature>/README.md
(cross-cutting). Keep this file a table of contents: one line per child doc, under 80 lines.
"scope" lists the code this feature owns as git pathspecs, relative to the project root
(the folder holding _docs/README.md): a folder (src/features/billing), files
(internal/core/enemy.go) or a glob (internal/core/enemy*.go). docs-check uses it to flag a
stale state, and reports entries git rejects. Delete rows for docs that don't exist yet, and
every placeholder line you can't fill: an unfilled link is a docs-check ERROR.

Document only what the code can't tell: intent (spec), reasons (decisions), progress (tasks,
state) and traps. Structure, function lists and call flows come from the code graph or the
code itself, which is never out of date. Add design.md only for a design that isn't
visible in the code (a protocol, a state machine, a data migration plan). -->

## Read in this order (stop when you have enough)

| Need | File |
|---|---|
| Where work stopped | [state.md](./state.md) |
| What's done and what's next | [tasks.md](./tasks.md) |
| What it must do (acceptance criteria) | [spec.md](./spec.md) |
| Why it's built this way | [decisions.md](./decisions.md) |
| How it's tested | [test-plan.md](./test-plan.md) |
| A design the code doesn't show (optional) | [design.md](./design.md) |

## Entry points (where to start reading code; not a full map)

| Path | Why start here |
|---|---|
| `<path>` | |

## Sub-topics (added when a doc is split)

| Topic | Index | One line |
|---|---|---|
| | | |

## Depends on / used by

- Uses: <other feature> (link its index README.md) for <what>
- Used by: <other feature> for <what>
