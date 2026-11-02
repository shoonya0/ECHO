---
name: close-increment
description: Close out a finished increment by updating the status log, the feature's tasks.md and state.md, and current-state, recording lessons, writing the completion report, and making an atomic commit. Invoke manually after /verify-change and review pass.
disable-model-invocation: true
---

# Close increment

Run this only after `/verify-change` passed and review findings were addressed. `$ARGUMENTS`
may name the phase or increment, e.g. `phase-3 increment 2`.

## 1. Confirm the gate

- `/verify-change` evidence exists from **this** state of the tree. If anything changed since,
  rerun `/verify-change`.
- Every acceptance criterion in the spec is marked PASS with evidence.

## 2. Update the docs (same commit as the code)

1. The **status log**: `_docs/status/increments.md` (newest first). Add or complete
   the increment section:
   - **Problem**: one or two sentences.
   - **Change**: files and modules, and behavior before vs after.
   - **Tests**: what was added, with suite names and counts.
   - **Evidence**: the verification table from `/verify-change`.
   - **Commit**: write `<pending>`. Fill in the hash in the next increment's commit; for
     the feature's last increment, in a follow-up `docs:` commit before pushing (an amend
     can't record its own hash, since amending changes it).

   A new log file needs a link from `_docs/README.md` (docs-check flags an unlinked doc
   as an orphan). If the log is past ~180 lines (budget 200), first rename it to
   `<name>-<YYYY-MM>.md` (month of its newest entry), link that archive from
   `_docs/README.md`, then start a fresh file. Don't use `/feature-docs split` on a log.
2. **Feature docs** (if the work belongs to a feature with its own `_docs/` folder):
   tick the finished items in its `tasks.md`, update each leaf doc whose facts changed,
   then the feature's `state.md` (overwrite
   "Now"; prepend one 3-line session entry linking this increment; keep the newest 5),
   then its registry row in `_docs/README.md` only if status or summary changed. The
   `update` subcommand of `/feature-docs` describes the same steps.
3. `_docs/current-state.md`: always update `## Active work` (feature, its `state.md`,
   next step). Update the rest only if the project's state changed (phase status, open
   decisions, baseline). Update the snapshot SHA and date.
4. If a doc you read during this work turned out to be **wrong**, fix it now and name
   it in the report.
5. Run `node scripts/docs-check.mjs`. Fix every ERROR, and
   every "stale state" or "too big" warning about the files this increment touched.

## 3. Lessons

Did a mistake happen that will happen again (a wrong command, a missed convention,
the wrong layer)?

- If yes, add **one** concise line to CLAUDE.md, *or* propose a hook if it must never
  happen, *or* a path rule in `.claude/rules/` if it only applies to some files.
- If no, change nothing. Don't add generic advice.

## 4. Completion report

```markdown
### Change summary
- Files changed / behavior changed / modules affected

### Validation
- Covered by automated tests: …
- Verified against a running server (curl / WS contract test): …
- Reviewed but not executed: …
- Requires a manual check (Android client, Render deploy): …

### Commands executed
- `…` → result

### Remaining risks
- …
```

## 5. Commit

- Stage **only** the files changed for this increment (`git add <paths>`, never
  `git add -A` blindly). Check with `git diff --cached --stat`, and confirm no new
  source file the change needs is left untracked.
- Commit on the feature branch, never on the default branch. Message:
  `<type>(<scope>): <what>`, a blank line, then *why*, then `Fixes #N` if an issue exists.
  No AI attribution (`Co-Authored-By`, "Generated with …"); the author is the GitHub user
  (CLAUDE.md "Git"; `git-guard.mjs` blocks the rest).
- Verify: `git log -1 --format='%an <%ae>%n%n%B'`, `git show --stat HEAD`,
  `git status --short`. Report the hash.
- Push only as CLAUDE.md "Git" says: increments are committed locally; when this one
  **finishes the feature** (every `tasks.md` box ticked), run `/code-review` on the branch
  if the feature is non-trivial and fix its findings, add the follow-up commit for the
  hash, then push, open a PR with the completion report in its body, and merge by the
  project's merge rule there. Otherwise, push or open a PR only if the user asked.
