import { Inject, Injectable, Logger, type OnModuleDestroy } from '@nestjs/common';
import { ModuleRef } from '@nestjs/core';
import { EventKind, type Event, type StatsBatch } from '@ngfw/proto';
import { isPlainObject, type AlarmMetric, type AlarmRule, type AlarmTarget } from '@ngfw/schema';
import { and, desc, eq, sql } from 'drizzle-orm';
import { AgentClient } from '../../agent/agent.client.js';
import { SystemEventsService } from '../../audit/system-events.service.js';
import { ENV, type Env } from '../../config.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import { DB, type Db } from '../../db/db.js';
import { alarm, secret } from '../../db/schema.js';
import { Bus } from '../../infra/bus.js';
import { deliver } from './webhook.js';
import { evaluate, pruneRules, type RuleState, type Sample } from './engine.js';

type Json = Record<string, unknown>;

/** Previous interface counters for rate derivation (bytes/drops are absolute in StreamStats). */
interface Prev {
  ts: number;
  rxBytes: number;
  txBytes: number;
  drops: number;
}

/**
 * F-dashboard-prom-alarms alarm engine (API side). It keeps its own upstream StreamStats subscription (the relay's
 * stats stream runs only while a WS client listens) and reads link events off the bus, derives the metric samples,
 * runs the pure `evaluate`, and persists raise/clear transitions to the `alarm` table, notifying the rule's targets.
 * Rules come from `management.alarms` in the running config (reloaded on every commit). Buffer/node metrics are
 * Prometheus-scraped (agent exporter), not evaluated here — see docs.
 */
@Injectable()
export class AlarmsService implements OnModuleDestroy {
  private readonly log = new Logger('alarms');
  private state = new Map<string, RuleState>();
  private rules: Record<string, AlarmRule> = {};
  private targets: Record<string, AlarmTarget> = {};
  private prev = new Map<string, Prev>();
  private stats: { cancel(): void } | undefined;
  private statsTimer: NodeJS.Timeout | undefined;
  private backoff = 1000;
  private stopped = false;
  private readonly offBus: () => void;
  private readonly offCommit: () => void;
  /** Clock (tests move it). */
  now: () => number = () => Date.now();

  constructor(
    @Inject(ENV) private readonly env: Env,
    private readonly ds: DatastoreService,
    private readonly agent: AgentClient,
    private readonly events: SystemEventsService,
    private readonly bus: Bus,
    private readonly moduleRef: ModuleRef,
    @Inject(DB) private readonly db: Db,
  ) {
    this.offBus = this.bus.onAgentEvent((e) => this.onAgentEvent(e));
    this.offCommit = this.bus.onPublish((m) => {
      if (m.topic === 'commit.events') {
        const type = (m.data as { type?: string } | null)?.type;
        if (type === 'applied' || type === 'confirmed' || type === 'reverted') void this.reload();
      }
    });
  }

  /** Called by the module once the app is ready (see index.ts provider factory). */
  async start(): Promise<void> {
    await this.reload();
    this.openStats();
  }

  onModuleDestroy(): void {
    this.stopped = true;
    this.offBus();
    this.offCommit();
    if (this.statsTimer) clearTimeout(this.statsTimer);
    this.stats?.cancel();
  }

  /** Reload the rules/targets from the running configuration; prune state for rules that went away. */
  async reload(): Promise<void> {
    const { doc } = await this.ds.getRunning();
    const mgmt = isPlainObject(doc['management']) ? (doc['management'] as Json) : {};
    const alarms = isPlainObject(mgmt['alarms']) ? (mgmt['alarms'] as Json) : {};
    this.rules = (isPlainObject(alarms['rules']) ? alarms['rules'] : {}) as Record<
      string,
      AlarmRule
    >;
    this.targets = (isPlainObject(alarms['targets']) ? alarms['targets'] : {}) as Record<
      string,
      AlarmTarget
    >;
    const clears = pruneRules(this.rules, this.state);
    for (const c of clears) await this.persistClear(c.rule, c.instance);
  }

  // ---- ingestion ----------------------------------------------------------------------------------------------------

  private onAgentEvent(e: Event): void {
    if (e.kind !== EventKind.EVENT_KIND_LINK_UP && e.kind !== EventKind.EVENT_KIND_LINK_DOWN)
      return;
    const iface = e.interface ?? '';
    if (iface === '') return;
    void this.ingestSamples(
      [
        {
          metric: 'interface_link_down',
          instance: iface,
          value: e.kind === EventKind.EVENT_KIND_LINK_DOWN ? 1 : 0,
        },
      ],
      this.now(),
    );
  }

