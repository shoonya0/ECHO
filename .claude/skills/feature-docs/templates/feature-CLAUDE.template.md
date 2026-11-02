# Feature: <feature>

Docs for this folder live in `<feature folder>/_docs/`.

- Before changing code here, read `<feature folder>/_docs/state.md` (where work stopped)
  and `tasks.md` (what's next), then follow `_docs/README.md` only as far as the task needs.
- After any piece of work here, save the state: `/feature-docs update` (tick `tasks.md`,
  overwrite "Now" in `state.md`, update the leaf doc whose facts changed). Link facts;
  don't copy them.
- <Rule that only applies to this feature, e.g. "Amounts are integer cents; never floats.">

<!-- Copy to <feature folder>/CLAUDE.md. Claude Code loads a subfolder's CLAUDE.md the first
time it reads or edits a file in that folder, so this pointer costs nothing until the feature
is touched. Keep it under 15 lines. It is named .template.md here so it doesn't auto-load
inside the playbook. For a feature with no single folder, put the same lines in
.claude/rules/features/<feature>.md with "paths:" frontmatter listing its files. -->
