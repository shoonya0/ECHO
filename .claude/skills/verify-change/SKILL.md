---
name: verify-change
description: Run the project's verification ladder (build, vet, gofmt, lint, unit tests, and when relevant race tests, DB tests and a live server check) and report evidence in a fixed format. Use before claiming any change is done, after implementing a plan step, or when the user asks "does it work?".
---

# Verify change

Prove the current change works. Report **evidence, not claims**.

## 1. Scope

- `git status` and `git diff --stat`: what changed?
- Choose the rungs needed. Rung 1 always runs. Add the others when the change
  touches them:

| Rung | Command | Run when |
|---|---|---|
| 1 Fast | `node scripts/verify.mjs` (docs-check, build, vet, gofmt, lint on new code, unit tests) | Always |
| 2 Race | `node scripts/verify.mjs --full` (adds `go test -race ./...`) | Anything in `internal/realtime`, goroutines, shared maps, channels |
| 3 DB | `docker compose -f docker/docker-compose.yml up -d`, then `go test ./... -v` (set `TEST_DB_URI` if not the default) | `internal/services`, queries, indexes, models. Check the DB tests **ran**, not skipped |
| 4 Live server | `go run ./cmd/server`, then `curl localhost:8080/health` and the changed endpoint or WS flow (`scripts/contract-test`) | Handlers, routes, middleware, WebSocket protocol |
| 5 Container | `docker build .` | `Dockerfile`, `go.mod`, build paths |

Single test: `go test ./internal/<pkg> -run TestName -v`.

## 2. Run

- Run each chosen rung. On failure, find the **root cause** and fix it, then rerun
  from rung 1.
- Never delete, skip or weaken a test to get green. If a test is wrong, stop and
  explain why.
- Compare failures against the **baseline** in `_docs/current-state.md` by test name.
  A pre-existing failure is reported as such, with evidence. It is never silently
  ignored. A DB test that skipped is "not verified", not a pass.

## 3. Report (exact format)

```markdown
### Verification
| Rung | Command | Result | Notes |
|---|---|---|---|
| Fast | `node scripts/verify.mjs` | ✅ pass (25 s) | 6/6 steps |
| Race | `node scripts/verify.mjs --full` | ✅ pass | -race clean |
| DB | `go test ./internal/services -v` | ✅ pass | 2 DB tests ran (not skipped) |

- New failures vs baseline @<sha>: none
- Not verified: <list, or "nothing">
- Requires a manual check (Android client, Render deploy): <list, or "nothing">
```

If any rung fails and can't be fixed in scope, stop and report it. Don't claim done.
