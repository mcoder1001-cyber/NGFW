import type { StatsBatch } from '@ngfw/proto';
import type { AlarmRule } from '@ngfw/schema';
import { getTableColumns, type SQL } from 'drizzle-orm';
import { PgDialect, type PgColumn } from 'drizzle-orm/pg-core';
import { EventEmitter } from 'node:events';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { SystemEventsService } from '../../audit/system-events.service.js';
import type { Env } from '../../config.js';
import type { Db } from '../../db/db.js';
import { alarm } from '../../db/schema.js';
import { Bus } from '../../infra/bus.js';
import { AlarmsService } from './alarms.service.js';
import { stateKey, type RuleState } from './engine.js';

/**
 * S-alarms-restart-rebuild (D-218): an API restart must not strand an `active` alarm row. `start()` rebuilds the
 * engine state from the active rows, so the first healthy sample clears the row and notifies once; rows whose rule
 * left the running config (or was disabled) are cleared once each, with a reason.
 */

// The webhook POST is a spy: the assertions count the notifications the service sends.
const deliver = vi.hoisted(() =>
  vi.fn<(url: string, body: unknown, opts: unknown) => Promise<void>>(async () => undefined),
);
vi.mock('./webhook.js', () => ({ deliver }));

// ---- in-memory `alarm` table ----------------------------------------------------------------------------------------

type AlarmRow = typeof alarm.$inferSelect;

const COLUMN_KEY = new Map(
  Object.entries(getTableColumns(alarm)).map(([key, col]) => [col.name, key as keyof AlarmRow]),
);
const dialect = new PgDialect();

function keyOf(col: PgColumn): keyof AlarmRow {
  const key = COLUMN_KEY.get(col.name);
  if (key === undefined) throw new Error(`fake db: unknown column ${col.name}`);
  return key;
}

/**
 * The conditions AlarmsService builds are conjunctions of `"alarm"."<column>" = $n`: render them with drizzle's own
 * dialect and evaluate them on the rows. Any other shape throws, so a query change cannot pass unnoticed.
 */
function matcher(cond: SQL): (r: AlarmRow) => boolean {
  const q = dialect.sqlToQuery(cond);
  const checks = q.sql
    .replace(/^\((.*)\)$/, '$1')
    .split(' and ')
    .map((term) => {
      const m = /^"alarm"\."(\w+)" = \$(\d+)$/.exec(term);
      const key = m?.[1] === undefined ? undefined : COLUMN_KEY.get(m[1]);
      if (m === null || key === undefined)
        throw new Error(`fake db: unsupported condition ${q.sql}`);
      const want = q.params[Number(m[2]) - 1];
      return (r: AlarmRow) => r[key] === want;
    });
  return (r) => checks.every((c) => c(r));
}

function project(r: AlarmRow, fields: Record<string, PgColumn>): Record<string, unknown> {
  return Object.fromEntries(Object.entries(fields).map(([out, col]) => [out, r[keyOf(col)]]));
}

/** The three Drizzle chains AlarmsService uses at start and on ingest (select, update…returning, insert…returning). */
function fakeDb(rows: AlarmRow[]) {
  const writes: ('update' | 'insert')[] = [];
  const onlyAlarm = (t: unknown) => {
    if (t !== alarm) throw new Error('fake db: only the alarm table is modelled');
  };
  const db = {
    select: (fields: Record<string, PgColumn>) => ({
      from: (t: unknown) => {
        onlyAlarm(t);
        return {
          where: async (cond: SQL) => rows.filter(matcher(cond)).map((r) => project(r, fields)),
        };
      },
    }),
    update: (t: unknown) => {
      onlyAlarm(t);
      return {
        set: (v: Partial<AlarmRow>) => ({
          where: (cond: SQL) => ({
            returning: async (fields: Record<string, PgColumn>) => {
              writes.push('update');
              const hit = rows.filter(matcher(cond));
              for (const r of hit) Object.assign(r, v);
              return hit.map((r) => project(r, fields));
            },
          }),
        }),
      };
    },
    insert: (t: unknown) => {
      onlyAlarm(t);
      return {
        values: (v: Partial<AlarmRow>) => ({
          onConflictDoNothing: () => ({
            returning: async (fields: Record<string, PgColumn>) => {
              writes.push('insert');
              // alarm_active_uq: at most one active row per (rule, instance)
              if (
                rows.some(
                  (r) => r.state === 'active' && r.rule === v.rule && r.instance === v.instance,
                )
              )
                return [];
              const r = { ...activeRow(rows.length + 1, v.rule ?? '', v.instance ?? ''), ...v };
              rows.push(r);
              return [project(r, fields)];
            },
          }),
        }),
      };
    },
  } as unknown as Db;
  return { db, rows, writes };
}

