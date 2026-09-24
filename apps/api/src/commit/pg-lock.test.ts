import { DesiredState } from '@ngfw/proto';
import { mkdtempSync, rmSync } from 'node:fs';
import net from 'node:net';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import pg from 'pg';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { AgentClient } from '../agent/agent.client.js';
import type { AuditService } from '../audit/audit.service.js';
import type { SystemEventsService } from '../audit/system-events.service.js';
import type { TokensService } from '../auth/tokens.service.js';
import { CommitLock } from '../common/mutex.js';
import { DatastoreService } from '../datastore/datastore.service.js';
import { emptyDocument } from '../datastore/documents.js';
import { Bus } from '../infra/bus.js';
import { FakeAgent } from '../testing/fake-agent.js';
import { ADMIN, TEST_HASH, testEnv } from '../testing/fixtures.js';
import { MemoryConfigRepo } from '../testing/memory-repo.js';
import { CommitService } from './commit.service.js';
import { PgAdvisoryLock } from './pg-lock.js';
import { ValidationService } from './validation.service.js';

/**
 * TD-10a fix round 1, review H1: the advisory lock is held on a pool client that stays checked out for a whole section
 * (up to the agent Apply). A PostgreSQL restart or a dropped connection in that window must not crash vrx-api: the
 * section finishes, the client is destroyed, and the loss is reported. No host database: a minimal PostgreSQL wire
 * server that answers every query with one row `ok = t` and drops the connection `dropAfterMs` after its first query.
 */
function fakePostgres(dropAfterMs: number): Promise<{ port: number; close(): Promise<void> }> {
  const msg = (t: string, body: Buffer) => {
    const b = Buffer.alloc(5 + body.length);
    b.write(t, 0);
    b.writeInt32BE(4 + body.length, 1);
    body.copy(b, 5);
    return b;
  };
  const ready = () => msg('Z', Buffer.from('I'));
  const rowDescription = () => {
    const name = Buffer.from('ok\0');
    const f = Buffer.alloc(18);
    f.writeInt32BE(0, 0); // table oid
    f.writeInt16BE(0, 4); // column
    f.writeInt32BE(16, 6); // bool
    f.writeInt16BE(1, 10); // typlen
    f.writeInt32BE(-1, 12); // typmod
    f.writeInt16BE(0, 16); // text format
    return msg('T', Buffer.concat([Buffer.from([0, 1]), name, f]));
  };
  const dataRow = () => msg('D', Buffer.from([0, 1, 0, 0, 0, 1, 't'.charCodeAt(0)]));
  const done = () => msg('C', Buffer.from('SELECT 1\0'));
  const sockets = new Set<net.Socket>();
  const server = net.createServer((s) => {
    sockets.add(s);
    s.on('close', () => sockets.delete(s));
    let started = false;
    let dropArmed = false;
    let buf = Buffer.alloc(0);
    s.on('data', (d) => {
      buf = Buffer.concat([buf, d]);
      for (;;) {
        if (!started) {
          if (buf.length < 4) return;
          const len = buf.readInt32BE(0);
          if (buf.length < len) return;
          buf = buf.subarray(len);
          started = true;
          s.write(Buffer.concat([msg('R', Buffer.alloc(4)), ready()]));
          continue;
        }
        if (buf.length < 5) return;
        const len = buf.readInt32BE(1);
        if (buf.length < 1 + len) return;
        const t = String.fromCharCode(buf[0]!);
        buf = buf.subarray(1 + len);
        if (t === 'Q') s.write(Buffer.concat([rowDescription(), dataRow(), done(), ready()]));
        if (t === 'S') {
          const parse = msg('1', Buffer.alloc(0));
          const bind = msg('2', Buffer.alloc(0));
          s.write(Buffer.concat([parse, bind, rowDescription(), dataRow(), done(), ready()]));
        }
        if ((t === 'Q' || t === 'S') && !dropArmed) {
          dropArmed = true;
          setTimeout(() => s.destroy(), dropAfterMs);
        }
      }
    });
    s.on('error', () => undefined);
  });
  return new Promise((resolve) =>
    server.listen(0, '127.0.0.1', () =>
      resolve({
        port: (server.address() as net.AddressInfo).port,
        close: () =>
          new Promise<void>((r) => {
            for (const s of sockets) s.destroy();
            server.close(() => r());
          }),
      }),
    ),
  );
}

