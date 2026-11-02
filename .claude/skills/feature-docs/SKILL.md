---
name: feature-docs
description: Manage the per-feature docs tree. "start <feature> [code-folder]" creates a feature's _docs folder and registers it; "resume [feature]" loads only the docs needed to continue where work stopped; "update" saves the current state (state.md, tasks.md); "split <file>" breaks an over-budget doc into an index plus smaller docs; "check" runs docs-check and fixes what it reports. Use when starting or resuming work on a feature, after finishing any piece of work, when a Stop hook says the state wasn't saved, or when docs-check reports a problem.
---

# Feature docs

Work with the docs tree described in the playbook's `11-feature-docs.md`. `$ARGUMENTS`
starts with one of: `start`, `resume`, `update`, `split`, `check`.

**Rules for every subcommand**

- Root index: `_docs/README.md` (it holds the **Feature registry**). Project pointer to
  active work: the `## Active work` section of `_docs/current-state.md`.
- A feature's docs live in `<code-folder>/_docs/` (co-located), or in
  `_docs/features/<feature>/` when the feature has no single code folder.
- **Document only what the code can't tell:** intent (`spec.md`), reasons
  (`decisions.md`), progress (`tasks.md`, `state.md`) and traps. Never describe structure,
  function lists or call flows; the code graph and the code are always current.
- One fact, one home. Link to a fact; never copy it into a second doc.
- Read **top-down** and stop when you have enough. Write **bottom-up**: leaf doc →
  `tasks.md` / `state.md` → registry row and Active work, and only if their one line changed.
- Finish every subcommand that writes by running the docs check
  (`node scripts/docs-check.mjs`) and fixing any ERROR.

## start <feature> [code-folder]

Use when a feature will take more than one session or gets a spec. Skip it for a tweak.

1. Choose the location. If `code-folder` is given, or one folder clearly owns the feature,
   use `<code-folder>/_docs/`. Otherwise use `_docs/features/<feature>/`. If it's unclear,
   ask the user with both options. Name the feature in lowercase-with-dashes: the session
   hook matches it against branch names (`feature/<feature>`).
   When the feature owns **files** in a shared folder rather than a folder of its own (a
   flat package such as Go's `internal/core`, or a `utils/` grab bag), use
   `_docs/features/<feature>/` and list the files, or a glob, in `scope:`
   (`scope: [internal/core/enemy.go, internal/core/enemy_ai.go]` or `[internal/core/enemy*.go]`).
   Scope entries are git pathspecs: docs-check matches them the same way locally and in CI.
2. If the folder already exists, stop and switch to `resume`. Never overwrite.
3. Create `README.md`, `state.md` and `tasks.md` from the templates bundled with this
   skill (`.claude/skills/feature-docs/templates/`). Fill in the purpose, `scope:`
   (folders, files or globs, relative to the project root), 1 to 5 entry points, and the
   "Up" link. Remove rows for docs that don't exist yet, and placeholder lines you can't
   fill (an unfilled link is a docs-check ERROR). Don't create `design.md` unless the
   design isn't visible in the code.
4. If a spec or plan exists elsewhere (e.g. `_docs/specs/<date>-<feature>.md`), **move**
   the spec to `spec.md` and turn the plan's steps into `tasks.md` checkboxes. Fix every
   link to the old path (search for the old filename).
5. Co-located only: create `<code-folder>/CLAUDE.md` from the bundled
   `feature-CLAUDE.template.md`. If a `CLAUDE.md` already exists there, show a diff that
   adds the pointer lines and ask. Cross-cutting features: create
   `.claude/rules/features/<feature>.md` with `paths:` listing the feature's files (globs
   are fine, e.g. `src/**/billing*.ts`), and the same lines.
6. Add one row to the Feature registry in `_docs/README.md`.
7. Set `## Active work` in `_docs/current-state.md` to this feature's `state.md`.
8. Suggest a branch named `feature/<feature>`, so parallel sessions resume the right feature.

## resume [feature]

Read-only. No feature named → the one matching the current git branch, else the one in
`## Active work`. (The SessionStart hook has usually printed this already.)

1. Read the feature's `state.md` "Now" and the first unchecked items of `tasks.md`.
2. From its `README.md`, read only the docs the **current task** needs. Use the code
   graph for code questions instead of reading whole files.
3. Report in at most 6 lines: status, current task, where it stopped, open questions,
   and which docs you read. Then wait for the user, unless they asked you to continue.

## update (save the current state)

Run after every piece of work that changed feature code, at the end of a session, and
when switching features. `/close-increment` also does this as part of closing an
increment. A Stop hook asks for it when code changed but the state didn't.

1. In `tasks.md`, tick each item whose check passed. Add newly found work at the end of
   its group or under "Later". Never tick an item whose check wasn't run.
2. For each leaf doc whose facts changed (spec, decisions, test plan), update that doc.
   When a doc and the code disagree, the code wins.
3. In `state.md`: **overwrite** "Now" (date, short SHA, status, current task, stopped at,
   open questions, traps). **Prepend** one 3-line "Recent sessions" entry and delete
   entries beyond the newest 5.
4. Update the feature's registry row only if its status or one-line summary changed.
5. Update `## Active work` in `_docs/current-state.md`: feature, link, current task, date.
6. Run the docs check.

If the change was too small to record (a typo, a rename), say so in one line instead.

## split <file>

Use when docs-check reports **too big**, or when one doc covers several topics.

1. Read the file. Group its `##` sections into 2 to 6 topics that are each readable on
   their own.
2. Turn `<topic>.md` into `<topic>/README.md` (a short index: one paragraph and a table
   with one line per child) plus `<topic>/<child>.md` files. Move text; don't rewrite it.
3. Find every link to the old path (`Grep` for its filename across `**/_docs/**` and
   `**/CLAUDE.md`) and point it at `<topic>/` or the specific child.
4. In the parent index, replace the old row with one row for the new index.
5. Run the docs check. It must show no broken links and no new orphans.

If a feature itself becomes too big (its index lists more than about 12 docs, or it
spans unrelated code folders), propose splitting it into two features instead, and ask.

## check

Run the docs check, then fix by kind:

| Finding | Fix |
|---|---|
| broken link | Point it at the moved file, or remove it if the target is gone for good |
| orphan index | Add a registry row in `_docs/README.md` |
| orphan doc | Link it from its folder's `README.md`, or delete it if it's obsolete (ask first) |
| missing state | Create `state.md` from the template |
| bad scope | Fix the `scope:` entries in the feature's index: folders, files or globs relative to the project root |
| too big | `split`, or trim `state.md` sessions to 5 |
| too deep | Flatten one level, or split the feature |
| stale state | Read the recent changes in the feature's scope (`git diff`, `git log -5 -- <scope>`) and run `update` |
