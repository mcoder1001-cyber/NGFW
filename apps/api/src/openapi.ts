// Writes the OpenAPI document to the path given as argv[2]. Used by `pnpm gen` in packages/api-client.
// Builds the whole application offline: no database, Valkey or agent connection is opened (all lazy).
import { writeFileSync } from 'node:fs';
import { buildOpenApi, createApp } from './app.js';
import { loadEnv } from './config.js';

const out = process.argv[2];
if (!out) throw new Error('usage: node dist/openapi.js <output.json>');
const env = loadEnv({});
const app = await createApp({ env, logger: ['error'] });
await app.init();
writeFileSync(out, JSON.stringify(buildOpenApi(app), null, 2) + '\n');
await app.close();
console.log(`api: OpenAPI written to ${out}`);
