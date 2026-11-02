// PreToolUse hook (matcher: Bash|PowerShell)
//
// Enforces the git rules in CLAUDE.md:
//   1. No `git commit` on main/master or the remote's default branch. Allowed: the
//      first commit of a new repo, and concluding a merge (MERGE_HEAD present).
//   2. Commits are authored by the GitHub user: user.name = <login> and
//      user.email = <id>+<login>@users.noreply.github.com (GitHub's private email).
//      Not on GitHub? Set CLAUDE_GIT_IDENTITY=off in settings `env`.
//   3. No AI watermark (Co-Authored-By, "Generated with Claude Code", session links,
//      🤖) in commit messages or in `gh pr|issue create|edit|comment` text.
//
// Exit code 2 blocks the call; stderr is shown to the agent.
// This is a backstop for accidents, not a sandbox: regexes can be bypassed, and a
// message passed by file (-F, --body-file) isn't inspected.

import { execFileSync } from 'node:child_process';
import { existsSync } from 'node:fs';
import path from 'node:path';

const WATERMARKS = [
  /co-authored-by\s*:/i,
  /generated\s+(with|by)\s+\[?claude/i,
  /claude\.(com\/claude-code|ai\/code)/i,
  /claude-session\s*:/i,
  /\u{1F916}/u, // 🤖
];

const ARG = String.raw`(?:"[^"]*"|'[^']*'|\S+)`;
const GIT_COMMIT = new RegExp(String.raw`\bgit(?:\s+(?:-[Cc]\s+${ARG}|--[\w-]+(?:=\S+)?))*\s+commit\b`, 'i');
const GH_TEXT = /\bgh\s+(pr|issue)\s+(create|edit|comment|review|merge)\b/i;

function readStdin() {
  return new Promise((resolve) => {
    let raw = '';
    process.stdin.setEncoding('utf8');
    process.stdin.on('data', (chunk) => (raw += chunk));
    process.stdin.on('end', () => resolve(raw));
    process.stdin.on('error', () => resolve(raw));
  });
}

function git(args, cwd) {
  try {
    return execFileSync('git', args, { cwd, encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
  } catch {
    return '';
  }
}

function block(why, command) {
  process.stderr.write(`Blocked by .claude/hooks/git-guard.mjs: ${why}\nCommand: ${command.slice(0, 200)}\n`);
  process.exit(2);
}

function unquote(s) {
  return s.replace(/^["']|["']$/g, '');
}

const raw = await readStdin();
let input = {};
try {
  input = JSON.parse(raw || '{}');
} catch {
  process.exit(0);
}

const command = String(input.tool_input?.command ?? '');
const isCommit = GIT_COMMIT.test(command);
if (!isCommit && !GH_TEXT.test(command)) process.exit(0);

// 3. AI watermark
const mark = WATERMARKS.find((re) => re.test(command));
if (mark) {
  block('remove the AI watermark/attribution (Co-Authored-By, "Generated with Claude Code", session link, 🤖). Work is attributed to the GitHub user only.', command);
}
if (!isCommit) process.exit(0);

// Which repo does the commit run in: `git -C <dir>`, a leading `cd <dir>`, or the session cwd.
let cwd = input.cwd || process.cwd();
const dashC = command.match(new RegExp(String.raw`\bgit\s+-C\s+(${ARG})`));
const cd = command.match(new RegExp(String.raw`^\s*(?:cd|Set-Location)\s+(${ARG})\s*(?:&&|;)`, 'i'));
const target = dashC || cd;
if (target) cwd = path.resolve(cwd, unquote(target[1]));
if (!existsSync(cwd)) process.exit(0);

const branch = git(['branch', '--show-current'], cwd);
if (!branch) process.exit(0); // not a repo, or detached HEAD

// 2. Identity = the GitHub user
if (process.env.CLAUDE_GIT_IDENTITY !== 'off') {
  const name = git(['config', 'user.name'], cwd);
  const email = git(['config', 'user.email'], cwd);
  const m = email.match(/^(\d+)\+([^@]+)@users\.noreply\.github\.com$/i);
  if (!m || name.toLowerCase() !== m[2].toLowerCase()) {
    block(
      `commit author must be the GitHub user with their private noreply email (now: "${name}" <${email}>).\n` +
        'Fix once per machine (ask the user first if unsure):\n' +
        "  gh api user --jq '.login, .id'\n" +
        '  git config --global user.name  "<login>"\n' +
        '  git config --global user.email "<id>+<login>@users.noreply.github.com"\n' +
        'Not using GitHub? Set CLAUDE_GIT_IDENTITY=off in .claude/settings.json env.',
      command,
    );
  }
}

// 1. Never commit on a protected branch
const protectedBranches = new Set(['main', 'master']);
const originHead = git(['symbolic-ref', '--short', 'refs/remotes/origin/HEAD'], cwd); // e.g. origin/main
if (originHead) protectedBranches.add(originHead.replace(/^origin\//, ''));

if (protectedBranches.has(branch)) {
  const hasCommits = git(['rev-parse', '--verify', '--quiet', 'HEAD'], cwd) !== '';
  const gitDir = git(['rev-parse', '--absolute-git-dir'], cwd);
  const merging = gitDir && existsSync(path.join(gitDir, 'MERGE_HEAD'));
  if (hasCommits && !merging) {
    block(
      `never commit directly on '${branch}'. Create a branch first, e.g. ` +
        '`git switch -c feature/<name>` (or fix/, refactor/, docs/, chore/).',
      command,
    );
  }
}

process.exit(0);
