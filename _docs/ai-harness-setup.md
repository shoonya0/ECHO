# AI harness setup log

**In plain words:** the "AI harness" is the tooling that keeps AI-assisted work on this
project safe and checked: hooks, the `verify` script, CI and the project skills. This page
records how it was set up on 2026-11-01. You only need it to change that tooling. To work
on the app itself, start at the [docs index](./README.md).

This is the setup agent's record and resume point. If the setup is interrupted, start
the agent again with the same prompt and it continues from the first `TODO` row. Every
other status is final. The rows below are a log of that time, so they aren't updated later.

| Field | Value |
|---|---|
| Started | 2026-11-01 |
| Target | `D:\projects\ECHO` |
| Playbook | `D:\projects\ECHO\app-setup` (gitignored, local only) |
| Playbook version | `2026-10-14` |
| OS / shell | `win32` / Bash (Git Bash) |
| Mode | EXISTING |
| Stack | Go 1.26 toolchain (go.mod 1.24), Gin, MongoDB, Redis |
| Platforms (CI matrix) | Backend/API only → `ubuntu-latest` |
| Depth | Full |
| Methodology plugin | none (playbook loop) |
| Hosting | GitHub (`shoonya0/ECHO`) |
| Setup branch | `chore/ai-harness` from `origin/main` (`e1291b2`) |
| Verify command | `node scripts/verify.mjs` (full: `--full`) |

## Tools

| Tool | Needed? | Status | Version / evidence | Who installed |
|---|---|---|---|---|
| git | yes | DONE | 2.55.0.windows.3 | present |
| node ≥ 20 | yes | DONE | v24.19.0 | present |
| gh (+ auth) | yes | DONE | 2.102.0, logged in | present |
| uv / pipx / pip | no | SKIPPED | uv missing; not needed, code-review-graph already installed | — |
| code-review-graph | yes | DONE | present; graph 61 files / 417 nodes | present |
| agent-browser (+ Chrome) | no | SKIPPED | 0.38.1 present; backend only, no UI | — |
| stack toolchain | yes | DONE | go1.26.7, golangci-lint 2.14.0, gcc (for `-race`) | present |

## Steps

| ID | Step | Status | Evidence (command → result) / decision |
|---|---|---|---|
| P1 | Target confirmed | DONE | user: this folder |
| P2 | Platform + Node | DONE | `win32 x64 node v24.19.0` |
| P3 | Project inspected | DONE | `inspect-project.mjs` → Go, suggested EXISTING |
| P4 | Mode chosen | DONE | EXISTING |
| P5 | Scope answers | DONE | backend only · full · no methodology plugin · GitHub |
| P6 | Setup log created | DONE | this file |
| T1–T8 | Tools (see table) | DONE | `check-tools.mjs` → no required tool missing |
| E1 | Git + safety (branch) | DONE | `chore/ai-harness` from `origin/main`; only `app-setup/` untracked → gitignored |
| E2 | Secrets check | DONE | only `.env.example` tracked; no `.env` in history |
| E3 | Graph build | DONE | `code-review-graph build` → 78 files parsed, 417 nodes |
| E4 | Architecture tour | DONE | full code review 2026-11-01 → issues #18–#43 |
| E5 | architecture-overview + current-state | DONE | `_docs/README.md` renamed to `product-overview.md` (user approved); new index |
| E6 | Checks found | DONE | go build/vet/test, gofmt, golangci-lint; `scripts/contract-test` + `load-test` need a running server (not in verify) |
| E7 | Baseline at `e1291b2` | DONE | see current-state.md: all green except lint (28 issues) |
| E8 | Verify command | DONE | `scripts/verify.mjs`; lint uses `--new-from-merge-base=origin/main` so the baseline is green |
| E9 | Characterization tests | SKIPPED | first fixes (#18, #19) come with their own tests |
| H1 | .gitignore + .claude tracking | DONE | appended local-state block + `app-setup/`; `git check-ignore` → shared files not ignored |
| H2 | Verify command | DONE | see E8 |
| H3 | Hook scripts copied, adapted, stdin-tested | DONE | 8/8 expected exit codes; format hook uses gofmt; graph-update skipped (user-level hook exists); git-guard kept (runs twice with the user-level copy, harmless) |
| H4 | .claude/settings.json | DONE | my write was blocked (self-modification); the user applied the merged file (hooks, env, permissions, attribution off, plugins) |
| H5 | Skills + reviewer agent | DONE | verify-change rewritten for Go (verify.mjs, race, DB, live server, docker build; UI rung removed); close-increment log path `_docs/status/increments.md`; npm references removed |
| H6 | CLAUDE.md | DONE | merge rule: ask before every `gh pr merge` (main auto-deploys to Render) |
| H7 | Docs skeleton | DONE | index + feature registry + Active work; `docs-check.mjs` copied |
| H8 | Formatter + linter | DONE | gofmt + golangci-lint (existing tools); no repo-wide reformat |
| H9 | Architecture rules | DEFERRED | depguard needs a `.golangci.yml`; re-enable after the first fixes land |
| H10 | Pre-commit | DONE | `.githooks/` (docs-check, gofmt, vet); `core.hooksPath` set for this clone; shim 100755 |
| H11 | CI | DONE | `.github/workflows/verify.yml`, branch `main`, ubuntu, `verify.mjs --full` |
| H12 | Plugins | DONE | `claude plugin install … --scope project` → context7, security-guidance enabled in `.claude/settings.json` |
| H13 | code-review-graph MCP | DONE | `.mcp.json` rewritten portable (`code-review-graph serve`, no machine paths); 4 generated skills removed (identical to user-level ones); `claude mcp list` → pending approval |
| H14 | Agent UI access | SKIPPED | backend only; no UI |
| H15 | Optional extras | SKIPPED | not requested |
| V1–V5 | Restart + live checks | DONE | SessionStart hook printed Active work; guard-shell blocked a test command; graph MCP `semantic_search_nodes` answered; `claude mcp list` → code-review-graph connected |
| V6 | Completion report | DONE | below |
| V7 | Commit + PR | DONE | 9 commits pushed to `chore/ai-harness`; PR #44; CI `verify (ubuntu-latest)` pass after the docs-check fix `38c5b34`; merge waits for the user |

## Test baseline (EXISTING)

See [current-state.md](./current-state.md#test-baseline).

## Decisions made by the user

- EXISTING mode, backend only, full harness, no methodology plugin (superpowers not used).
- Branch from `origin/main`; `app-setup/` gitignored; graph rebuilt.
- `_docs/README.md` → `_docs/product-overview.md`, new index.
- Ask before every merge. Plugins: context7, security-guidance. Graph MCP: install.

## Deferred items (and what re-enables them)

- H9 depguard layer rules: after #18/#19 land, add `.golangci.yml` with the target layers.
- Lint baseline (28 issues): fix in a `chore/lint-baseline` branch, then drop `--new-from-merge-base`.

## Completion report

- **Works now:** hooks (session context, file protection, shell and git guards, gofmt on
  edit, Stop verify + state check), `node scripts/verify.mjs` (+ `--full` in CI),
  pre-commit via `.githooks/`, skills `verify-change` / `close-increment` /
  `feature-docs`, `code-reviewer` agent, code-review-graph MCP, plugins context7 and
  security-guidance.
- **Needs a human:** context7 asks for authentication on first use; each new clone runs
  `git config core.hooksPath .githooks`; the first CI run happens on the first push.
- **Deferred:** H9 depguard layer rules; the 28-issue lint baseline.
- **Next:** fix #18, then #19.
