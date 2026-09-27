import { Inject, Injectable, Logger, type OnModuleDestroy } from '@nestjs/common';
import {
  isPlainObject,
  type AutoBlock,
  type AutoBlockRule,
  type AutoBlockSourceKind,
} from '@ngfw/schema';
import { and, asc, desc, eq, gt, lte, sql } from 'drizzle-orm';
import { SystemEventsService } from '../../audit/system-events.service.js';
import { AuditService, type AuditEntry } from '../../audit/audit.service.js';
import { problems } from '../../common/problem.js';
import type { Principal } from '../../common/principal.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import { DB, type Db } from '../../db/db.js';
import { autoBlock as autoBlockTable } from '../../db/schema.js';
import { Bus } from '../../infra/bus.js';
import {
  blockDurationSec,
  canonicalSource,
  compileAllowlist,
  isAllowlisted,
  SlidingWindows,
  type ParsedPrefix,
} from './engine.js';

type Json = Record<string, unknown>;

/** A blocked source as the API returns it. */
export interface BlockedEntry {
  source: string;
  reason: string;
  hits: number;
  offences: number;
  origin: string;
  note: string;
  firstSeen: string;
  blockedAt: string;
  expiresAt: string;
}

/** How often expired blocks are swept and window memory pruned. */
const SWEEP_MS = 30_000;
/** A manual block with no explicit duration lasts this long. */
const DEFAULT_MANUAL_BLOCK_SEC = 3600;
/** Manual blocks are capped at 30 days. */
const MAX_MANUAL_BLOCK_SEC = 30 * 86_400;

/**
 * F-bruteforce-block (API side). Watches audited login failures (via `AuditService.onEntry`, so the login path is
 * untouched) and, for any detector enabled in `security.autoBlock`, counts failures per source in a sliding window;
 * once the threshold trips, the source is written to the `auto_block` table with a TTL that escalates on repeat
 * offences. The allow-list (plus loopback) always wins. Expired entries are swept on a timer.
 *
 * The `auto_block` table is the API's view of the live set; enforcement on the data plane (VPP acl / nftables local-in)
 * is the agent's, fed from this set as a system-owned Global Blocking list — that push is F-bruteforce-block-host.
 * `observe()` is also the entry point other detectors (VPN/EAP auth failures) call; SSH and port-scan detectors are
 * host-side and land with the same call over the agent bridge (host follow-up).
 */
@Injectable()
export class AutoBlockService implements OnModuleDestroy {
  private readonly log = new Logger('auto-block');
  private cfg: AutoBlock | undefined;
  private allow: ParsedPrefix[] = compileAllowlist([]);
  private readonly windows = new SlidingWindows();
  private sweepTimer: NodeJS.Timeout | undefined;
  private readonly offAudit: () => void;
  private readonly offCommit: () => void;
  private stopped = false;
  /** Clock (tests move it). */
  now: () => number = () => Date.now();

  constructor(
    private readonly ds: DatastoreService,
    private readonly audit: AuditService,
    private readonly events: SystemEventsService,
    private readonly bus: Bus,
    @Inject(DB) private readonly db: Db,
  ) {
    this.offAudit = this.audit.onEntry((e) => this.onAudit(e));
    this.offCommit = this.bus.onPublish((m) => {
      if (m.topic === 'commit.events') {
        const type = (m.data as { type?: string } | null)?.type;
        if (type === 'applied' || type === 'confirmed' || type === 'reverted') void this.reload();
      }
    });
  }

  /** Called once the app is ready (index.ts provider factory), like the alarms engine. */
  async start(): Promise<void> {
    await this.reload();
    this.sweepTimer = setInterval(() => void this.sweep(), SWEEP_MS);
    this.sweepTimer.unref?.();
  }

  onModuleDestroy(): void {
    this.stopped = true;
    this.offAudit();
    this.offCommit();
    if (this.sweepTimer) clearInterval(this.sweepTimer);
  }

  /** Reload thresholds and the allow-list from the running configuration. */
  async reload(): Promise<void> {
    const { doc } = await this.ds.getRunning();
    const sec = isPlainObject(doc['security']) ? (doc['security'] as Json) : {};
    this.cfg = (isPlainObject(sec['autoBlock']) ? sec['autoBlock'] : undefined) as
      | AutoBlock
      | undefined;
    this.allow = compileAllowlist(this.cfg?.allowlist ?? []);
  }

