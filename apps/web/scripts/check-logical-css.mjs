#!/usr/bin/env node
// Fails when physical (left/right) CSS is used in the web app or ui-kit sources. RTL must come for free from
// logical properties (docs/05-ui-spec.md); MUI's own styles are mirrored by stylis-plugin-rtl.
// Escape hatch for a justified exception: put `logical-css-ignore` in a comment on the same line.
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { dirname, join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const repo = join(here, '..', '..', '..');
const roots = [join(repo, 'apps/web/src'), join(repo, 'packages/ui-kit/src')];

const RULES = [
  [/\b(margin|padding|border|inset|scrollMargin|scrollPadding)(Left|Right)\b/, 'use *InlineStart / *InlineEnd'],
  [/\b(margin|padding|border|inset|scroll-margin|scroll-padding)-(left|right)\b/, 'use *-inline-start / *-inline-end'],
  [/(?<![\w-])(ml|mr|pl|pr)\s*:/, 'use marginInlineStart/End (ms/me) or paddingInlineStart/End (ps/pe)'],
  [/\btextAlign:\s*['"](left|right)['"]/, "use textAlign: 'start' | 'end'"],
  [/\btext-align:\s*(left|right)\b/, 'use text-align: start | end'],
  [/\bfloat:\s*['"]?(left|right)\b/, 'avoid float; use flex with logical alignment'],
  [/(?<![\w-])(left|right):\s*['"]?[-\d]/, 'use insetInlineStart / insetInlineEnd'],
];

const files = [];
function walk(dir) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) walk(p);
    else if (/\.(tsx?|css|html)$/.test(name) && !name.endsWith('.d.ts')) files.push(p);
  }
}
for (const r of roots) walk(r);

const problems = [];
for (const file of files) {
  const lines = readFileSync(file, 'utf8').split('\n');
  lines.forEach((line, i) => {
    if (line.includes('logical-css-ignore')) return;
    for (const [re, hint] of RULES) {
      if (re.test(line)) problems.push(`${relative(repo, file)}:${i + 1}: ${line.trim()}  → ${hint}`);
    }
  });
}
if (problems.length) {
  console.error(`check-logical-css: ${problems.length} physical CSS usage(s) found:`);
  for (const p of problems) console.error('  ' + p);
  process.exit(1);
}
console.log(`check-logical-css: OK (${files.length} files, no physical left/right CSS)`);
