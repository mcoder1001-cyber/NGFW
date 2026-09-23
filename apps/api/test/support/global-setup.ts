import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { Valkey } from 'iovalkey';
import type { TestProject } from 'vitest/node';

/**
 * e2e bootstrap on the shared host (docs/lab/shared-host-rules.md): the slot's own database via
 * `deploy/dev/pg-test.sh create <prefix>` (role + db `vrx_<prefix>`, password only in /run/vrx-test/<prefix>/pg.env),
 * dropped again at the end; Valkey = the slot's logical db, keys under `vrx:<prefix>:e2e:` (deleted by pattern at the
 * end — never FLUSHALL/FLUSHDB). Set VRX_E2E_KEEP_DB=1 to keep the database for debugging.
 */
declare module 'vitest' {
  export interface ProvidedContext {
    pgDsn: string;
    prefix: string;
    valkeyDb: number;
    runDir: string;
  }
}

const REPO = resolve(dirname(fileURLToPath(import.meta.url)), '../../../..');
const PG_TEST = resolve(REPO, 'deploy/dev/pg-test.sh');

export default async function setup(project: TestProject) {
  const prefix = process.env['VRX_TEST_PREFIX'];
  if (!prefix || !/^[a-z][a-z0-9]{0,5}$/.test(prefix)) {
    throw new Error(
      'VRX_TEST_PREFIX (the slot prefix, e.g. w1 — eval "$(tools/lab env <slot>)") is required for e2e tests',
    );
  }
  const slot = Number(/(\d+)$/.exec(prefix)?.[1] ?? '0');
  const valkeyDb = Number(process.env['VRX_VALKEY_DB'] ?? slot);
  const runDir = `/run/vrx-test/${prefix}`;
  execFileSync(PG_TEST, ['create', prefix], { stdio: ['ignore', 'inherit', 'inherit'] });
  const env = readFileSync(`${runDir}/pg.env`, 'utf8');
  const dsn = /^VRX_PG_DSN=(.+)$/m.exec(env)?.[1];
  if (!dsn) throw new Error(`no VRX_PG_DSN in ${runDir}/pg.env`);
  project.provide('pgDsn', dsn);
  project.provide('prefix', prefix);
  project.provide('valkeyDb', valkeyDb);
  project.provide('runDir', runDir);

  return async () => {
    const kv = new Valkey({ host: '127.0.0.1', port: 6379, db: valkeyDb, lazyConnect: true });
    try {
      await kv.connect();
      let cursor = '0';
      let deleted = 0;
      do {
        const [next, keys] = await kv.scan(cursor, 'MATCH', `vrx:${prefix}:e2e:*`, 'COUNT', 500);
        cursor = next;
        if (keys.length > 0) deleted += await kv.del(...keys);
      } while (cursor !== '0');
      console.log(
        `e2e teardown: deleted ${deleted} Valkey keys vrx:${prefix}:e2e:* in db ${valkeyDb}`,
      );
    } finally {
      kv.disconnect();
    }
    if (process.env['VRX_E2E_KEEP_DB'] !== '1') {
      execFileSync(PG_TEST, ['drop', prefix], { stdio: ['ignore', 'inherit', 'inherit'] });
    }
  };
}