  private ruleFor(kind: AutoBlockSourceKind): AutoBlockRule | undefined {
    if (this.cfg === undefined || !this.cfg.enabled) return undefined;
    return this.cfg.rules.find((r) => r.source === kind && r.enabled);
  }

  // ---- ingestion ----------------------------------------------------------------------------------------------------

  private onAudit(e: AuditEntry): void {
    if (e.action !== 'auth.login' || e.result !== 'failure' || e.sourceIp === null) return;
    this.observe(e.sourceIp, 'webLogin');
  }

  /**
   * Record one failed authentication from `sourceRaw` for `kind`. Fire-and-forget: never throws into the caller (the
   * login path). When the detector's threshold trips and the source is not allow-listed, it is blocked.
   */
  observe(sourceRaw: string, kind: AutoBlockSourceKind): void {
    try {
      const rule = this.ruleFor(kind);
      if (rule === undefined) return;
      const source = canonicalSource(sourceRaw);
      if (source === undefined) return;
      if (isAllowlisted(this.allow, sourceRaw)) return;
      const { count, tripped } = this.windows.observe(
        kind,
        source,
        this.now(),
        rule.windowSec,
        rule.threshold,
      );
      if (tripped) {
        this.windows.clear(kind, source);
        void this.block(source, kind, rule, count).catch((err) =>
          this.log.error(`auto-block of ${source} failed: ${String(err)}`),
        );
      }
    } catch (err) {
      this.log.warn(`observe(${kind}) threw: ${String(err)}`);
    }
  }

  private async block(
    source: string,
    kind: AutoBlockSourceKind,
    rule: AutoBlockRule,
    hits: number,
  ): Promise<void> {
    const now = new Date(this.now());
    const existing = await this.db
      .select({ offences: autoBlockTable.offences })
      .from(autoBlockTable)
      .where(eq(autoBlockTable.source, source));
    const isNew = existing.length === 0;
    if (isNew && !(await this.makeRoom())) {
      this.log.warn(`auto-block set is full (maxEntries); not blocking ${source}`);
      return;
    }
    const offences = (existing[0]?.offences ?? 0) + 1;
    const secs = blockDurationSec(offences, rule);
    const expiresAt = new Date(now.getTime() + secs * 1000);
    await this.db
      .insert(autoBlockTable)
      .values({
        source,
        reason: kind,
        hits,
        offences,
        origin: 'auto',
        blockedAt: now,
        expiresAt,
      })
      .onConflictDoUpdate({
        target: autoBlockTable.source,
        set: { reason: kind, hits, offences, blockedAt: now, expiresAt },
      });
    this.emit('blocked', source, { reason: kind, hits, offences, blockSec: secs });
    void this.events
      .record('warning', 'security', 'AUTO_BLOCK', `blocked ${source} (${kind}, ${hits} hits, offence ${offences}) for ${secs}s`, {
        source,
        reason: kind,
        offences,
      })
      .catch(() => {});
  }

  /** Ensure there is room for one more entry; evict the soonest-expiring auto entry if at the cap. Returns true when there is room. */
  private async makeRoom(): Promise<boolean> {
    const max = this.cfg?.maxEntries ?? 10_000;
    const now = new Date(this.now());
    const [{ n } = { n: 0 }] = await this.db
      .select({ n: sql<number>`count(*)` })
      .from(autoBlockTable)
      .where(gt(autoBlockTable.expiresAt, now));
    if (Number(n) < max) return true;
    const victim = await this.db
      .select({ id: autoBlockTable.id })
      .from(autoBlockTable)
      .where(and(gt(autoBlockTable.expiresAt, now), eq(autoBlockTable.origin, 'auto')))
      .orderBy(asc(autoBlockTable.expiresAt))
      .limit(1);
    if (victim.length === 0) return false;
    await this.db.delete(autoBlockTable).where(eq(autoBlockTable.id, victim[0]!.id));
    return true;
  }

  // ---- admin actions ------------------------------------------------------------------------------------------------

