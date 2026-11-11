# ECHO

Real-time chat backend: REST + WebSocket API for an Android client. Stack: Go 1.24+
(Gin, gorilla/websocket), MongoDB, Redis (token blacklist + cross-instance pub/sub).
Platform: Linux server, Docker image auto-deployed to Render from `main`.

## Read first (map, not manual)

| Need | Read |
|---|---|
| Where the project stands, active work, test baseline | [_docs/current-state.md](_docs/current-state.md) |
| Docs index and feature registry | [_docs/README.md](_docs/README.md) |
| Packages, flows, data model, target layers, riskiest areas | [_docs/architecture-overview.md](_docs/architecture-overview.md) |
| Pub/sub and hub internals | [_docs/PUBSUB_ARCHITECTURE.md](_docs/PUBSUB_ARCHITECTURE.md) |
| WebSocket request/event contract | [_docs/WEBSOCKET_PROTOCOL.md](_docs/WEBSOCKET_PROTOCOL.md) |
| Known bugs | GitHub issues (`gh issue list`) |

## Commands

- Services: `docker compose -f docker/docker-compose.yml up -d` (MongoDB + Redis)
- Run: `go run ./cmd/server` (reads `.env`; copy `.env.example`)
- **Fast check, run before saying done: `node scripts/verify.mjs`**
  (docs-check, build, vet, gofmt, golangci-lint on new code, unit tests)
- Full check (CI): `node scripts/verify.mjs --full` (adds `go test -race ./...`)
- Single test: `go test ./internal/services -run TestName -v`
- DB tests skip unless MongoDB is reachable at `TEST_DB_URI`
  (default `mongodb://echo:echo@localhost:27017`). CI sets `TEST_DB_REQUIRED=1`, so
  there they fail instead of skipping. A skipped DB test is "not verified". Redis tests
  (`testutil.UseTestRedis`) work the same way, with `TEST_REDIS_URI` (default `localhost:6379`).
- Once per clone: `git config core.hooksPath .githooks` (pre-commit: docs-check, gofmt, vet)

## Workflow

- Find code with the code graph (code-review-graph: `semantic_search_nodes`,
  `query_graph` callers_of/tests_for, `get_impact_radius`) before Grep or reading files.
- Non-trivial change: spec → plan → implement in small steps → `/verify-change` → `/code-review`
  → `/close-increment`.
- Feature docs live in `<feature folder>/_docs/`. Read its `state.md` and `tasks.md` first.
  Link facts, don't copy them. Docs changed: `node scripts/docs-check.mjs`.
- Bugs: write a failing test that reproduces the bug first. Concurrency bugs in
  `internal/realtime`: reproduce under `go test -race`.
- Never delete, skip or weaken a test without explaining why and getting approval.
- Report evidence (commands + output), not claims. Use the validation categories in
  `/close-increment`.

## Git

- Never commit on `main`. Branch from an up-to-date base: `feature/<name>`,
  `fix/<issue#>-<name>`, `refactor/`, `docs/`, `chore/`. One branch, one change.
- Author = the signed-in GitHub user (`gh api user`): name = login, email =
  `<id>+<login>@users.noreply.github.com`. No AI watermark (Co-Authored-By, "Generated with
  Claude Code", 🤖) in commits, PRs or issues. `.claude/hooks/git-guard.mjs` enforces all three.
- Conventional Commits (`type(scope): summary`, body says why, `Fixes #N`). Stage by path;
  review `git diff --staged`.
- After each commit, verify: `git log -1 --format='%an <%ae>%n%n%B'`, `git show --stat HEAD`, `git status --short`.
- Feature done and verified → `/code-review` if non-trivial → push → PR (completion report
  in the body) → **ask the user before every `gh pr merge`** (merging to `main` deploys to
  Render) → delete branch.
- Bug outside the current task → propose a GitHub issue (title, repro, expected/actual); don't fix it here.

## Rules (each one exists because something went wrong)

- IMPORTANT: Always save the current state of the app after every piece of work: run
  `/feature-docs update` (tick `tasks.md`, overwrite "Now" in the feature's `state.md`,
  update "Active work" in `_docs/current-state.md`). A Stop hook checks this.
- IMPORTANT: Secrets only from env (`internal/config`). Never in code, logs, fixtures or
  prompts. `.env` is gitignored and hook-protected.
- New dependency: say why, and check that it exists and is maintained, before `go get`.
- Hub maps (`ChatClients`, `UserClients`, `UserActiveChats`) are touched only under
  `hub.Mutex`; never range over an inner map after unlocking (#19).
- Every WebSocket handler that takes a `chatId` checks membership (#23).
- Multi-document writes: check `MatchedCount`, don't discard errors (`_, _ =`), prefer
  `$addToSet` over `$push` for ID lists (#37).
- Service and WebSocket errors go through a mapper; never send `err.Error()` to clients (#41).
- Respect the target layers in `_docs/architecture-overview.md#target-layers-not-yet-enforced`.
- `go.sum` is protected: change it with `go get` / `go mod tidy`, not by editing.

## When compacting

Keep: the list of modified files, the current task and its issue number, and the
verify command with its latest result.
