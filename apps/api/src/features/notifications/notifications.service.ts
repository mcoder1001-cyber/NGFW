import {
  Inject,
  Injectable,
  Logger,
  type OnModuleDestroy,
  type OnApplicationBootstrap,
} from '@nestjs/common';
import { ModuleRef } from '@nestjs/core';
import { EventKind, type Event } from '@ngfw/proto';
import {
  NotificationsSchema,
  type NotificationsConfig,
  type NotificationChannel,
  isPlainObject,
} from '@ngfw/schema';
import { eq } from 'drizzle-orm';
import { DatastoreService } from '../../datastore/datastore.service.js';
import { DB, type Db } from '../../db/db.js';
import { secret } from '../../db/schema.js';
import { Bus, type BusMessage } from '../../infra/bus.js';
import { problems } from '../../common/problem.js';
import { SecretsService } from '../../secrets/secrets.service.js';
import { DeliveryError, sendNotification } from './transport.js';

type Severity = 'info' | 'warning' | 'critical';
type Kind = 'alarm' | 'commit' | 'link' | 'vpn' | 'global-blocking';
export interface Notice {
  kind: Kind;
  severity: Severity;
  source: string;
  transition: string;
  at: string;
}
interface Job {
  rule: string | null;
  channel: string;
  notice: Notice;
  tries: number;
  due: number;
}
export interface Delivery {
  channel: string;
  rule: string | null;
  kind: Kind;
  at: string;
  attempt: number;
  result: 'sent' | 'failed' | 'discarded';
  error: string | null;
}
const ranks = { info: 0, warning: 1, critical: 2 };
const scalar = (v: unknown): string =>
  typeof v === 'string' ? v.replace(/[^a-zA-Z0-9_.:/-]/g, '').slice(0, 96) : '';

/** API-owned dispatcher: one bounded worker, latest-only reload, no event payloads or provider errors escape. */
@Injectable()
export class NotificationsService implements OnModuleDestroy, OnApplicationBootstrap {
  private readonly log = new Logger('notifications');
  private config: NotificationsConfig = { channels: [], rules: [] };
  private queue: Job[] = [];
  private history: Delivery[] = [];
  private throttles = new Map<string, number>();
  private dedup = new Map<string, number>();
  private stopped = false;
  private reloadBusy = false;
  private reloadDirty = false;
  private generation = 0;
  private workerBusy = false;
  private timer: NodeJS.Timeout | undefined;
  private retryTimer: NodeJS.Timeout | undefined;
  private active: AbortController | undefined;
  private runtimeError: string | null = null;
  private deliveryTimedOut = false;
  private pendingCommit: { severity: Severity; transition: string } | null = null;
  private readonly offPublish: () => void;
  private readonly offAgent: () => void;
  now: () => number = () => Date.now();
  deliver = sendNotification;
  readSecret = async (ref: string): Promise<string> => {
    const [row] = await this.db
      .select({ ciphertext: secret.ciphertext })
      .from(secret)
      .where(eq(secret.ref, ref))
      .limit(1);
    if (!row) throw new DeliveryError('secret-unavailable');
    return this.moduleRef.get(SecretsService, { strict: false }).decrypt(row.ciphertext, ref);
  };

  constructor(
    private readonly ds: DatastoreService,
    private readonly bus: Bus,
    @Inject(DB) private readonly db: Db,
    private readonly moduleRef: ModuleRef,
  ) {
    this.offPublish = bus.onPublish((m) => this.message(m));
    this.offAgent = bus.onAgentEvent((e) => this.agent(e));
  }
  onApplicationBootstrap(): void {
    void this.reload();
  }
  onModuleDestroy(): void {
    this.stopped = true;
    this.offPublish();
    this.offAgent();
    this.active?.abort();
    clearTimeout(this.timer);
    clearTimeout(this.retryTimer);
    this.queue = [];
  }

