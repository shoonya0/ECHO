// Compares two snapshots written by run.mjs and prints every step whose
// status, message, error code, or response shape changed.
//
// Usage: node diff.mjs before.json after.json

import { readFileSync } from "node:fs";

const [a, b] = process.argv.slice(2).map((f) => JSON.parse(readFileSync(f, "utf8")));
let changes = 0;

// Map keys that are MongoDB ObjectIds (e.g. chat participants) differ every run.
const normalize = (v) => JSON.stringify(v)?.replace(/"[0-9a-f]{24}":/g, '"<id>":');

for (const name of new Set([...Object.keys(a), ...Object.keys(b)])) {
  const before = normalize(a[name]);
  const after = normalize(b[name]);
  if (before === after) continue;
  changes++;
  console.log(`\n~ ${name}`);
  console.log(`  before: ${before ?? "(missing)"}`);
  console.log(`  after:  ${after ?? "(missing)"}`);
}

console.log(changes ? `\n${changes} step(s) differ` : "No contract differences.");
process.exit(changes ? 1 : 0);
