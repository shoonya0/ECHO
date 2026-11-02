// Stop hook
//
// Enforces the rule "save the current state after every piece of work". When the agent
// tries to finish a turn, it runs scripts/docs-check.mjs and BLOCKS the stop once (exit
// code 2, stderr shown to the agent) if a feature's code has uncommitted changes but its
// state.md / tasks.md don't. The agent then records the state (/feature-docs update) or
// says why there's nothing to record, and the next stop goes through.
//
// Guards against loops and noise:
// - Blocks at most once per stop: when Claude Code reports stop_hook_active (the agent is
//   already continuing because of a Stop hook), it lets the stop through.
// - Only uncommitted changes count, so turns that changed no feature code are free.
// - Set CLAUDE_STATE_HOOK=off in settings "env" to disable it without editing settings.
//
// Cross-platform: runs docs-check with the current Node binary, no shell.

import { spawnSync } from 'node:child_process';
import { existsSync } from 'node:fs';
import path from 'node:path';

function readStdin() {
  return new Promise((resolve) => {
    let raw = '';
    process.stdin.setEncoding('utf8');
    process.stdin.on('data', (chunk) => (raw += chunk));
    process.stdin.on('end', () => resolve(raw));
    process.stdin.on('error', () => resolve(raw));
  });
}

const raw = await readStdin();
let input = {};
try {
  input = JSON.parse(raw || '{}');
} catch {
  // malformed input: behave as if there's nothing to check
}

if (process.env.CLAUDE_STATE_HOOK === 'off' || input.stop_hook_active) process.exit(0);

const root = process.env.CLAUDE_PROJECT_DIR || input.cwd || process.cwd();
const checker = path.join(root, 'scripts', 'docs-check.mjs');
if (!existsSync(checker)) process.exit(0);

const r = spawnSync(process.execPath, [checker, '--json'], {
  cwd: root,
  encoding: 'utf8',
  windowsHide: true,
  timeout: 60_000,
});
let report;
try {
  report = JSON.parse(r.stdout);
} catch {
  process.exit(0); // the checker itself failed; verify / CI will surface that
}

const stale = report.findings.filter((f) => f.msg.startsWith('stale state: uncommitted'));
if (stale.length === 0) process.exit(0);

process.stderr.write(
  'Save the current state before stopping. Feature code changed but its state was not updated:\n' +
    stale.map((f) => `  - ${f.file}`).join('\n') +
    '\nRun /feature-docs update: overwrite "Now" in state.md (stopped at, next step, open ' +
    'questions), tick finished items in tasks.md, and add one "Recent sessions" entry. If the ' +
    'change is too small to record, say so in one line and stop.\n',
);
process.exit(2);
