import type { Pool, PoolClient } from 'pg';
import type { Held, ProcessLock } from '../common/mutex.js';

/** Advisory-lock key of the commit engine: (0x56525841 'VRXA', 1). Every vrx-api process on one database uses it. */
const KEY = [0x56525841, 1];
const POLL_MS = 100;

/**
 * TD-10a (review 2.4b, D-111 follow-up): the commit engine's cross-process lock — a PostgreSQL SESSION advisory lock
 * held on a dedicated pool connection for the length of one section (commit, rollback, confirm, reconcile, password
 * set, secret delete). A second API process on the same database (tools/app next to a systemd unit, an overlap during
 * an upgrade) therefore serialises with this one instead of interleaving promotes and password writes. If the
 * connection dies, PostgreSQL drops the lock with the session.
 *
 * Review H1: the client stays CHECKED OUT for the whole section (up to the agent Apply), and pg-pool removes its idle
 * `error` listener on checkout — so this code keeps its own listener on the client from checkout to release. A
 * PostgreSQL restart or a dropped connection then marks the lock `lost()` (the section finishes; `CommitLock` reports
 * it) instead of an unhandled `error` event that would kill vrx-api. A lost client is destroyed, never re-pooled.
 */
export class PgAdvisoryLock implements ProcessLock {
  constructor(private readonly pool: Pool) {}

  async acquire(): Promise<Held> {
    const w = watched(await this.pool.connect());
    try {
      await w.client.query('select pg_advisory_lock($1, $2)', KEY);
    } catch (e) {
      w.drop(e as Error);
      throw e;
    }
    return w.held();
  }

  async tryAcquire(waitMs: number): Promise<Held | null> {
    const deadline = Date.now() + waitMs;
    // review L2: an exhausted pool must not stretch the wait past the budget either
    const c = await connectWithin(this.pool, waitMs);
    if (c === null) return null;
    const w = watched(c);
    try {
      for (;;) {
        const lost = w.lost();
        if (lost) throw lost;
        const r = await w.client.query<{ ok: boolean }>(
          'select pg_try_advisory_lock($1, $2) as ok',
          KEY,
        );
        if (r.rows[0]?.ok === true) return w.held();
        const left = deadline - Date.now();
        if (left <= 0) {
          w.free();
          return null;
        }
        await new Promise((res) => setTimeout(res, Math.min(POLL_MS, left)));
      }
    } catch (e) {
      w.drop(e as Error);
      throw e;
    }
  }
}

/** A checked-out client with our own `error` listener until it goes back (review H1). */
function watched(client: PoolClient) {
  let lost: Error | undefined;
  const onError = (e: Error) => {
    lost ??= e;
  };
  client.on('error', onError);
  let done = false;
  const giveBack = (err?: Error) => {
    if (done) return;
    done = true;
    // release first: pg-pool puts its own idle listener back (or destroys the client on `err`); only then drop ours,
    // so there is no moment without a listener
    client.release(err);
    client.removeListener('error', onError);
  };
  return {
    client,
    lost: () => lost,
    free: () => giveBack(),
    drop: (e: Error) => giveBack(e),
    held: (): Held => ({
      lost: () => lost,
      release: async () => {
        if (lost) return giveBack(lost); // the session — and with it the lock — is gone: destroy the client
        try {
          await client.query('select pg_advisory_unlock($1, $2)', KEY);
          giveBack();
        } catch (e) {
          lost ??= e as Error; // the connection died at the very end of the section
          giveBack(e as Error);
        }
      },
    }),
  };
}

/** `pool.connect()`, or null when no connection comes within `ms` (a late one is returned to the pool). */
async function connectWithin(pool: Pool, ms: number): Promise<PoolClient | null> {
  const p = pool.connect();
  let timer: NodeJS.Timeout | undefined;
  const timeout = new Promise<null>((res) => {
    timer = setTimeout(() => res(null), ms);
  });
  const c = await Promise.race([p, timeout]);
  clearTimeout(timer);
  if (c === null) {
    p.then(
      (late) => late.release(),
      () => undefined,
    );
  }
  return c;
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
