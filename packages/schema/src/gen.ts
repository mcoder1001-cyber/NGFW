import { mkdirSync, writeFileSync } from 'node:fs';
import { generateSchemas } from './generate.js';
import { ROOT_KEYS } from './index.js';

// One schema, three consumers: TS types (tsc), JSON Schema (UI forms), OpenAPI components (API).
// Writes dist/json-schema/{root,<key>}.json and dist/openapi-components.json. Output is deterministic.
const out = new URL('../dist/', import.meta.url);
mkdirSync(new URL('json-schema/', out), { recursive: true });

const write = (relative: string, value: unknown): void => {
  writeFileSync(new URL(relative, out), JSON.stringify(value, null, 2) + '\n');
};

const generated = generateSchemas();
write('json-schema/root.json', generated.root);
for (const key of ROOT_KEYS) write(`json-schema/${key}.json`, generated.domains[key]);
write('openapi-components.json', generated.openapiComponents);
console.log(`schema: wrote root + ${ROOT_KEYS.length} domain schemas + openapi components`);