  /** Convert a StreamStats batch into rate samples and ingest them. */
  handleStatsBatch(b: StatsBatch): Promise<void> {
    const now = b.ts ? b.ts.getTime() : this.now();
    const samples: Sample[] = [];
    for (const c of b.interfaceCounters) {
      const p = this.prev.get(c.name);
      const cur: Prev = {
        ts: now,
        rxBytes: Number(c.rxBytes),
        txBytes: Number(c.txBytes),
        drops: Number(c.drops),
      };
      this.prev.set(c.name, cur);
      if (p === undefined || now <= p.ts) continue;
      const dt = (now - p.ts) / 1000;
      samples.push({
        metric: 'interface_rx_bps',
        instance: c.name,
        value: (Math.max(0, cur.rxBytes - p.rxBytes) * 8) / dt,
      });
      samples.push({
        metric: 'interface_tx_bps',
        instance: c.name,
        value: (Math.max(0, cur.txBytes - p.txBytes) * 8) / dt,
      });
      samples.push({
        metric: 'interface_rx_drops',
        instance: c.name,
        value: Math.max(0, cur.drops - p.drops) / dt,
      });
    }
    let worst = 0;
    for (const w of b.workerCpu) worst = Math.max(worst, w.utilizationPct);
    if (b.workerCpu.length > 0)
      samples.push({ metric: 'worker_cpu_percent', instance: '', value: worst });
    return this.ingestSamples(samples, now);
  }

  /** Evaluate `samples` and persist/notify the transitions (exposed for tests and the stream plumbing). */
  async ingestSamples(samples: Sample[], now: number): Promise<void> {
    const t = evaluate(this.rules, samples, now, this.state);
    for (const r of t.raises) {
      await this.persistRaise(r.rule, r.instance, r.metric, r.severity, r.value, r.threshold);
    }
    for (const c of t.clears) await this.persistClear(c.rule, c.instance);
  }

  // ---- persistence + notify ----------------------------------------------------------------------------------------

  private async persistRaise(
    rule: string,
    instance: string,
    metric: AlarmMetric,
    severity: string,
    value: number,
    threshold: number,
  ): Promise<void> {
    const message = `${metric}${instance ? ` on ${instance}` : ''} = ${round(value)} (threshold ${threshold})`;
    // idempotent: the partial unique index means one active row per (rule, instance)
    const rows = await this.db
      .insert(alarm)
      .values({
        rule,
        instance,
        metric,
        severity,
        state: 'active',
        value: String(round(value)),
        threshold: String(threshold),
        message,
        raisedAt: new Date(this.now()),
      })
      .onConflictDoNothing()
      .returning({ id: alarm.id });
    if (rows.length === 0) return; // already active
    await this.events.record(
      severity === 'critical' ? 'error' : 'warning',
      'alarms',
      'ALARM_RAISED',
      message,
      { rule, instance, metric },
    );
    this.bus.publish('alarm.events', { type: 'raised', rule, instance, metric, severity, message });
    await this.notify(rule, 'raised', {
      rule,
      instance,
      metric,
      severity,
      value: round(value),
      threshold,
      message,
    });
  }

  private async persistClear(rule: string, instance: string): Promise<void> {
    const rows = await this.db
      .update(alarm)
      .set({ state: 'cleared', clearedAt: new Date(this.now()) })
      .where(and(eq(alarm.rule, rule), eq(alarm.instance, instance), eq(alarm.state, 'active')))
      .returning({
        id: alarm.id,
        message: alarm.message,
        severity: alarm.severity,
        metric: alarm.metric,
      });
    const row = rows[0];
    if (row === undefined) return;
    await this.events.record('info', 'alarms', 'ALARM_CLEARED', row.message, { rule, instance });
    this.bus.publish('alarm.events', { type: 'cleared', rule, instance });
    await this.notify(rule, 'cleared', {
      rule,
      instance,
      metric: row.metric,
      severity: row.severity,
      message: row.message,
    });
  }

