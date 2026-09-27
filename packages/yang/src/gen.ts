import { mkdirSync, writeFileSync } from 'node:fs';
import { generateModules } from './generate.js';

// F-restconf-yang: write the generated YANG modules to `generated/*.yang` (checked in; a golden test fails CI on
// drift). Runs as part of `pnpm gen` so a schema change regenerates the modules with the other generated files.
const out = new URL('../generated/', import.meta.url);
mkdirSync(out, { recursive: true });

const modules = generateModules();
for (const [name, text] of Object.entries(modules)) {
  writeFileSync(new URL(`${name}.yang`, out), text);
}
console.log(`yang: wrote ${Object.keys(modules).length} modules to generated/`);
