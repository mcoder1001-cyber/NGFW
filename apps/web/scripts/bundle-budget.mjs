#!/usr/bin/env node
// Enforces docs/05-ui-spec.md: initial JS+CSS < 600 kB gzipped, routes code-split. Run after `vite build`.
import { readdirSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { gzipSync } from 'node:zlib';

const BUDGET = 600 * 1024;
const dist = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const html = readFileSync(join(dist, 'index.html'), 'utf8');
const initial = new Set();
for (const m of html.matchAll(/<script[^>]+type="module"[^>]+src="\/?([^"]+)"/g)) initial.add(m[1]);
for (const m of html.matchAll(/<link[^>]+rel="modulepreload"[^>]+href="\/?([^"]+)"/g)) initial.add(m[1]);
for (const m of html.matchAll(/<link[^>]+rel="stylesheet"[^>]+href="\/?([^"]+)"/g)) initial.add(m[1]);

const assets = readdirSync(join(dist, 'assets')).filter((f) => /\.(js|css)$/.test(f));
const rows = assets.map((f) => {
  const buf = readFileSync(join(dist, 'assets', f));
  return { file: `assets/${f}`, raw: buf.length, gz: gzipSync(buf, { level: 9 }).length, initial: initial.has(`assets/${f}`) };
});
rows.sort((a, b) => Number(b.initial) - Number(a.initial) || b.gz - a.gz);
const kb = (n) => (n / 1024).toFixed(1).padStart(7) + ' kB';
console.log('bundle-budget: gzipped sizes (I = loaded on first paint, L = lazy route/vendor chunk)');
for (const r of rows) console.log(`  ${r.initial ? 'I' : 'L'}  ${kb(r.gz)}  (raw ${kb(r.raw)})  ${r.file}`);
const total = rows.filter((r) => r.initial).reduce((s, r) => s + r.gz, 0);
const lazy = rows.filter((r) => !r.initial).length;
console.log(`bundle-budget: initial ${kb(total)} gzipped of ${kb(BUDGET)} budget; ${lazy} lazy chunk(s)`);
if (total > BUDGET) {
  console.error('bundle-budget: FAIL — initial bundle exceeds the 600 kB gzipped budget');
  process.exit(1);
}
if (lazy === 0) {
  console.error('bundle-budget: FAIL — no lazy chunks; routes must be code-split');
  process.exit(1);
}
console.log('bundle-budget: OK');
