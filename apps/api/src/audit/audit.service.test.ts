import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Db } from '../db/db.js';
import { AGGREGATE_MAX_KEYS, AuditService, type AuditEntry } from './audit.service.js';
import type { SystemEventsService } from './system-events.service.js';

/** TD-10b (review 2.3e): aggregated rows, the failure counter + system_event, the write-ahead row. */
function fakeDb(fail: () => boolean) {
  const rows: Record<string, unknown>[] = [];
  const updates: Record<string, unknown>[] = [];
  const db = {
    insert: () => ({
      values: (v: Record<string, unknown>) => {
        const p = fail()
          ? Promise.reject(Object.assign(new Error('insert refused'), { code: '23514' }))
          : Promise.resolve().then(() => void rows.push(v));
        return Object.assign(p, {
          returning: () => p.then(() => [{ id: rows.length }]),
        });
      },
    }),
    update: () => ({
      set: (v: Record<string, unknown>) => ({
        where: () =>
          fail()
            ? Promise.reject(new Error('update refused'))
            : (updates.push(v), Promise.resolve()),
      }),
    }),
  } as unknown as Db;
  return { db, rows, updates };
}

function fakeEvents() {
  const events: { code: string; data: unknown }[] = [];
  const svc = {
    record: async (_s: string, _sub: string, code: string, _m: string, data?: unknown) =>
      void events.push({ code, data }),
  } as unknown as SystemEventsService;
  return { svc, events };
}

const entry = (over: Partial<AuditEntry> = {}): AuditEntry => ({
  userId: null,
  username: null,
  sourceIp: '198.51.100.7',
  action: 'POST /api/v1/config/commit',
  resource: '/api/v1/config/commit',
  after: { reason: 'no-credentials' },
  result: 'failure',
  status: 401,
  ...over,
});

describe('AuditService (TD-10b)', () => {
  afterEach(() => vi.useRealTimers());

  it('writeAggregated: one row at the 1st, 10th, 100th … occurrence per key and minute', async () => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-09-25T00:10:30Z'));
    const { db, rows } = fakeDb(() => false);
    const a = new AuditService(db, fakeEvents().svc);
    for (let i = 0; i < 150; i++) await a.writeAggregated(entry(), 'k1');
    expect(
      rows.map((r) => (r['after'] as { aggregated: { count: number } }).aggregated.count),
    ).toEqual([1, 10, 100]);
    expect(rows[0]!['after']).toEqual({
      reason: 'no-credentials',
      aggregated: { window: '2026-09-25T00:10:00.000Z', count: 1 },
    });
    // a new minute starts a new count
    vi.setSystemTime(new Date('2026-09-25T00:11:01Z'));
    await a.writeAggregated(entry(), 'k1');
    expect(rows).toHaveLength(4);
  });

  it(`writeAggregated: past ${AGGREGATE_MAX_KEYS} keys a minute, new keys share one row per action (no address)`, async () => {
    const { db, rows } = fakeDb(() => false);
    const a = new AuditService(db, fakeEvents().svc);
    for (let i = 0; i < AGGREGATE_MAX_KEYS + 50; i++)
      await a.writeAggregated(entry({ sourceIp: `203.0.113.${i % 250}` }), `k${i}`);
    expect(rows).toHaveLength(AGGREGATE_MAX_KEYS + 2); // + the overflow row at 1 and 10
    expect(rows.at(-1)).toMatchObject({
      sourceIp: null,
      after: { aggregated: { count: 10, sources: 'many' } },
    });
  });

  it('a failed write is counted and reported as ONE system_event a minute (fail open: write() resolves)', async () => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-09-25T00:20:00Z'));
    const { db } = fakeDb(() => true);
    const ev = fakeEvents();
    const a = new AuditService(db, ev.svc);
    for (let i = 0; i < 5; i++) await expect(a.write(entry())).resolves.toBeUndefined();
    expect(a.writeFailures).toBe(5);
    expect(ev.events).toEqual([
      {
        code: 'AUDIT_WRITE_FAILED',
        data: {
          action: 'POST /api/v1/config/commit',
          code: '23514',
          failuresTotal: 1,
          foldedSinceLastEvent: 0,
        },
      },
    ]);
    vi.setSystemTime(new Date('2026-09-25T00:21:01Z'));
    await a.write(entry());
    expect(ev.events[1]).toMatchObject({ data: { failuresTotal: 6, foldedSinceLastEvent: 4 } });
  });

  it('begin (privileged write-ahead) throws when the row cannot be written; finish updates the outcome', async () => {
    let refuse = true;
    const { db, rows, updates } = fakeDb(() => refuse);
    const ev = fakeEvents();
    const a = new AuditService(db, ev.svc);
    await expect(a.begin(entry({ action: 'POST /api/v1/auth/api-keys' }))).rejects.toThrow(
      'insert refused',
    );
    expect(a.writeFailures).toBe(1);
    expect(ev.events).toHaveLength(1);
    refuse = false;
    const id = await a.begin(entry({ action: 'POST /api/v1/auth/api-keys' }));
    expect(rows[0]).toMatchObject({ result: 'failure', status: null, after: { incomplete: true } });
    await a.finish(id, entry({ result: 'success', status: 201, after: { name: 'k' } }));
    expect(updates[0]).toMatchObject({ result: 'success', status: 201, after: { name: 'k' } });
  });
});
