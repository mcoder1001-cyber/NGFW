#!/usr/bin/env node
// Review P07a M1: a production build must contain neither the /dev/* demo routes, the "Developer" navigation entries,
// nor the demo page chunks. Run after `vite build` (production mode). A build made with VITE_VRX_DEV_ROUTES=1 is exempt.
import { readdirSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

if (process.env.VITE_VRX_DEV_ROUTES === '1') {
  console.log('check-no-dev-routes: skipped (VITE_VRX_DEV_ROUTES=1 build)');
  process.exit(0);
}
// Optional argument: the dist directory to check (default apps/web/dist).
const dist = process.argv[2] ?? join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const NEEDLES = ['dev/schema-form', 'dev/data-grid', 'dev/stream', 'dev:nav.', 'nav:devSchemaForm', 'SchemaFormDemoPage', 'DataGridDemoPage', 'StreamDemoPage', 'Developer demo'];
const files = ['index.html', ...readdirSync(join(dist, 'assets')).filter((f) => /\.(js|css|html)$/.test(f)).map((f) => `assets/${f}`)];
const hits = [];
for (const f of files) {
  if (/DemoPage/.test(f)) hits.push(`${f}: demo chunk emitted`);
  const text = readFileSync(join(dist, f), 'utf8');
  for (const n of NEEDLES) if (text.includes(n)) hits.push(`${f}: contains "${n}"`);
}
if (hits.length > 0) {
  console.error('check-no-dev-routes: FAIL — developer demo routes leaked into the production build');
  for (const h of hits) console.error(`  ${h}`);
  process.exit(1);
}
console.log(`check-no-dev-routes: OK (${files.length} files, no /dev routes, dev nav or demo chunks)`);