  /** At most one DB read, regardless of commit storms; no overlapping reads on a hung datastore. */
  async reload(): Promise<void> {
    this.generation++;
    this.reloadDirty = true;
    this.active?.abort();
    clearTimeout(this.timer);
    if (this.reloadBusy || this.stopped) return;
    this.reloadBusy = true;
    try {
      while (this.reloadDirty && !this.stopped) {
        this.reloadDirty = false;
        const g = this.generation;
        const watchdog = setTimeout(() => {
          if (!this.stopped) this.runtimeError = 'configuration-read-timeout';
        }, 5000);
        watchdog.unref();
        try {
          const { doc } = await this.ds.getRunning();
          if (this.stopped || g !== this.generation) continue;
          const mgmt = isPlainObject(doc['management']) ? doc['management'] : {};
          const parsed = NotificationsSchema.safeParse(mgmt['notifications'] ?? {});
          if (!parsed.success) throw new Error('invalid notification configuration');
          this.runtimeError = null;
          this.configure(parsed.data);
        } catch {
          this.runtimeError = 'configuration-unavailable';
          this.log.warn('notification configuration unavailable');
          // Keep last good config, fail closed while unavailable. Retry only after the read settled.
          if (!this.stopped) {
            clearTimeout(this.retryTimer);
            this.retryTimer = setTimeout(() => void this.reload(), 1000);
            this.retryTimer.unref();
          }
          break;
        } finally {
          clearTimeout(watchdog);
        }
      }
    } finally {
      this.reloadBusy = false;
      if (!this.stopped && !this.runtimeError) {
        const pending = this.pendingCommit;
        this.pendingCommit = null;
        if (pending) this.emit('commit', pending.severity, 'configuration', pending.transition);
        this.wake();
      }
    }
  }
  configure(config: NotificationsConfig): void {
    this.config = NotificationsSchema.parse(config);
    const names = new Set([
      ...config.rules.map((r) => r.name),
      ...config.channels.map((c) => `test:${c.name}`),
    ]);
    for (const key of this.throttles.keys()) if (!names.has(key)) this.throttles.delete(key);
    this.active?.abort();
    this.wake();
  }
  state() {
    return {
      queued: this.queue.length,
      busy: this.workerBusy,
      error: this.reloadBusy
        ? 'configuration-loading'
        : (this.runtimeError ?? (this.deliveryTimedOut ? 'delivery-timeout' : null)),
      configuredChannels: this.config.channels.length,
      deliveries: [...this.history].reverse(),
    };
  }