// ---- fixtures -------------------------------------------------------------------------------------------------------

const T0 = Date.parse('2026-10-01T10:00:00.000Z');
const NOW = T0 + 120_000;
const HOOK = 'http://127.0.0.1:9/hook'; // never contacted: deliver is a spy
const CPU_MSG = 'worker_cpu_percent = 97 (threshold 90)';

const cpuRule = (over: Partial<AlarmRule> = {}): AlarmRule => ({
  metric: 'worker_cpu_percent',
  op: 'gt',
  threshold: 90,
  forSec: 0,
  severity: 'warning',
  enabled: true,
  targets: ['ops'],
  ...over,
});

function activeRow(
  id: number,
  rule: string,
  instance: string,
  over: Partial<AlarmRow> = {},
): AlarmRow {
  return {
    id,
    rule,
    instance,
    metric: 'worker_cpu_percent',
    severity: 'warning',
    state: 'active',
    value: '97',
    threshold: '90',
    message: CPU_MSG,
    raisedAt: new Date(T0),
    clearedAt: null,
    ackedAt: null,
    ackedBy: null,
    ...over,
  };
}

const linkRow = (id: number, rule: string, instance: string): AlarmRow =>
  activeRow(id, rule, instance, {
    metric: 'interface_link_down',
    severity: 'critical',
    value: '1',
    threshold: '1',
    message: `interface_link_down on ${instance} = 1 (threshold 1)`,
  });

const cpuBatch = (pct: number, at: number): StatsBatch => ({
  ts: new Date(at),
  seq: '1',
  interfaceCounters: [],
  workerCpu: [
    {
      worker: 0,
      name: 'vpp_main',
      utilizationPct: pct,
      vectorsPerCall: 1,
      calls: '10',
      vectors: '10',
    },
  ],
  intervalMs: 2000,
});

const services: AlarmsService[] = [];
afterEach(() => {
  for (const s of services.splice(0)) s.onModuleDestroy();
  deliver.mockClear();
});

/** A (re)started API: AlarmsService over the table, a running config with `rules` + the `ops` webhook, a fake agent. */
function boot(db: Db, rules: Record<string, AlarmRule>, opts: { firstBatch?: StatsBatch } = {}) {
  const doc = {
    management: { alarms: { targets: { ops: { kind: 'webhook', url: HOOK } }, rules } },
  };
  const agent = {
    streamStats: vi.fn(() => {
      const s = Object.assign(new EventEmitter(), { cancel: vi.fn() });
      // the agent answers as soon as the stream opens: this sample must already meet the rebuilt state
      const first = opts.firstBatch;
      if (first) setImmediate(() => s.emit('data', first));
      return s;
    }),
  };
  const record = vi.fn<SystemEventsService['record']>(async () => undefined);
  const bus = new Bus();
  const published: unknown[] = [];
  bus.onPublish((m) => {
    if (m.topic === 'alarm.events') published.push(m.data);
  });
  const env = { NGFW_ALARM_STATS_INTERVAL_MS: 2000, NGFW_ALARM_WEBHOOK_TIMEOUT_MS: 5000 } as Env;
  const svc = new AlarmsService(
    env,
    { getRunning: async () => ({ revision: null, doc }) } as never,
    agent as never,
    { record } as unknown as SystemEventsService,
    bus,
    {} as never,
    db,
  );
  svc.now = () => NOW;
  services.push(svc);
  const cleared = () => record.mock.calls.filter((c) => c[2] === 'ALARM_CLEARED');
  return { svc, agent, record, published, cleared, doc };
}

