// .githooks/pre-commit.mjs: fast pre-commit checks for stacks without husky or the
// pre-commit framework (Go, Rust, .NET, …). No dependencies beyond Node.
//
// 1. docs-check: broken links, orphan feature indexes and missing state.md block the
//    commit. Budget and stale-state warnings are printed only; CI fails on them for PRs.
// 2. CHECKS: the stack's fast checks on the staged files. Keep them under a few seconds;
//    tests, builds and full lint belong in verify (Stop hook) and CI.
//
// Git runs hooks from the top of the working tree. Enable with:
//   git config core.hooksPath .githooks
// That setting is per clone: say so in the README / CLAUDE.md so every clone runs it once.

import { spawnSync } from 'node:child_process';
import { existsSync } from 'node:fs';

// Adapt per stack (see the playbook's agent/reference/stack-matrix.md). `files` filters the
// staged files a check gets as arguments; `whole: true` runs once without file arguments.
// `failOnOutput` treats any stdout as a failure (for "list unformatted files" commands).
// Commands run without a shell, so on Windows name real executables (gofmt, cargo, ruff),
// not .cmd shims such as npx; JS projects use husky instead (agent/05-harness.md H10).
const CHECKS = [
  // Go:
  { name: 'gofmt', files: /\.go$/, cmd: 'gofmt', args: ['-l'], failOnOutput: true },
  { name: 'go vet', files: /\.go$/, cmd: 'go', args: ['vet', './...'], whole: true },
  // Rust:
  // { name: 'rustfmt', files: /\.rs$/, cmd: 'cargo', args: ['fmt', '--check'], whole: true },
  // Python (ruff):
  // { name: 'ruff', files: /\.py$/, cmd: 'ruff', args: ['check'] },
];

const run = (cmd, args, opts = {}) => spawnSync(cmd, args, { encoding: 'utf8', windowsHide: true, ...opts });
const fail = (msg, code = 1) => {
  process.stderr.write(`pre-commit: ${msg}\n`);
  process.exit(code);
};

if (existsSync('scripts/docs-check.mjs')) {
  const docs = run(process.execPath, ['scripts/docs-check.mjs'], { stdio: 'inherit' });
  if (docs.status !== 0) fail('docs-check found errors (fix them, or run /feature-docs check)', docs.status || 1);
}

const staged = run('git', ['diff', '--cached', '--name-only', '--diff-filter=ACMR'])
  .stdout.split(/\r?\n/)
  .filter(Boolean);

for (const c of CHECKS) {
  const files = staged.filter((f) => c.files.test(f));
  if (files.length === 0) continue;
  const r = run(c.cmd, c.whole ? c.args : [...c.args, ...files], {
    stdio: c.failOnOutput ? 'pipe' : 'inherit',
  });
  if (r.error) fail(`${c.name}: ${c.cmd} not found (${r.error.message})`);
  if (r.status !== 0) fail(`${c.name} failed${r.stderr ? `:\n${r.stderr}` : ''}`, r.status || 1);
  if (c.failOnOutput && r.stdout.trim()) fail(`${c.name} flagged these files:\n${r.stdout}`);
}