  private async notify(ruleName: string, kind: 'raised' | 'cleared', payload: Json): Promise<void> {
    const rule = this.rules[ruleName];
    if (rule === undefined) return;
    for (const name of rule.targets) {
      const target = this.targets[name];
      if (target === undefined || target.kind !== 'webhook') continue; // email handled by F-notifications later
      try {
        const token = target.secretRef ? await this.readSecret(target.secretRef) : undefined;
        await deliver(
          target.url,
          { kind, ...payload, at: new Date(this.now()).toISOString() },
          { ...(token ? { token } : {}), timeoutMs: this.env.NGFW_ALARM_WEBHOOK_TIMEOUT_MS },
        );
      } catch (e) {
        this.log.warn(`webhook '${name}' for alarm '${ruleName}' failed: ${String(e)}`);
        await this.events.record(
          'warning',
          'alarms',
          'ALARM_NOTIFY_FAILED',
          `notify '${name}' failed`,
          { rule: ruleName, target: name },
        );
      }
    }
  }

  private async readSecret(ref: string): Promise<string | null> {
    const [row] = await this.db
      .select({ ciphertext: secret.ciphertext })
      .from(secret)
      .where(eq(secret.ref, ref))
      .limit(1);
    if (!row) return null;
    const { SecretsService } = await import('../../secrets/secrets.service.js');
    // resolved lazily to avoid a provider cycle (like mgmt-tls)
    return this.moduleRef.get(SecretsService, { strict: false }).decrypt(row.ciphertext, ref);
  }

  // ---- reads --------------------------------------------------------------------------------------------------------

  async list(opts: { active?: boolean; page: number; pageSize: number }) {
    const where =
      opts.active === undefined ? undefined : eq(alarm.state, opts.active ? 'active' : 'cleared');
    const q = this.db
      .select()
      .from(alarm)
      .orderBy(desc(alarm.raisedAt))
      .limit(opts.pageSize)
      .offset((opts.page - 1) * opts.pageSize);
    const rows = await (where ? q.where(where) : q);
    const counted = await (where
      ? this.db
          .select({ n: sql<number>`count(*)::int` })
          .from(alarm)
          .where(where)
      : this.db.select({ n: sql<number>`count(*)::int` }).from(alarm));
    const n = counted[0]?.n ?? 0;
    return {
      total: n,
      items: rows.map((r) => ({
        id: r.id,
        rule: r.rule,
        instance: r.instance,
        metric: r.metric,
        severity: r.severity,
        state: r.state,
        value: r.value,
        threshold: r.threshold,
        message: r.message,
        raisedAt: r.raisedAt.toISOString(),
        clearedAt: r.clearedAt?.toISOString() ?? null,
        ackedAt: r.ackedAt?.toISOString() ?? null,
        ackedBy: r.ackedBy ?? null,
      })),
    };
  }

  async ack(id: number, user: string): Promise<boolean> {
    const rows = await this.db
      .update(alarm)
      .set({ ackedAt: new Date(this.now()), ackedBy: user })
      .where(and(eq(alarm.id, id), sql`${alarm.ackedAt} is null`))
      .returning({ id: alarm.id });
    return rows.length > 0;
  }

  /** Counts for the dashboard tile (active alarms by severity). */
  async summary() {
    const rows = await this.db
      .select({ severity: alarm.severity, n: sql<number>`count(*)::int` })
      .from(alarm)
      .where(eq(alarm.state, 'active'))
      .groupBy(alarm.severity);
    const bySeverity: Record<string, number> = { info: 0, warning: 0, critical: 0 };
    let active = 0;
    for (const r of rows) {
      bySeverity[r.severity] = r.n;
      active += r.n;
    }
    return { active, bySeverity };
  }

  // ---- stream plumbing --------------------------------------------------------------------------------------------

  private openStats(): void {
    if (this.stopped) return;
    const call = this.agent.streamStats({
      intervalMs: this.env.NGFW_ALARM_STATS_INTERVAL_MS,
      interfaces: [],
      includeWorkerCpu: true,
    });
    this.stats = call;
    call.on('data', (b: StatsBatch) => {
      this.backoff = 1000;
      void this.handleStatsBatch(b).catch((e: unknown) => this.log.warn(`ingest: ${String(e)}`));
    });
    const retry = () => {
      if (this.stats !== call || this.stopped) return;
      this.stats = undefined;
      const delay = this.backoff;
      this.backoff = Math.min(this.backoff * 2, 30_000);
      this.statsTimer = setTimeout(() => this.openStats(), delay);
      this.statsTimer.unref();
    };
    call.on('error', retry);
    call.on('end', retry);
  }
}

function round(v: number): number {
  return Math.round(v * 100) / 100;
}
