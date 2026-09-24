import type { Pool, PoolClient } from 'pg';
import type { ProcessLock, Release } from '../common/mutex.js';

/** Advisory-lock key of the commit engine: (0x56525841 'VRXA', 1). Every vrx-api process on one database uses it. */
const KEY_SPACE = 0x56525841;
const KEY_COMMIT = 1;
const POLL_MS = 100;

/**
 * TD-10a (review 2.4b, D-111 follow-up): the commit engine's cross-process lock — a PostgreSQL SESSION advisory lock
 * held on a dedicated pool connection for the length of one section (commit, rollback, confirm, reconcile, password
 * set, secret delete). A second API process on the same database (tools/app next to a systemd unit, an overlap during
 * an upgrade) therefore serialises with this one instead of interleaving promotes and password writes. If the
 * connection dies, PostgreSQL drops the lock with the session.
 */
export class PgAdvisoryLock implements ProcessLock {
  constructor(private readonly pool: Pool) {}

  async acquire(): Promise<Release> {
    const c = await this.pool.connect();
    try {
      await c.query('select pg_advisory_lock($1, $2)', [KEY_SPACE, KEY_COMMIT]);
    } catch (e) {
      c.release(e as Error);
      throw e;
    }
    return releaser(c);
  }

  async tryAcquire(waitMs: number): Promise<Release | null> {
    const deadline = Date.now() + waitMs;
    const c = await this.pool.connect();
    try {
      for (;;) {
        const r = await c.query<{ ok: boolean }>('select pg_try_advisory_lock($1, $2) as ok', [
          KEY_SPACE,
          KEY_COMMIT,
        ]);
        if (r.rows[0]?.ok === true) return releaser(c);
        const left = deadline - Date.now();
        if (left <= 0) {
          c.release();
          return null;
        }
        await new Promise((res) => setTimeout(res, Math.min(POLL_MS, left)));
      }
    } catch (e) {
      c.release(e as Error);
      throw e;
    }
  }
}

function releaser(c: PoolClient): Release {
  return async () => {
    try {
      await c.query('select pg_advisory_unlock($1, $2)', [KEY_SPACE, KEY_COMMIT]);
      c.release();
    } catch (e) {
      // destroy the connection: the session, and with it the lock, ends
      c.release(e as Error);
    }
  };
}

/** The pg.Pool behind a Drizzle database, or undefined (no DSN configured: the in-process lock alone). */
export function poolOf(db: unknown): Pool | undefined {
  if (db === null || typeof db !== 'object') return undefined;
  const client = (db as { $client?: unknown }).$client;
  return client !== null &&
    typeof client === 'object' &&
    typeof (client as { connect?: unknown }).connect === 'function'
    ? (client as Pool)
    : undefined;
}
