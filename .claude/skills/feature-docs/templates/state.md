# <Feature>: state

<!-- The feature's "where did we stop" file. "Now" is overwritten at the end of every session
or increment; it never grows. Progress lives in tasks.md as checkboxes; "Now" says only what
a checklist can't: where inside the current task work stopped, open questions, and traps.
"Recent sessions" keeps the last 5 entries (git keeps the rest). Budget: 120 lines. Evidence
and test tables belong in the status log (_docs/status/phase-<N>.md, or
_docs/status/increments.md in a project without a roadmap), not here. -->

## Now (updated YYYY-MM-DD at `<sha>`)

- **Status:** ⏳ planned / 🟡 in progress / ✅ done / ⚠️ blocked
- **Current task:** <number and title from [tasks.md](./tasks.md)>
- **Stopped at:** <inside that task: `file:line`, or "test written, failing as expected">
- **Open questions / blockers:** <none, or each one with who decides>
- **Don't forget:** <a trap found this session that the next one would step into>

## Recent sessions (newest first, keep 5)

### YYYY-MM-DD: <title>

- Changed: <files or modules, one line>
- Verified: <command → result, or "not verified">
- Docs touched: <which docs were updated>

## Known issues

- <issue> (`file:line`, tracked in <issue link>)