// ---- tests ----------------------------------------------------------------------------------------------------------

describe('AlarmsService restart rebuild (S-alarms-restart-rebuild, D-218)', () => {
  it('start() rebuilds the raise state of every active row, keyed and shaped as evaluate keeps it', async () => {
    const { db, writes } = fakeDb([
      activeRow(1, 'cpu', ''),
      linkRow(2, 'wanlink', 'wan0'),
      activeRow(3, 'cpu', '', {
        state: 'cleared',
        raisedAt: new Date(T0 - 60_000),
        clearedAt: new Date(T0 - 30_000),
      }),
    ]);
    const { svc, record } = boot(db, {
      cpu: cpuRule(),
      wanlink: cpuRule({ metric: 'interface_link_down', op: 'ge', threshold: 1 }),
    });
    await svc.start();
    expect(svc['state']).toEqual(
      new Map<string, RuleState>([
        [stateKey('cpu', ''), { since: T0, raised: true }],
        [stateKey('wanlink', 'wan0'), { since: T0, raised: true }],
      ]),
    );
    expect(writes).toEqual([]); // both rules still exist: nothing is cleared at start
    expect(record).not.toHaveBeenCalled();
    expect(deliver).not.toHaveBeenCalled();
  });

  it('the first healthy sample after a restart clears the still-active row and sends exactly one cleared webhook', async () => {
    const { db, rows } = fakeDb([activeRow(1, 'cpu', '')]);
    const { svc, agent, published, cleared } = boot(
      db,
      { cpu: cpuRule() },
      { firstBatch: cpuBatch(1.5, T0 + 60_000) },
    );
    await svc.start();
    expect(agent.streamStats).toHaveBeenCalledTimes(1);
    await vi.waitFor(() => expect(rows[0]?.state).toBe('cleared'));
    expect(rows[0]?.clearedAt).toEqual(new Date(NOW));
    expect(deliver).toHaveBeenCalledTimes(1);
    expect(deliver).toHaveBeenCalledWith(
      HOOK,
      {
        kind: 'cleared',
        rule: 'cpu',
        instance: '',
        metric: 'worker_cpu_percent',
        severity: 'warning',
        message: CPU_MSG,
        at: new Date(NOW).toISOString(),
      },
      { timeoutMs: 5000 },
    );
    expect(cleared()).toEqual([
      ['info', 'alarms', 'ALARM_CLEARED', CPU_MSG, { rule: 'cpu', instance: '' }],
    ]);
    expect(published).toEqual([{ type: 'cleared', rule: 'cpu', instance: '' }]);

    // the next healthy sample has nothing left to clear: still exactly one webhook
    await svc.handleStatsBatch(cpuBatch(1.2, T0 + 62_000));
    expect(deliver).toHaveBeenCalledTimes(1);
    expect(cleared()).toHaveLength(1);
  });

  it('a still-violating sample after a restart raises nothing new; the row clears once the condition ends', async () => {
    const { db, rows, writes } = fakeDb([activeRow(1, 'cpu', '')]);
    const { svc, record } = boot(db, { cpu: cpuRule() });
    await svc.start();
    await svc.handleStatsBatch(cpuBatch(97, T0 + 60_000));
    expect(writes).toEqual([]); // no INSERT attempt: the engine knows the alarm is raised
    expect(record).not.toHaveBeenCalled(); // no ALARM_RAISED
    expect(deliver).not.toHaveBeenCalled();
    await svc.handleStatsBatch(cpuBatch(10, T0 + 62_000));
    expect(rows.map((r) => r.state)).toEqual(['cleared']);
    expect(writes).toEqual(['update']);
    expect(deliver).toHaveBeenCalledTimes(1);
    expect(deliver.mock.calls[0]?.[1]).toMatchObject({ kind: 'cleared', rule: 'cpu' });
  });

  it('rows whose rule was removed while the API was down are cleared once each, with a reason and no webhook', async () => {
    const { db, rows } = fakeDb([
      linkRow(1, 'gone', 'wan0'),
      linkRow(2, 'gone', 'wan1'),
      activeRow(3, 'cpu', ''),
    ]);
    const first = boot(db, { cpu: cpuRule() });
    await first.svc.start();
    expect(rows.map((r) => [r.rule, r.instance, r.state])).toEqual([
      ['gone', 'wan0', 'cleared'],
      ['gone', 'wan1', 'cleared'],
      ['cpu', '', 'active'], // its rule still exists: kept until a healthy sample
    ]);
    const why = ' — cleared: its rule is no longer configured';
    expect(first.cleared()).toEqual([
      [
        'info',
        'alarms',
        'ALARM_CLEARED',
        `interface_link_down on wan0 = 1 (threshold 1)${why}`,
        { rule: 'gone', instance: 'wan0', reason: 'rule-removed' },
      ],
      [
        'info',
        'alarms',
        'ALARM_CLEARED',
        `interface_link_down on wan1 = 1 (threshold 1)${why}`,
        { rule: 'gone', instance: 'wan1', reason: 'rule-removed' },
      ],
    ]);
    expect(first.published).toEqual([
      { type: 'cleared', rule: 'gone', instance: 'wan0', reason: 'rule-removed' },
      { type: 'cleared', rule: 'gone', instance: 'wan1', reason: 'rule-removed' },
    ]);
    expect(deliver).not.toHaveBeenCalled(); // a removed rule has no targets left: no webhook storm
    expect([...first.svc['state'].keys()]).toEqual([stateKey('cpu', '')]);

    // a later reload (any commit) and another restart find nothing more to clear: one `cleared` per row
    await first.svc.reload();
    const second = boot(db, { cpu: cpuRule() });
    await second.svc.start();
    expect(first.cleared()).toHaveLength(2);
    expect(second.record).not.toHaveBeenCalled();
    expect(deliver).not.toHaveBeenCalled();
    expect(rows.filter((r) => r.state === 'active').map((r) => r.rule)).toEqual(['cpu']);
  });

  it('a rule disabled while the API was down: its row clears once with reason rule-disabled, its target gets one webhook', async () => {
    const { db, rows } = fakeDb([activeRow(1, 'cpu', '')]);
    const { svc, cleared } = boot(db, { cpu: cpuRule({ enabled: false }) });
    await svc.start();
    expect(rows[0]?.state).toBe('cleared');
    expect(cleared()).toEqual([
      [
        'info',
        'alarms',
        'ALARM_CLEARED',
        `${CPU_MSG} — cleared: its rule is disabled`,
        { rule: 'cpu', instance: '', reason: 'rule-disabled' },
      ],
    ]);
    expect(deliver).toHaveBeenCalledTimes(1);
    expect(deliver.mock.calls[0]?.[1]).toMatchObject({
      kind: 'cleared',
      rule: 'cpu',
      instance: '',
      message: CPU_MSG,
      reason: 'rule-disabled',
    });
  });

  it('a rule removed by a commit while running still clears its raised alarm, now with reason rule-removed', async () => {
    const { db, rows } = fakeDb([]);
    const { svc, doc, cleared } = boot(db, { cpu: cpuRule() });
    await svc.start();
    await svc.handleStatsBatch(cpuBatch(97, T0 + 1_000));
    expect(rows.map((r) => [r.rule, r.state])).toEqual([['cpu', 'active']]);
    expect(deliver.mock.calls.map((c) => (c[1] as { kind: string }).kind)).toEqual(['raised']);
    doc.management.alarms.rules = {};
    await svc.reload();
    expect(rows[0]?.state).toBe('cleared');
    expect(cleared().map((c) => c[4])).toEqual([
      { rule: 'cpu', instance: '', reason: 'rule-removed' },
    ]);
    expect(deliver).toHaveBeenCalledTimes(1); // only the raise: the removed rule has no targets left
  });
});
