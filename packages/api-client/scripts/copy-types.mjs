// TD-2 #2: `tsc` never emits the generated `src/generated/schema.d.ts` (a declaration file is an input, not an
// output), but `dist/index.d.ts` imports it — without this copy every consumer sees `paths = any`.
// `--check` only verifies that the built package carries it (the test runs after the build).
import { copyFileSync, existsSync, mkdirSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const from = resolve(root, 'src/generated/schema.d.ts');
const to = resolve(root, 'dist/generated/schema.d.ts');

if (process.argv.includes('--check')) {
  if (!existsSync(to)) {
    console.error(`api-client: ${to} is missing — run the build (it copies the generated types)`);
    process.exit(1);
  }
  console.log('api-client: dist/generated/schema.d.ts present');
} else {
  mkdirSync(dirname(to), { recursive: true });
  copyFileSync(from, to);
  console.log('api-client: copied src/generated/schema.d.ts → dist/generated/schema.d.ts');
}
