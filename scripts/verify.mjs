#!/usr/bin/env node
// scripts/verify.mjs — one cross-platform "verify" entry point for projects that
// don't use npm scripts (Python, Go, Rust, Java, .NET …). Runs the fast checks in
// order and stops at the first failure; `--full` adds the slow checks.
//
//   node scripts/verify.mjs          # fast: what the Stop hook runs
//   node scripts/verify.mjs --full   # fast + slow: what CI runs
//
// Set CLAUDE_VERIFY_CMD to "node scripts/verify.mjs" in .claude/settings.json "env".
// Edit STEPS for your stack (see the playbook's agent/reference/stack-matrix.md).
// Commands are constant strings from this file, run through the platform shell so
// tool shims (.cmd/.bat on Windows) resolve; never build them from untrusted input.

import { spawnSync } from 'node:child_process';

// gofmt -l prints unformatted files but exits 0, so the format step fails on any output.
// Lint only reports issues new since the merge base with origin/main: the 28 issues in
// the baseline (see _docs/current-state.md) don't block, new ones do.
const FAST = [
  { name: 'docs-check', cmd: 'node scripts/docs-check.mjs' },
  { name: 'build', cmd: 'go build ./...' },
  { name: 'vet', cmd: 'go vet ./...' },
  { name: 'format', cmd: 'gofmt -l cmd internal', failOnOutput: true },
  { name: 'lint (new issues)', cmd: 'golangci-lint run --new-from-merge-base=origin/main ./...' },
  { name: 'unit tests', cmd: 'go test ./...' },
];

const SLOW = [
  // DB tests skip without MongoDB; they use TEST_DB_URI, default mongodb://echo:echo@localhost:27017 (docker-compose).
  { name: 'race tests', cmd: 'go test -race ./...' },
];

const steps = process.argv.includes('--full') ? [...FAST, ...SLOW] : FAST;
const summary = [];

for (const step of steps) {
  const started = Date.now();
  process.stdout.write(`\n▶ ${step.name}: ${step.cmd}\n`);
  const r = spawnSync(step.cmd, {
    shell: true,
    stdio: step.failOnOutput ? ['inherit', 'pipe', 'inherit'] : 'inherit',
    encoding: 'utf8',
    windowsHide: true,
  });
  if (step.failOnOutput && r.stdout) process.stdout.write(r.stdout);
  const seconds = ((Date.now() - started) / 1000).toFixed(1);
  const ok = r.status === 0 && !r.error && !(step.failOnOutput && r.stdout?.trim());
  summary.push(`${ok ? '✔' : '✘'} ${step.name} (${seconds}s)${r.error ? ` — ${r.error.message}` : ''}`);
  if (!ok) {
    process.stdout.write(`\n${summary.join('\n')}\nverify FAILED at "${step.name}"\n`);
    process.exit(r.status || 1);
  }
}

process.stdout.write(`\n${summary.join('\n')}\nverify passed (${steps.length} steps)\n`);
