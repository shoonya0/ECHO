#!/usr/bin/env node
// scripts/docs-check.mjs: integrity check for the docs tree (playbook 11-feature-docs.md).
// Treats the docs like a small database and enforces its constraints:
//
//   ERROR  broken link        a relative markdown link points at a file that doesn't exist
//   ERROR  orphan index       a <feature>/_docs/README.md isn't reachable from _docs/README.md
//   ERROR  missing state      a feature index has no state.md next to it
//   ERROR  bad scope          git rejects a feature's scope: entries (so its stale check can't run)
//   WARN   orphan doc         a doc under _docs/ isn't linked from any index
//   WARN   too big            a file is over its line budget: split it (/feature-docs split <file>)
//   WARN   too deep           a doc is more than MAX_DEPTH links below the root index
//   WARN   stale state        code in a feature's scope changed but its state.md / tasks.md didn't
//
//   node scripts/docs-check.mjs              # human output; exit 1 on errors
//   node scripts/docs-check.mjs --strict     # warnings fail too
//   node scripts/docs-check.mjs --json       # machine output
//   node scripts/docs-check.mjs --root <dir> # check another folder (default: cwd)
//   node scripts/docs-check.mjs --strict --base origin/main   # pull requests (CI)
//
// --base <ref> judges only what this branch changed since <ref>: a feature is stale when
// the branch touches its code but neither its state.md nor its tasks.md, and size, depth
// and orphan warnings are reported only for files the branch changed. Broken links,
// orphan indexes and missing state are always reported. A commit message in the range
// containing [docs-skip] waives the stale check (for renames, formatting, dependency bumps).
// CI needs the full history for this: actions/checkout with fetch-depth: 0.
//
// A feature's "scope:" entries are git pathspecs, relative to the project root: a folder
// (src/billing), a file (internal/core/enemy.go) or a glob (internal/core/enemy*.go). Git
// matches them, so local runs and --base runs agree on what counts as the feature's code.
//
// Node built-ins only; works the same on Windows and macOS. Git is optional: without it
// the stale-state check is skipped.

