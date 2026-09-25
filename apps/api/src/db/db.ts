import { drizzle, type NodePgDatabase } from 'drizzle-orm/node-postgres';
import { migrate } from 'drizzle-orm/node-postgres/migrator';
import { fileURLToPath } from 'node:url';
import pg from 'pg';
import { databaseUrl, type Env } from '../config.js';
import { problems } from '../common/problem.js';
import * as schema from './schema.js';

export type Db = NodePgDatabase<typeof schema>;
export type DbTx = Parameters<Parameters<Db['transaction']>[0]>[0];

/** DI token of the Drizzle database. */
export const DB = Symbol('VRX_DB');

export interface DbHandle {
  db: Db;
  pool: pg.Pool | undefined;
  close(): Promise<void>;
}

/**
 * ARCH-11 (TD-15): how long a query waits for a pool client (or a new connection) before it fails. pg's default is
 * 0 = wait forever, so an exhausted pool (e.g. N concurrent edits each wanting a second client) hung every request
 * instead of failing one of them.
 */
export const POOL_CONNECT_TIMEOUT_MS = 10_000;

/**
 * Lazy: `pg.Pool` does not connect until the first query, so the OpenAPI generator and the route-guard test build the
 * whole application without a database. Without a DSN every query fails with 503 (never with a stack trace).
 */
export function createDb(env: Env): DbHandle {
  const url = databaseUrl(env);
  if (url === undefined) {
    const unavailable = new Proxy(
      {},
      {
        // only query entry points fail; Nest probes instances for lifecycle hooks and `then`
        get(_target, prop) {
          if (
            typeof prop === 'string' &&
            /^(select|insert|update|delete|transaction|execute|query|\$count)$/.test(prop)
          ) {
            throw problems.unavailable('no database configured (VRX_DATABASE_URL)');
          }
          return undefined;
        },
      },
    ) as Db;
    return { db: unavailable, pool: undefined, close: async () => undefined };
  }
  const pool = new pg.Pool({
    connectionString: url,
    max: env.VRX_DB_POOL_MAX,
    connectionTimeoutMillis: POOL_CONNECT_TIMEOUT_MS,
  });
  // idle-client errors (server restart) must not crash the process; the next query reconnects
  pool.on('error', () => undefined);
  return { db: drizzle(pool, { schema }), pool, close: () => pool.end() };
}

/** apps/api/migrations — the same relative location from src/db (vitest, tsx) and dist/db (node). */
export const MIGRATIONS_DIR = fileURLToPath(new URL('../../migrations', import.meta.url));

export async function runMigrations(db: Db): Promise<void> {
  await migrate(db, { migrationsFolder: MIGRATIONS_DIR });
}