  /** List the live (unexpired) blocks, newest first. */
  async list(): Promise<BlockedEntry[]> {
    const now = new Date(this.now());
    const rows = await this.db
      .select()
      .from(autoBlockTable)
      .where(gt(autoBlockTable.expiresAt, now))
      .orderBy(desc(autoBlockTable.blockedAt));
    return rows.map((r) => ({
      source: r.source,
      reason: r.reason,
      hits: r.hits,
      offences: r.offences,
      origin: r.origin,
      note: r.note,
      firstSeen: r.firstSeen.toISOString(),
      blockedAt: r.blockedAt.toISOString(),
      expiresAt: r.expiresAt.toISOString(),
    }));
  }

  /** Remove a block (admin). Returns false when the source was not blocked. */
  async unblock(sourceRaw: string, who: Principal): Promise<boolean> {
    const source = canonicalSource(sourceRaw);
    if (source === undefined) throw problems.badRequest(`'${sourceRaw}' is not an IP address`);
    const deleted = await this.db
      .delete(autoBlockTable)
      .where(eq(autoBlockTable.source, source))
      .returning({ id: autoBlockTable.id });
    if (deleted.length === 0) return false;
    this.emit('unblocked', source, { by: who.username, reason: 'manual' });
    void this.events
      .record('info', 'security', 'AUTO_UNBLOCK', `${who.username} unblocked ${source}`, {
        source,
        by: who.username,
      })
      .catch(() => {});
    return true;
  }

  /** Block a source by hand (admin). Refused for an allow-listed source (a lock-out of a management net is impossible). */
  async manualBlock(
    sourceRaw: string,
    blockSec: number | undefined,
    note: string,
    who: Principal,
  ): Promise<BlockedEntry> {
    const source = canonicalSource(sourceRaw);
    if (source === undefined) throw problems.badRequest(`'${sourceRaw}' is not an IP address`);
    if (isAllowlisted(this.allow, sourceRaw)) {
      throw problems.conflict('allowlisted', `${source} is on the allow-list and cannot be blocked`);
    }
    const secs = Math.min(Math.max(blockSec ?? DEFAULT_MANUAL_BLOCK_SEC, 1), MAX_MANUAL_BLOCK_SEC);
    const now = new Date(this.now());
    const expiresAt = new Date(now.getTime() + secs * 1000);
    await this.makeRoom();
    await this.db
      .insert(autoBlockTable)
      .values({ source, reason: 'manual', hits: 0, offences: 1, origin: 'manual', note, blockedAt: now, expiresAt })
      .onConflictDoUpdate({
        target: autoBlockTable.source,
        set: { reason: 'manual', origin: 'manual', note, blockedAt: now, expiresAt },
      });
    this.emit('blocked', source, { reason: 'manual', by: who.username, blockSec: secs });
    void this.events
      .record('warning', 'security', 'AUTO_BLOCK', `${who.username} blocked ${source} for ${secs}s`, {
        source,
        by: who.username,
        manual: true,
      })
      .catch(() => {});
    const [row] = await this.db
      .select()
      .from(autoBlockTable)
      .where(eq(autoBlockTable.source, source));
    return {
      source,
      reason: row!.reason,
      hits: row!.hits,
      offences: row!.offences,
      origin: row!.origin,
      note: row!.note,
      firstSeen: row!.firstSeen.toISOString(),
      blockedAt: row!.blockedAt.toISOString(),
      expiresAt: row!.expiresAt.toISOString(),
    };
  }

  // ---- expiry -------------------------------------------------------------------------------------------------------

  /** Delete expired blocks (emitting an unblock event for each) and prune window memory. */
  async sweep(): Promise<void> {
    if (this.stopped) return;
    try {
      const now = new Date(this.now());
      const expired = await this.db
        .delete(autoBlockTable)
        .where(lte(autoBlockTable.expiresAt, now))
        .returning({ source: autoBlockTable.source });
      for (const e of expired) this.emit('unblocked', e.source, { reason: 'expired' });
      // forget windows quiet for a day (the longest reasonable detector window plus slack)
      this.windows.prune(this.now(), 86_400_000);
    } catch (err) {
      this.log.warn(`sweep failed: ${String(err)}`);
    }
  }

  private emit(type: 'blocked' | 'unblocked', source: string, extra: Json): void {
    this.bus.publish('security.events', { type, source, at: new Date(this.now()).toISOString(), ...extra });
  }
}
