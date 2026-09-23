// Writes the OpenAPI document to the path given as argv[2]. Used by `pnpm gen` in packages/api-client.
import { writeFileSync } from 'node:fs';
import { createApp, buildOpenApi } from './app.js';

const out = process.argv[2];
if (!out) throw new Error('usage: node dist/openapi.js <output.json>');
const app = await createApp();
await app.init();
writeFileSync(out, JSON.stringify(buildOpenApi(app), null, 2) + '\n');
await app.close();
console.log(`api: OpenAPI written to ${out}`);
