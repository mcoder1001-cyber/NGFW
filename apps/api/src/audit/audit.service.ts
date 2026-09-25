import { Inject, Injectable, Logger } from '@nestjs/common';
import { count, desc, eq } from 'drizzle-orm';
import { DB, type Db } from '../db/db.js';
import { auditLog } from '../db/schema.js';
import { SystemEventsService } from './system-events.service.js';

export interface AuditEntry {
  userId: number | null;
  username: string | null;
  sourceIp: string | null;
  action: string;
  resource: string | null;
  /** Must already be redacted (redactSecrets) — the audit log never stores secrets. */
  before?: unknown;
  after?: unknown;
  result: 'success' | 'failure';
  status: number | null;
}

/** Distinct aggregation keys per minute; beyond this every new key shares one overflow row per action. */
export const AGGREGATE_MAX_KEYS = 200;
/** At most one AUDIT_WRITE_FAILED system_event per this interval (the counter keeps the exact number). */
const FAILURE_EVENT_MS = 60_000;

/** 1, 10, 100, … — the counts at which an aggregated row is written. */
function powerOfTen(n: number): boolean {
  while (n >= 10 && n % 10 === 0) n /= 10;
  return n === 1;
}

/**
 * Writes and lists `audit_log` (P06 §7: user, ip, route, before/after diff, result for every mutation).
 *
 * TD-10b (review 2.3e):
 * - A failed write never fails the request it records (except the privileged routes, `begin`), but it is no longer
 *   only a log line: `writeFailures` counts every one, and a `system_event` AUDIT_WRITE_FAILED (error, subsystem
 *   `audit`) says so — at most one per minute, carrying the running count and how many were folded into it.
 * - `writeAggregated` records floods (401 mutations, unknown refresh/logout tokens) as one row per key and minute at
 *   the 1st, 10th, 100th … occurrence (`after.aggregated = {window, count}`: "at least `count` in this minute").
 * - `begin`/`finish`: write-ahead for privileged routes (audit.interceptor.ts) — the row exists BEFORE the action,
 *   and if it cannot be written the action does not run (fail closed).
 */
@Injectable()
export class AuditService {
  private readonly log = new Logger('Audit');
  private failures = 0;
  private folded = 0;
  private lastFailureEvent = 0;
  private window = { minute: -1, counts: new Map<string, number>() };

  constructor(
    @Inject(DB) private readonly db: Db,
    private readonly events: SystemEventsService,
  ) {}

  /** Audit writes that failed since the process started (TD-10b counter; also in each AUDIT_WRITE_FAILED event). */
  get writeFailures(): number {
    return this.failures;
  }

  private values(e: AuditEntry) {
    return {
      userId: e.userId,
      username: e.username,
      sourceIp: e.sourceIp,
      action: e.action,
      resource: e.resource,
      before: e.before ?? null,
      after: e.after ?? null,
      result: e.result,
      status: e.status,
    };
  }

  async write(e: AuditEntry): Promise<void> {
    try {
      await this.db.insert(auditLog).values(this.values(e));
    } catch (err) {
      // never fail (or leak into) the response because of the audit write — but never silently either
      this.failed(e.action, err);
    }
  }

  /**
   * One occurrence of a flood-prone event under `key` (e.g. `<route>|<reason>|<client>`): a row at the 1st, 10th,
   * 100th … occurrence of the key in the current minute. Past AGGREGATE_MAX_KEYS keys in a minute, new keys fold into
   * one `<action>|*` key per action (source address dropped), so spraying addresses cannot grow the table either.
   */
  writeAggregated(e: AuditEntry, key: string): Promise<void> {
    const minute = Math.floor(Date.now() / 60_000);
    if (minute !== this.window.minute) this.window = { minute, counts: new Map() };
    const counts = this.window.counts;
    let k = key;
    let entry = e;
    if (!counts.has(k) && counts.size >= AGGREGATE_MAX_KEYS) {
      k = `${e.action}|*`;
      entry = { ...e, sourceIp: null };
    }
    const n = (counts.get(k) ?? 0) + 1;
    counts.set(k, n);
    if (!powerOfTen(n)) return Promise.resolve();
    return this.write({
      ...entry,
      after: {
        ...(entry.after as Record<string, unknown> | undefined),
        aggregated: {
          window: new Date(minute * 60_000).toISOString(),
          count: n,
          ...(k.endsWith('|*') ? { sources: 'many' } : {}),
        },
      },
    });
  }

  /**
   * Write-ahead row of a privileged request (TD-10b, fail closed): inserted before the action runs, as a failure
   * with `after.incomplete` (what a crash in the middle leaves behind). Throws when it cannot be written — the
   * caller then refuses the request. Returns the row id for `finish`.
   */
  async begin(e: AuditEntry): Promise<number> {
    try {
      const [row] = await this.db
        .insert(auditLog)
        .values(
          this.values({
            ...e,
            result: 'failure',
            status: null,
            after: { incomplete: true },
          }),
        )
        .returning({ id: auditLog.id });
      return row!.id;
    } catch (err) {
      this.failed(e.action, err);
      throw err;
    }
  }

  /** The outcome of a `begin` row. A failure here is counted and reported; the row keeps saying `incomplete`. */
  async finish(id: number, e: AuditEntry): Promise<void> {
    try {
      await this.db
        .update(auditLog)
        .set({
          resource: e.resource,
          before: e.before ?? null,
          after: e.after ?? null,
          result: e.result,
          status: e.status,
        })
        .where(eq(auditLog.id, id));
    } catch (err) {
      this.failed(e.action, err);
    }
  }

  private failed(action: string, err: unknown): void {
    this.failures += 1;
    // drizzle wraps the driver error, and its own message repeats the query WITH its parameters (the row): log the
    // driver's cause, which names the constraint/connection problem, never row values
    const cause = ((err as { cause?: unknown } | null)?.cause ?? err) as {
      message?: unknown;
      code?: unknown;
    } | null;
    this.log.error(`audit write failed for ${action}: ${String(cause?.message ?? cause)}`);
    const now = Date.now();
    if (now - this.lastFailureEvent < FAILURE_EVENT_MS) {
      this.folded += 1;
      return;
    }
    this.lastFailureEvent = now;
    const folded = this.folded;
    this.folded = 0;
    void this.events.record(
      'error',
      'audit',
      'AUDIT_WRITE_FAILED',
      `an audit_log row could not be written (${action}); the action itself was not blocked unless it is privileged`,
      {
        action,
        code: cause?.code ?? null,
        failuresTotal: this.failures,
        foldedSinceLastEvent: folded,
      },
    );
  }

  async list(limit: number, offset: number) {
    const items = await this.db
      .select()
      .from(auditLog)
      .orderBy(desc(auditLog.id))
      .limit(limit)
      .offset(offset);
    const [c] = await this.db.select({ n: count() }).from(auditLog);
    return { items, total: c?.n ?? 0 };
  }
}