  private message(m: BusMessage): void {
    const d = isPlainObject(m.data) ? m.data : {};
    if (m.topic === 'commit.events') {
      if (['applied', 'confirmed', 'reverted', 'failed'].includes(String(d['type'])))
        this.pendingCommit = {
          severity: d['type'] === 'failed' ? 'critical' : 'info',
          transition: scalar(d['type']),
        };
      // Pause and abort sends immediately. Only the latest commit notice is
      // dispatched after the latest running config read succeeds.
      void this.reload();
    } else if (m.topic === 'security.events' && d['type'] === 'global-blocking-fetch-failed') {
      this.emit('global-blocking', 'warning', scalar(d['list']), 'fetch-failed');
    } else if (m.topic === 'alarm.events') {
      const severity = d['severity'];
      this.emit(
        'alarm',
        severity === 'critical' || severity === 'info' ? severity : 'warning',
        scalar(d['rule']),
        scalar(d['type']),
      );
    }
  }
  private agent(e: Event): void {
    if (e.kind === EventKind.EVENT_KIND_LINK_UP || e.kind === EventKind.EVENT_KIND_LINK_DOWN)
      this.emit(
        'link',
        e.kind === EventKind.EVENT_KIND_LINK_DOWN ? 'warning' : 'info',
        e.interface ?? '',
        e.kind === EventKind.EVENT_KIND_LINK_DOWN ? 'down' : 'up',
      );
    else if (
      e.kind ===
        (e.attributes['event'] === 'daemon' && e.attributes['up'] === 'no'
          ? EventKind.EVENT_KIND_ERROR
          : EventKind.EVENT_KIND_UNSPECIFIED) &&
      e.attributes['source'] === 'strongswan' &&
      ['ike-updown', 'child-updown', 'ike-rekey', 'child-rekey', 'daemon', 'poll'].includes(
        e.attributes['event'] ?? '',
      ) &&
      ['yes', 'no'].includes(e.attributes['up'] ?? '') &&
      (e.attributes['event'] === 'daemon' || !!scalar(e.attributes['tunnel']))
    )
      this.emit(
        'vpn',
        e.attributes['up'] === 'no' ? 'warning' : 'info',
        e.attributes['event'] === 'daemon' ? 'strongswan-daemon' : e.attributes['tunnel']!,
        e.attributes['up'] === 'no' ? 'down' : 'up',
      );
    else if (e.kind === EventKind.EVENT_KIND_WIREGUARD_PEER_CHANGED)
      this.emit(
        'vpn',
        e.attributes['dead'] === 'true' ? 'warning' : 'info',
        e.interface ?? '',
        e.attributes['dead'] === 'true' ? 'down' : 'changed',
      );
  }
  emit(kind: Kind, severity: Severity, source: string, transition: string): void {
    if (this.stopped || this.runtimeError || this.deliveryTimedOut || this.reloadBusy) return;
    const at = this.now();
    const notice: Notice = {
      kind,
      severity,
      source: scalar(source),
      transition: scalar(transition),
      at: new Date(at).toISOString(),
    };
    for (const [k, expiry] of this.dedup) if (expiry <= at) this.dedup.delete(k);
    for (const rule of this.config.rules) {
      if (!rule.enabled || !rule.events.includes(kind) || ranks[severity] < ranks[rule.minSeverity])
        continue;
      const key = `${rule.name}|${kind}|${notice.source}|${notice.transition}`;
      if ((this.throttles.get(rule.name) ?? 0) > at || this.dedup.has(key)) continue;
      let accepted = false;
      for (const name of rule.channels) {
        if (this.queue.length >= 256) break;
        if (!this.channel(name)) continue;
        this.queue.push({ rule: rule.name, channel: name, notice, tries: 0, due: at });
        accepted = true;
      }
      if (accepted) {
        this.throttles.set(rule.name, at + rule.throttleSec * 1000);
        if (this.dedup.size >= 2048) this.dedup.delete(this.dedup.keys().next().value!);
        this.dedup.set(key, at + rule.throttleSec * 1000);
      }
    }
    this.wake();
  }
  test(name: string): { queued: true } {
    if (this.runtimeError || this.deliveryTimedOut || this.reloadBusy)
      throw problems.unavailable('notification configuration unavailable');
    if (!this.channel(name))
      throw problems.notFound('enabled running notification channel not found');
    if (this.queue.length >= 256)
      throw problems.conflict('notifications-busy', 'notification queue is full');
    // Tests share bounded per-channel throttling with ordinary jobs, and are not identified by a user-provided rule name.
    const key = `test:${name}`,
      at = this.now();
    if ((this.throttles.get(key) ?? 0) > at)
      throw problems.conflict('notifications-busy', 'wait before sending another test');
    this.throttles.set(key, at + 10000);
    this.queue.push({
      rule: null,
      channel: name,
      notice: {
        kind: 'commit',
        severity: 'info',
        source: 'test',
        transition: 'test',
        at: new Date(at).toISOString(),
      },
      tries: 0,
      due: at,
    });
    this.wake();
    return { queued: true };
  }
  private channel(name: string): NotificationChannel | undefined {
    return this.config.channels.find((c) => c.name === name && c.enabled);
  }
  private valid(job: Job): boolean {
    if (!this.channel(job.channel)) return false;
    if (job.rule === null) return true;
    const r = this.config.rules.find((r) => r.name === job.rule);
    return (
      !!r?.enabled &&
      r.channels.includes(job.channel) &&
      r.events.includes(job.notice.kind) &&
      ranks[job.notice.severity] >= ranks[r.minSeverity]
    );
  }
  private wake(): void {
    if (
      this.workerBusy ||
      this.stopped ||
      this.runtimeError ||
      this.deliveryTimedOut ||
      this.reloadBusy
    )
      return;
    clearTimeout(this.timer);
    const next = this.queue.reduce((n, j) => Math.min(n, j.due), Infinity);
    if (!Number.isFinite(next)) return;
    this.timer = setTimeout(() => void this.run(), Math.max(0, next - this.now()));
    this.timer.unref();
  }
  async run(): Promise<void> {
    if (
      this.workerBusy ||
      this.stopped ||
      this.runtimeError ||
      this.deliveryTimedOut ||
      this.reloadBusy
    )
      return;
    const index = this.queue.findIndex((j) => j.due <= this.now());
    if (index < 0) {
      this.wake();
      return;
    }
    const job = this.queue.splice(index, 1)[0]!;
    if (!this.valid(job)) {
      this.record(job, 'discarded', null);
      this.wake();
      return;
    }
    this.workerBusy = true;
    const controller = new AbortController();
    this.active = controller;
    const timer = setTimeout(() => {
      controller.abort();
      this.deliveryTimedOut = true;
    }, 10000);
    timer.unref();
    try {
      job.tries++;
      await this.deliver(
        this.channel(job.channel)!,
        JSON.stringify(job.notice),
        this.readSecret,
        controller.signal,
      );
      if (!this.stopped) this.record(job, controller.signal.aborted ? 'discarded' : 'sent', null);
    } catch (e) {
      if (!this.stopped) {
        this.record(
          job,
          'failed',
          e instanceof DeliveryError &&
            (/^http-[1-5][0-9]{2}$/.test(e.reason) ||
              [
                'secret-unavailable',
                'destination-rejected',
                'delivery-timeout',
                'transport-failed',
                'channel-invalid',
                'smtp-failed',
                'management-vrf-unsupported',
              ].includes(e.reason))
            ? e.reason
            : 'delivery-failed',
        );
        if (this.valid(job) && job.tries < 3 && this.queue.length < 256) {
          job.due = this.now() + 1000 * 2 ** (job.tries - 1);
          this.queue.push(job);
        }
      }
    } finally {
      clearTimeout(timer);
      this.active = undefined;
      this.workerBusy = false;
      this.deliveryTimedOut = false;
      this.wake();
    }
  }
  private record(job: Job, result: Delivery['result'], error: string | null): void {
    this.history.push({
      channel: job.channel,
      rule: job.rule,
      kind: job.notice.kind,
      at: new Date(this.now()).toISOString(),
      attempt: job.tries,
      result,
      error,
    });
    if (this.history.length > 500) this.history.shift();
  }
}