import { readFileSync, readdirSync, existsSync, statSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import path from 'node:path';

// Budgets in lines. An index is a table of contents; a leaf holds one topic.
const BUDGET = { index: 80, state: 120, leaf: 200 };
const MAX_DEPTH = 4; // root index → feature index → topic index → leaf
const SKIP_DIRS = new Set([
  'node_modules', '.git', 'vendor', 'dist', 'build', 'out', 'coverage', 'target',
  '.venv', 'venv', '__pycache__', '.next', '.code-review-graph', '.scaffold-tmp',
]);

const args = process.argv.slice(2);
const STRICT = args.includes('--strict');
const JSON_OUT = args.includes('--json');
const rootArg = args.indexOf('--root');
const baseArg = args.indexOf('--base');
const BASE = baseArg >= 0 ? args[baseArg + 1] : null;
const ROOT = path.resolve(rootArg >= 0 ? args[rootArg + 1] : process.cwd());
const ROOT_INDEX = path.join(ROOT, '_docs', 'README.md');

let findings = [];
const rel = (p) => path.relative(ROOT, p).split(path.sep).join('/');
const add = (level, file, line, msg) => findings.push({ level, file: rel(file), line, msg });

// ---------- collect files ----------------------------------------------------------
const docs = []; // every .md under a _docs/ folder, plus every CLAUDE.md
function walk(dir, insideDocs) {
  let entries;
  try { entries = readdirSync(dir, { withFileTypes: true }); } catch { return; }
  for (const e of entries) {
    const full = path.join(dir, e.name);
    if (e.isDirectory()) {
      if (SKIP_DIRS.has(e.name)) continue;
      if (e.name === 'worktrees' && path.basename(dir) === '.claude') continue;
      walk(full, insideDocs || e.name === '_docs');
    } else if (e.name.endsWith('.md') && (insideDocs || e.name === 'CLAUDE.md')) {
      docs.push(full);
    }
  }
}
walk(ROOT, false);

if (!existsSync(ROOT_INDEX)) {
  add('error', ROOT_INDEX, 0, 'root index _docs/README.md is missing; create it from templates/docs/docs-index.md');
  finish();
}

// ---------- parse ---------------------------------------------------------------------
function parseFrontmatter(text) {
  const m = /^---\r?\n([\s\S]*?)\r?\n---/.exec(text);
  const fm = {};
  if (!m) return fm;
  const rows = m[1].split(/\r?\n/);
  for (let i = 0; i < rows.length; i++) {
    const kv = /^([A-Za-z_][\w-]*):\s*(.*)$/.exec(rows[i]);
    if (!kv) continue;
    let v = kv[2].trim();
    if (v === '') {
      // Block list:  scope:\n  - a\n  - b
      const items = [];
      while (i + 1 < rows.length && /^\s*-\s+/.test(rows[i + 1])) {
        items.push(rows[++i].replace(/^\s*-\s+/, '').trim().replace(/^['"]|['"]$/g, ''));
      }
      fm[kv[1]] = items.length ? items.filter(Boolean) : '';
      continue;
    }
    if (v.startsWith('[') && v.endsWith(']')) {
      v = v.slice(1, -1).split(',').map((s) => s.trim().replace(/^['"]|['"]$/g, '')).filter(Boolean);
    } else {
      v = v.replace(/^['"]|['"]$/g, '');
    }
    fm[kv[1]] = v;
  }
  return fm;
}

// Relative links outside code fences, HTML comments and inline code: [text](target)
function parseLinks(text) {
  const links = [];
  let inFence = false;
  let inComment = false;
  text.split(/\r?\n/).forEach((raw, i) => {
    if (!inComment && /^\s*(```|~~~)/.test(raw)) { inFence = !inFence; return; }
    if (inFence) return;
    let line = raw;
    if (inComment) {
      const end = line.indexOf('-->');
      if (end < 0) return;
      line = line.slice(end + 3);
      inComment = false;
    }
    line = line.replace(/<!--[\s\S]*?-->/g, '');
    const open = line.indexOf('<!--');
    if (open >= 0) { line = line.slice(0, open); inComment = true; }
    line = line.replace(/`[^`]*`/g, '');
    // Targets in <…> may contain spaces, so an unfilled placeholder such as
    // (<relative path to _docs/README.md>) is checked, and reported, like any link.
    for (const m of line.matchAll(/\[[^\]]*\]\(\s*(?:<([^>]+)>|([^)\s]+))(?:\s+"[^"]*")?\s*\)/g)) {
      const target = (m[1] ?? m[2]).trim();
      if (/^[a-z][a-z0-9+.-]*:/i.test(target) || target.startsWith('#')) continue; // URL or anchor
      links.push({ target: decodeURIComponent(target.split('#')[0].split('?')[0]), line: i + 1 });
    }
  });
  return links;
}

const info = new Map(); // abs path → { lines, fm, links: [abs] }
for (const file of docs) {
  const text = readFileSync(file, 'utf8');
  const entry = { lines: text.split(/\r?\n/).length, fm: parseFrontmatter(text), links: [] };
  for (const { target, line } of parseLinks(text)) {
    let abs = path.resolve(path.dirname(file), target);
    if (!existsSync(abs)) { add('error', file, line, `broken link → ${target}`); continue; }
    if (statSync(abs).isDirectory()) {
      const idx = path.join(abs, 'README.md');
      if (!existsSync(idx)) continue; // a link to a code folder is fine
      abs = idx;
    }
    if (abs.endsWith('.md')) entry.links.push(abs);
  }
  info.set(file, entry);
}

// ---------- reachability from the root index (the "registry") ------------------------
const depth = new Map([[ROOT_INDEX, 0]]);
const queue = [ROOT_INDEX];
while (queue.length) {
  const cur = queue.shift();
  const entry = info.get(cur);
  if (!entry) continue;
  for (const next of entry.links) {
    if (!depth.has(next)) { depth.set(next, depth.get(cur) + 1); queue.push(next); }
  }
}
// current-state.md and CLAUDE.md are entry points too: links from them count.
for (const entry of [path.join(ROOT, '_docs', 'current-state.md'), path.join(ROOT, 'CLAUDE.md')]) {
  for (const next of info.get(entry)?.links ?? []) if (!depth.has(next)) depth.set(next, 1);
}

// A feature index is a README.md with "kind: feature" in its frontmatter, or any
// <folder>/_docs/README.md other than the root one unless it says "kind: area".
const isFeatureIndex = (f) => {
  if (path.basename(f) !== 'README.md' || f === ROOT_INDEX) return false;
  const kind = info.get(f)?.fm.kind;
  if (kind) return kind === 'feature';
  return path.basename(path.dirname(f)) === '_docs';
};

for (const [file, entry] of info) {
  const base = path.basename(file);
  if (base === 'CLAUDE.md') continue; // pointer files: link-checked only
  const inDocs = rel(file).split('/').includes('_docs');
  if (!inDocs) continue;

  if (file !== ROOT_INDEX && !depth.has(file)) {
    if (isFeatureIndex(file)) {
      add('error', file, 0, 'orphan index: add a row for it to the Feature registry in _docs/README.md');
    } else {
      add('warn', file, 0, 'orphan doc: link it from its folder index (README.md)');
    }
  }
  const d = depth.get(file);
  if (d !== undefined && d > MAX_DEPTH) {
    add('warn', file, 0, `too deep: ${d} links below _docs/README.md (max ${MAX_DEPTH}); flatten it or split the feature`);
  }

  const kind = base === 'README.md' ? 'index' : base === 'state.md' ? 'state' : 'leaf';
  if (entry.lines > BUDGET[kind]) {
    const fix = kind === 'state'
      ? 'trim "Recent sessions" to the last 5 and move detail into topic docs'
      : kind === 'index'
        ? 'keep one line per child; move detail into the child docs'
        : `split it: /feature-docs split ${rel(file)}`;
    add('warn', file, 0, `too big: ${entry.lines} lines (budget ${BUDGET[kind]}); ${fix}`);
  }
}

// ---------- feature indexes: state.md exists, and isn't stale ------------------------
const git = (...a) => spawnSync('git', a, { cwd: ROOT, encoding: 'utf8', windowsHide: true });
const hasGit = git('rev-parse', '--is-inside-work-tree').stdout?.trim() === 'true';
const hasHead = hasGit && git('rev-parse', '--verify', '-q', 'HEAD').status === 0; // false before the first commit
const lines = (r) => (r.stdout || '').split(/\r?\n/).filter(Boolean);

// Git output for a feature's scope. A command git rejects (a malformed pathspec, a path
// outside the repo) throws instead of reading as "nothing changed": a typo must never
// switch the stale check off silently.
class ScopeError extends Error {}
const gitScope = (...a) => {
  const r = git(...a);
  if (r.status !== 0) throw new ScopeError((r.stderr || `git exited ${r.status}`).trim().split(/\r?\n/)[0]);
  return r;
};

// --base: files this branch changed (committed since BASE, plus staged and unstaged edits)
let changed = null;
let skipStale = false;
// Files changed in this branch, optionally limited to git pathspecs.
const branchChanges = (...spec) => [
  ...lines(gitScope('diff', '--name-only', '--relative', `${BASE}...HEAD`, '--', ...spec)),
  ...lines(gitScope('diff', '--name-only', '--relative', '--', ...spec)),
  ...lines(gitScope('diff', '--name-only', '--relative', '--cached', '--', ...spec)),
];
if (BASE && hasGit) {
  if (git('diff', '--quiet', `${BASE}...HEAD`).status > 1) {
    add('error', ROOT_INDEX, 0, `--base ${BASE}: git can't diff that ref (in CI, check out with fetch-depth: 0)`);
    finish();
  }
  changed = new Set(branchChanges());
  skipStale = /\[docs-skip\]/i.test(git('log', '--format=%B', `${BASE}..HEAD`).stdout || '');
}

for (const [file, entry] of info) {
  if (!isFeatureIndex(file)) continue;
  const docsDir = path.dirname(file);
  const state = path.join(docsDir, 'state.md');
  const tasks = path.join(docsDir, 'tasks.md');
  if (!existsSync(state)) {
    add('error', file, 0, 'missing state: every feature index needs a state.md next to it (templates/docs/feature/state.md)');
    continue;
  }
  if (!hasGit) continue;
  const stateFiles = [state, ...(existsSync(tasks) ? [tasks] : [])].map(rel);

  // scope: frontmatter list of git pathspecs (folders, files, globs); default = the folder holding _docs/
  let scope = entry.fm.scope;
  if (typeof scope === 'string') scope = [scope];
  scope = (scope || []).map((p) => p.split('\\').join('/').replace(/\/\*\*$/, '').replace(/\/+$/, '')).filter(Boolean);
  if (scope.length === 0) {
    const owner = path.dirname(docsDir);
    if (owner === ROOT) continue; // root-level feature folder with no scope: nothing to compare
    scope = [rel(owner)];
  }
  const docsRel = rel(docsDir);
  const where = scope.join(', ');

  const pathspec = [...scope, `:(exclude)${docsRel}`];

  try {
    if (changed) {
      if (skipStale) continue;
      const codeTouched = branchChanges(...pathspec).length > 0;
      const stateTouched = stateFiles.some((f) => changed.has(f));
      if (codeTouched && !stateTouched) {
        add('warn', state, 0, `stale state: this branch changes code in [${where}] but neither state.md nor tasks.md; run /feature-docs update (or put [docs-skip] in a commit message if there's nothing to record)`);
      }
      continue;
    }

    // Commits that touched the code after the last commit that touched state.md / tasks.md.
    // Ancestry, not timestamps: same-second commits, rebases and clock skew can't hide them.
    if (hasHead) {
      const lastState = gitScope('log', '-1', '--format=%H', '--', ...stateFiles).stdout.trim();
      const codeAfter = Number(gitScope('rev-list', '--count', lastState ? `${lastState}..HEAD` : 'HEAD', '--', ...pathspec).stdout.trim() || 0);
      if (codeAfter > 0) {
        add('warn', state, 0, `stale state: code in [${where}] was committed after state.md / tasks.md last changed; update "Now"`);
      }
    }
    const dirtyCode = gitScope('status', '--porcelain', '--', ...pathspec).stdout.trim();
    const dirtyState = gitScope('status', '--porcelain', '--', ...stateFiles).stdout.trim();
    if (dirtyCode && !dirtyState) {
      add('warn', state, 0, `stale state: uncommitted changes in [${where}] but state.md / tasks.md are unchanged`);
    }
  } catch (e) {
    if (!(e instanceof ScopeError)) throw e;
    add('error', file, 0, `bad scope: git rejected [${where}] (${e.message}); scope entries are git pathspecs relative to the project root (the folder holding _docs/README.md)`);
  }
}

// --base: keep size, depth and orphan-doc warnings only for files this branch changed.
if (changed) {
  findings = findings.filter((f) => f.level === 'error' || f.msg.startsWith('stale state') || changed.has(f.file));
}
if (skipStale) process.stderr.write(`docs-check: [docs-skip] found in ${BASE}..HEAD; stale-state check waived\n`);

finish();

function finish() {
  const errors = findings.filter((f) => f.level === 'error').length;
  const warns = findings.length - errors;
  if (JSON_OUT) {
    process.stdout.write(JSON.stringify({ root: ROOT, errors, warnings: warns, findings }, null, 2) + '\n');
  } else {
    findings.sort((a, b) => (a.level === b.level ? a.file.localeCompare(b.file) : a.level === 'error' ? -1 : 1));
    for (const f of findings) {
      process.stdout.write(`${f.level === 'error' ? 'ERROR' : 'WARN '} ${f.file}${f.line ? `:${f.line}` : ''}  ${f.msg}\n`);
    }
    process.stdout.write(`docs-check: ${docs.length} files, ${errors} error(s), ${warns} warning(s)\n`);
  }
  process.exit(errors > 0 || (STRICT && warns > 0) ? 1 : 0);
}