function poolFor(port: number): pg.Pool {
  const pool = new pg.Pool({ host: '127.0.0.1', port, user: 'x', database: 'x', max: 3 });
  pool.on('error', () => undefined); // as apps/api/src/db/db.ts does for idle clients
  return pool;
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

describe('H1: losing the lock connection mid-section', { timeout: 20_000 }, () => {
  const uncaught: unknown[] = [];
  const onUncaught = (e: unknown) => uncaught.push(e);
  beforeAll(() => {
    process.prependListener('uncaughtException', onUncaught);
  });
  afterAll(() => {
    process.removeListener('uncaughtException', onUncaught);
  });

  for (const mode of ['run', 'tryRun'] as const) {
    it(`${mode}: the process survives, the section finishes, the loss is reported, the next section works`, async () => {
      const db = await fakePostgres(300);
      const pool = poolFor(db.port);
      const lost: Error[] = [];
      const lock = new (
        CommitLock as unknown as new (
          cross: PgAdvisoryLock,
          opts: { onLost: (e: Error) => void },
        ) => CommitLock
      )(new PgAdvisoryLock(pool), { onLost: (e) => lost.push(e) });
      try {
        const section = async () => {
          await sleep(900); // the agent Apply: the server drops the connection meanwhile
          return 'done';
        };
        const r = await (mode === 'run' ? lock.run(section) : lock.tryRun(section, 1000));
        expect(r).toBe('done');
        await sleep(50);
        expect(uncaught).toEqual([]);
        expect(lost).toHaveLength(1);
        expect(lost[0]).toBeInstanceOf(Error);
        // the dead client was destroyed, not returned to the pool: a new section gets a fresh connection
        expect(await lock.run(async () => 'again')).toBe('again');
        expect(lost).toHaveLength(1);
      } finally {
        await pool.end().catch(() => undefined);
        await db.close();
      }
    });
  }

  it('CommitService: a commit whose lock connection drops completes; running is marked UNKNOWN and reconciled', async () => {
    const dir = mkdtempSync(join(tmpdir(), 'vrx-td10a-h1-'));
    const socket = join(dir, 'agent.sock');
    const env = testEnv({
      VRX_AGENT_SOCKET: socket,
      VRX_AGENT_OWNER: 'w1',
      VRX_AGENT_TIMEOUT_MS: '5000',
    });
    const fake = new FakeAgent({ owner: 'w1' });
    await fake.start(socket);
    fake.reset(
      DesiredState.toJSON(ValidationService.desiredState(emptyDocument())) as Record<
        string,
        unknown
      >,
    );
    const agent = new AgentClient(env);
    const db = await fakePostgres(300);
    const pool = poolFor(db.port);
    const repo = new MemoryConfigRepo();
    repo.addUser('admin', 'admin', TEST_HASH);
    const events = { record: vi.fn(async (..._a: unknown[]) => undefined) };
    const commits = new CommitService(
      repo,
      new ValidationService(repo, agent),
      agent,
      events as unknown as SystemEventsService,
      new Bus(),
      env,
      { revokeUser: vi.fn() } as unknown as TokensService,
      { write: vi.fn() } as unknown as AuditService,
      { $client: pool } as never, // the Drizzle database's pg.Pool: PgAdvisoryLock on it
    );
    try {
      const ds = new DatastoreService(repo, env);
      await ds.patchCandidate(ADMIN, '/interfaces/loop1', { ipv4: ['10.1.0.1/24'] });
      fake.applyDelayMs = 900; // the lock connection drops 300 ms into the section
      const r = await commits.commit(ADMIN, {});
      expect(r.status).toBe('applied');
      expect(uncaught).toEqual([]);
      await vi.waitFor(
        () => expect(events.record.mock.calls.map((c) => c[2])).toContain('COMMIT_LOCK_LOST'),
        { timeout: 3000, interval: 50 },
      );
      expect(events.record.mock.calls.map((c) => c[2])).toContain('RUNNING_UNKNOWN');
      fake.applyDelayMs = 0;
      await vi.waitFor(async () => expect((await commits.syncStatus()).state).toBe('in-sync'), {
        timeout: 8000,
        interval: 100,
      });
      expect(repo.state.revisions).toHaveLength(1);
      expect(fake.current['interfaces']).toMatchObject({ loop1: { ipv4: ['10.1.0.1/24'] } });
    } finally {
      commits.onApplicationShutdown();
      agent.close();
      await fake.stop();
      await pool.end().catch(() => undefined);
      await db.close();
      rmSync(dir, { recursive: true, force: true });
    }
  });
});
