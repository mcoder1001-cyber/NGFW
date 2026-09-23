import { mkdirSync, writeFileSync } from 'node:fs';
import { z } from 'zod';
import { RootConfig, ROOT_KEYS } from './index.js';

// One schema, three consumers: TS types (tsc), JSON Schema (UI forms), OpenAPI components (API).
const out = new URL('../dist/', import.meta.url);
mkdirSync(new URL('json-schema/', out), { recursive: true });

const root = z.toJSONSchema(RootConfig, { target: 'draft-2020-12', io: 'input' });
writeFileSync(new URL('json-schema/root.json', out), JSON.stringify(root, null, 2) + '\n');
for (const key of ROOT_KEYS) {
  const s = z.toJSONSchema(RootConfig.shape[key], { target: 'draft-2020-12', io: 'input' });
  writeFileSync(new URL(`json-schema/${key}.json`, out), JSON.stringify(s, null, 2) + '\n');
}
const components = { schemas: { RootConfig: root } };
writeFileSync(new URL('openapi-components.json', out), JSON.stringify(components, null, 2) + '\n');
console.log(`schema: wrote root + ${ROOT_KEYS.length} domain schemas`);
