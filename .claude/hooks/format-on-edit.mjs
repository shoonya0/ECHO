// PostToolUse hook (matcher: Edit|Write)
//
// Formats the Go file the agent just edited with gofmt, so diffs stay free of style
// noise without the agent spending tokens on formatting.
//
// - No-op for non-Go files, or when gofmt isn't on PATH.
// - Spawns gofmt directly (no shell), so it behaves the same on Windows and macOS.
// - On a gofmt failure (usually a syntax error) it exits 2: for PostToolUse that
//   shows stderr to the agent (the edit already happened), so it can fix the syntax.

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
  process.exit(0);
}

const target = input.tool_input?.file_path;
if (!target) process.exit(0);

const projectDir = process.env.CLAUDE_PROJECT_DIR || input.cwd || process.cwd();
const file = path.resolve(projectDir, target);
if (path.extname(file).toLowerCase() !== '.go' || !existsSync(file)) process.exit(0);

const result = spawnSync('gofmt', ['-w', file], {
  cwd: projectDir,
  encoding: 'utf8',
  timeout: 25_000,
  windowsHide: true,
});

if (result.error) process.exit(0); // gofmt not installed: nothing to do

if (result.status !== 0) {
  const output = `${result.stderr ?? ''}${result.stdout ?? ''}`.trim();
  process.stderr.write(`gofmt could not format ${path.relative(projectDir, file)}:\n${output.slice(-2000)}\n`);
  process.exit(2);
}

process.exit(0);
