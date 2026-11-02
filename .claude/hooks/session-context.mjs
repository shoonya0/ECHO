#!/usr/bin/env node
// SessionStart hook: tells the agent where work stopped, without it having to read anything.
// Stdout of a SessionStart hook is added to the session's context. With matcher "" it also
// runs after /clear and /compact, so the pointer survives compaction (nested CLAUDE.md
// files don't reload until a file in their folder is touched again).
//
// Which feature is active:
//   1. The git branch: "feature/billing", "fix/billing" or "billing" selects the row named
//      "billing" in the "## Feature registry" section of _docs/README.md (or whose docs
//      folder is named billing). Parallel sessions on different branches each get their
//      own feature.
//   2. Otherwise the feature linked from "## Active work" in _docs/current-state.md.
// Then it prints that feature's "## Now" section and the next unchecked items of tasks.md.
// Falls back to the first 40 lines of current-state.md when neither exists.
// Never fails the session: any error just prints less.

import { readFileSync, existsSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import path from 'node:path';

const MAX_LINES = 60;
const NEXT_TASKS = 3;
const root = process.env.CLAUDE_PROJECT_DIR || process.cwd();
const docsDir = path.join(root, '_docs');
const currentState = path.join(docsDir, 'current-state.md');
const registry = path.join(docsDir, 'README.md');
const shown = (p) => path.relative(root, p).split(path.sep).join('/');
const read = (p) => (existsSync(p) ? readFileSync(p, 'utf8') : '');
// HTML comments are maintainer notes and examples: never print them or follow their links.
const stripComments = (t) => t.replace(/<!--[\s\S]*?-->/g, '');
const slug = (s) => s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');

// Lines of "## <heading>" up to the next "## " heading (heading line included).
function section(text, heading) {
  const lines = stripComments(text).split(/\r?\n/);
  const start = lines.findIndex((l) => new RegExp(`^##\\s+${heading}`, 'i').test(l));
  if (start < 0) return null;
  let end = lines.findIndex((l, i) => i > start && /^##\s/.test(l));
  if (end < 0) end = lines.length;
  return lines.slice(start, end).join('\n').replace(/\n{3,}/g, '\n\n').trim();
}

function branchFeature() {
  const r = spawnSync('git', ['rev-parse', '--abbrev-ref', 'HEAD'], { cwd: root, encoding: 'utf8', windowsHide: true });
  const branch = (r.stdout || '').trim();
  if (!branch || ['HEAD', 'main', 'master', 'develop', 'trunk'].includes(branch)) return null;
  const want = slug(branch.split('/').pop());
  // Only registry rows name features. Rows elsewhere in the index ("Start here") link docs
  // in _docs/ itself, whose owner folder is the project root, so a branch named after the
  // project would match them.
  const text = read(registry);
  const rows = section(text, 'Feature registry') ?? stripComments(text);
  let best = null;
  for (const line of rows.split(/\r?\n/)) {
    const m = /^\|\s*([^|]+?)\s*\|\s*\[[^\]]*\]\(([^)\s]+)\)/.exec(line);
    if (!m) continue;
    const index = path.resolve(docsDir, m[2]);
    const folder = path.dirname(index);
    const owner = path.basename(folder) === '_docs' ? path.dirname(folder) : folder;
    for (const name of [slug(m[1]), slug(path.basename(owner))]) {
      const hit = want === name || want.startsWith(`${name}-`);
      if (hit && (!best || name.length > best.name.length)) best = { name, state: path.join(folder, 'state.md'), branch };
    }
  }
  return best;
}

function featureBlock(statePath) {
  if (!existsSync(statePath)) return [`(${shown(statePath)} doesn't exist. Fix the link or run /feature-docs start.)`];
  const out = [];
  const now = section(read(statePath), 'Now');
  if (now) out.push('', `# From ${shown(statePath)}`, '', now);
  const tasksPath = path.join(path.dirname(statePath), 'tasks.md');
  const tasks = stripComments(read(tasksPath)).split(/\r?\n/).filter((l) => /^\s*[-*]\s+\[[ xX]\]/.test(l));
  if (tasks.length) {
    const open = tasks.filter((l) => /\[ \]/.test(l));
    out.push('', `# ${shown(tasksPath)}: ${tasks.length - open.length}/${tasks.length} done. Next:`);
    out.push(...(open.length ? open.slice(0, NEXT_TASKS).map((l) => l.trim()) : ['(all tasks checked)']));
  }
  return out;
}

try {
  const out = [];
  const fromBranch = branchFeature();
  const stateText = read(currentState);
  const active = stateText ? section(stateText, 'Active work') : null;

  if (fromBranch) {
    out.push(`# Session context: branch "${fromBranch.branch}" → feature "${fromBranch.name}"`);
    out.push(...featureBlock(fromBranch.state));
  } else if (active) {
    out.push('# Session context (from _docs/current-state.md)', '', active);
    const link = /\]\(([^)\s]+state\.md)\)/.exec(active);
    if (link) out.push(...featureBlock(path.resolve(docsDir, link[1])));
  } else if (stateText) {
    out.push(stateText.split(/\r?\n/).slice(0, 40).join('\n'));
  }

  if (out.length) process.stdout.write(out.join('\n').split('\n').slice(0, MAX_LINES).join('\n') + '\n');
} catch {
  // Context is a convenience; never block a session over it.
}
process.exit(0);
