import { Inject, Injectable, Logger, type OnApplicationShutdown } from '@nestjs/common';
import { ApplyStatus, EventKind, type ApplyResponse, type ObjectResult } from '@ngfw/proto';
import { deepEqual, diff, parsePointer, type UserConfig } from '@ngfw/schema';
import { randomUUID } from 'node:crypto';
import { AgentClient } from '../agent/agent.client.js';
import { AuditService } from '../audit/audit.service.js';
import { SystemEventsService } from '../audit/system-events.service.js';
import { TokensService } from '../auth/tokens.service.js';
import { ENV, type Env } from '../config.js';
import { documentHash } from '../common/json.js';
import { Mutex } from '../common/mutex.js';
import { ProblemError, problems, type ProblemIssue } from '../common/problem.js';
import type { Principal } from '../common/principal.js';
import { CONFIG_REPO } from '../datastore/datastore.service.js';
import {
  emptyDocument,
  hydrateHashes,
  replaceUserHash,
  secretChanges,
  stagedHashesOnly,
  privilegedChanges,
  redact,
  secretRefs,
} from '../datastore/documents.js';
import { checkLock } from '../datastore/lock.js';
import type {
  ConfigRepo,
  Doc,
  PasswordReset,
  PendingCommit,
  RevisionMeta,
  SyncStatus,
} from '../datastore/repo.js';
import { Bus } from '../infra/bus.js';
import { planEntry, ValidationService, type PlanEntry } from './validation.service.js';

export interface CommitOptions {
  /** Seconds until the agent self-reverts unless confirmed (P06 §4, proto.md §4). 0/undefined = no timer. */
  confirmSec?: number;
  comment?: string;
}

export interface ObjectResultJson {
  key: string;
  op: string;
  code: string;
  message: string;
  pointer: string;
  subsystem: string;
}

export interface SyncJson {
  state: SyncStatus['state'];
  reason: string;
  txnId: string | null;
  since: string;
}

/**
 * - `applied`: every changed domain is implemented by the agent (or applied by the API itself: management.users).
 * - `partially-applied` / `not-applied`: some / all changed domains are stored in running but NOT enforced by this agent
 *   build (review M4) — `notApplied` names them.
 */
export type CommitStatus =
  'applied' | 'partially-applied' | 'not-applied' | 'pending' | 'unchanged' | 'confirmed';

export interface CommitResult {
  status: CommitStatus;
  txnId?: string;
  revision?: RevisionMeta;
  confirmDeadline?: string;
  results: ObjectResultJson[];
  summary?: Record<string, number>;
  warnings: ProblemIssue[];
  /** Changed top-level keys the agent does not implement: stored in running, not enforced (review M4). */
  notApplied: string[];
  /** Whether running and the data plane are known to agree (review M3). */
  sync?: SyncJson;
}

export interface PendingInfo {
  txnId: string;
  author: number | null;
  comment: string;
  kind: string;
  deadline: string;
  createdAt: string;
}

const CODE_NAMES = [
  'unspecified',
  'ok',
  'failed',
  'skipped',
  'reverted',
  'revert-failed',
  'dependency-missing',
  'invalid',
];

export function resultJson(r: ObjectResult): ObjectResultJson {
  return { ...planEntry(r), code: CODE_NAMES[r.code] ?? String(r.code), message: r.message };
}

function statusName(s: ApplyStatus): string {
  return ApplyStatus[s]?.replace('APPLY_STATUS_', '').toLowerCase() ?? String(s);
}

export function syncJson(s: SyncStatus): SyncJson {
  return { state: s.state, reason: s.reason, txnId: s.txnId, since: s.since.toISOString() };
}

/** How long after a confirm deadline the API waits before asking the agent whether it reverted. */
const REVERT_GRACE_MS = 1500;
/** Reconcile retry delays (agent unreachable / still degraded). */
const RECONCILE_RETRY_MS = [1000, 2000, 5000, 10_000, 30_000];

/** The API applies `management.users` itself (app_user), so changes there count as applied (review M4). */
const API_APPLIED = ['/management/users'];

/**
 * Changed top-level keys between two documents that the agent does not implement (review M4). A domain whose only
 * changes are API-applied subtrees is not listed.
 */
export function notAppliedChanges(
  before: Doc,
  after: Doc,
  unimplemented: readonly string[],
): string[] {
  const out = new Set<string>();
  for (const c of diff(redact(before), redact(after))) {
    const key = parsePointer(c.pointer)[0] ?? '';
    if (!unimplemented.includes(key)) continue;
    if (API_APPLIED.some((p) => c.pointer === p || c.pointer.startsWith(p + '/'))) continue;
    out.add(key);
  }
  return [...out];
}

/** Agent failures after which the data plane state is not known (the request may have been applied). */
function outcomeUnknown(e: unknown): boolean {
  if (!(e instanceof ProblemError)) return true;
  if (e.slug === 'agent-timeout' || e.slug === 'agent-error') return true;
  if (e.slug === 'agent-unavailable') {
    // refused/missing socket: the request never left the API
    return !/No connection established|ECONNREFUSED|ENOENT|connect /i.test(String(e.detail));
  }
  return false;
}

interface InFlight {
  txnId: string;
  config: Doc;
  meta: { authorId: number | null; comment: string; kind: string; clearPending: boolean };
}

/**
 * The commit engine (P06 §4): validate (3 tiers) → admin check of the whole change (review M1) → agent Apply(txn) →
 * on APPLIED persist a revision (full redacted snapshot, sha256, author, comment, parent, pinned secret versions) and
 * promote the candidate; FAILED/ROLLED_BACK → 422 with per-object results, running untouched. Anything that leaves
 * the data plane in an unknown state (DEGRADED, agent timeout/lost answer, revision not saved) marks sync `unknown`/
 * `degraded` and starts a reconcile: if the agent applied the transaction (Health.last_txn_id) the revision is saved,
 * otherwise running is re-applied (review M3). Confirmed commits wait in `config_pending` until CONFIRMED.
 */
@Injectable()
export class CommitService implements OnApplicationShutdown {
  private readonly log = new Logger('Commit');
  private readonly mutex = new Mutex();
  private watchTimer: NodeJS.Timeout | undefined;
  private reconcileTimer: NodeJS.Timeout | undefined;
  private reconcileAttempt = 0;
  private inflight: InFlight | undefined;
  /** In-memory copy: authoritative in this process even when the database write of the sync state fails. */
  private sync: SyncStatus = { state: 'in-sync', reason: '', txnId: null, since: new Date() };
  private readonly unsubscribe: () => void;

  constructor(
    @Inject(CONFIG_REPO) private readonly repo: ConfigRepo,
    private readonly validation: ValidationService,
    private readonly agent: AgentClient,
    private readonly events: SystemEventsService,
    private readonly bus: Bus,
    @Inject(ENV) private readonly env: Env,
    private readonly tokens: TokensService,
    private readonly audit: AuditService,
  ) {
    this.unsubscribe = this.bus.onAgentEvent((e) => {
      if (e.kind === EventKind.EVENT_KIND_CONFIRM_REVERTED && e.txnId)
        void this.onReverted(e.txnId, 'event');
    });
  }

  onApplicationShutdown(): void {
    this.unsubscribe();
    if (this.watchTimer) clearTimeout(this.watchTimer);
    if (this.reconcileTimer) clearTimeout(this.reconcileTimer);
  }

  // ------------------------------------------------------------------------------------------------ sync state

  async syncStatus(): Promise<SyncJson> {
    return syncJson(this.sync);
  }

  private async setSync(
    state: SyncStatus['state'],
    reason: string,
    txnId: string | null,
  ): Promise<void> {
    const changed = state !== this.sync.state;
    this.sync = { state, reason, txnId, since: new Date() };
    await this.repo.setSync({ state, reason, txnId }).catch((e: Error) => {
      this.log.error(`could not persist sync state '${state}': ${e.message}`);
    });
    if (changed) {
      this.bus.publish('commit.events', { type: 'sync', state, reason, txnId });
      await this.events.record(
        state === 'in-sync' ? 'info' : 'error',
        'commit',
        state === 'in-sync' ? 'RUNNING_IN_SYNC' : `RUNNING_${state.toUpperCase()}`,
        state === 'in-sync' ? 'running and the data plane agree again' : reason,
        { txnId },
      );
    }
  }

  /** After an API restart: a persisted unknown/degraded state is reconciled again. */
  async resumeSync(): Promise<void> {
    const s = await this.repo.getSync();
    this.sync = s;
    if (s.state !== 'in-sync') this.scheduleReconcile(0);
  }

  private scheduleReconcile(delayMs?: number): void {
    if (this.reconcileTimer) clearTimeout(this.reconcileTimer);
    const d =
      delayMs ??
      RECONCILE_RETRY_MS[Math.min(this.reconcileAttempt, RECONCILE_RETRY_MS.length - 1)] ??
      30_000;
    this.reconcileAttempt += 1;
    this.reconcileTimer = setTimeout(() => void this.reconcile(), d);
    this.reconcileTimer.unref();
  }

  /**
   * Bring running and the data plane together again (review M3). Returns the resulting state. Retries on its own while
   * the agent is unreachable or still degraded.
   */
  reconcile(): Promise<SyncJson> {
    return this.mutex.run(async () => {
      this.reconcileTimer = undefined;
      if (this.sync.state === 'in-sync') return syncJson(this.sync);
      try {
        const h = await this.agent.health();
        const f = this.inflight;
        if (f !== undefined && h.lastTxnId === f.txnId) {
          // the agent applied the transaction whose answer (or revision save) we lost: keep it
          await this.promote(f.config, { ...f.meta, txnId: f.txnId });
          this.inflight = undefined;
          await this.setSync(
            'in-sync',
            `transaction ${f.txnId} was applied; revision saved`,
            f.txnId,
          );
        } else if (h.pendingConfirmTxnId) {
          // a confirm window is open on the agent: wait for it to close (confirm or self-revert)
          this.scheduleReconcile();
          return syncJson(this.sync);
        } else {
          // put the data plane back on running
          const running = await this.repo.latestRevision();
          const doc = hydrateHashes(
            running?.payload ?? emptyDocument(),
            await this.repo.userHashes(),
          );
          const txnId = randomUUID();
          const res = await this.agent.apply({
            txnId,
            desiredState: ValidationService.desiredState(doc),
            subsystems: (await this.validation.implemented()).subsystems,
            confirmTimeoutSec: 0,
            confirmTxnId: '',
          });
          if (res.status !== ApplyStatus.APPLY_STATUS_APPLIED) {
            await this.setSync(
              res.status === ApplyStatus.APPLY_STATUS_DEGRADED ? 'degraded' : 'unknown',
              `reconcile apply of running answered ${statusName(res.status)}`,
              txnId,
            );
            this.scheduleReconcile();
            return syncJson(this.sync);
          }
          this.inflight = undefined;
          await this.setSync('in-sync', `running re-applied (${txnId})`, txnId);
        }
        this.reconcileAttempt = 0;
      } catch (e) {
        this.log.warn(`reconcile failed, retrying: ${(e as Error).message}`);
        this.scheduleReconcile();
      }
      return syncJson(this.sync);
    });
  }

  /** Mark running as not matching the data plane and start reconciling (review M3). */
  private async lostTrack(
    state: 'unknown' | 'degraded',
    reason: string,
    inflight: InFlight | undefined,
  ): Promise<SyncJson> {
    this.inflight = inflight;
    this.reconcileAttempt = 0;
    await this.setSync(state, reason, inflight?.txnId ?? null);
    this.scheduleReconcile(250);
    return syncJson(this.sync);
  }

  // ------------------------------------------------------------------------------------------------ commit

  /** Validate the candidate without applying: 200 with plan/warnings, 400 with pointers. */
  async validateCandidate(user: Principal): Promise<{
    ok: true;
    warnings: ProblemIssue[];
    plan: PlanEntry[];
    notApplied: string[];
  }> {
    const c = await this.repo.candidate();
    const running = await this.repo.latestRevision();
    const doc = c.payload ?? running?.payload;
    if (doc === undefined) return { ok: true, warnings: [], plan: [], notApplied: [] };
    await this.assertMayApply(user, running?.payload ?? emptyDocument(), doc);
    const v = await this.validation.validate(doc, `validate-${randomUUID()}`);
    if (!v.ok)
      throw problems.validation(v.errors, `${v.tier} validation failed`, {
        tier: v.tier,
        warnings: v.warnings,
      });
    return {
      ok: true,
      warnings: v.warnings,
      plan: v.plan,
      notApplied: notAppliedChanges(running?.payload ?? emptyDocument(), doc, v.notApplied),
    };
  }

  /**
   * Run `fn` while no commit/confirm/rollback is in flight (TD-2 #1: a password set must not interleave with a commit
   * that validated against the old hash and would write it back to app_user when it promotes).
   */
  exclusive<T>(fn: () => Promise<T>): Promise<T> {
    return this.mutex.run(fn);
  }

  /**
   * Password set (D-097, review H1), called inside `exclusive`: a hash that the in-flight (lost-answer) document
   * staged for `username` is replaced, so the reconcile's promote cannot bring the old password back.
   */
  replaceInflightHash(username: string, hash: string): void {
    if (this.inflight === undefined) return;
    const next = replaceUserHash(this.inflight.config, username, hash);
    if (next !== null) this.inflight = { ...this.inflight, config: next };
  }

  commit(user: Principal, opts: CommitOptions): Promise<CommitResult> {
    return this.mutex.run(async () => {
      await this.assertNoPending();
      const c = await this.repo.candidate();
      checkLock(c, user, new Date(), this.env.VRX_LOCK_TTL_SEC);
      if (c.payload === null)
        return {
          status: 'unchanged',
          results: [],
          warnings: [],
          notApplied: [],
          sync: syncJson(this.sync),
        };
      const running = await this.repo.latestRevision();
      if (c.baseRevisionId !== (running?.id ?? null)) {
        throw problems.conflict(
          'candidate-stale',
          `the candidate was based on revision ${c.baseRevisionId ?? 'none'} but running is ${running?.id ?? 'none'}; discard and edit again`,
        );
      }
      return this.applyDocument(user, c.payload, {
        ...opts,
        kind: 'commit',
        parentId: running?.id ?? null,
      });
    });
  }

  /** New revision whose payload is revision `rev`'s, applied through the agent (P06 §4). */
  rollback(user: Principal, rev: number, opts: CommitOptions): Promise<CommitResult> {
    return this.mutex.run(async () => {
      await this.assertNoPending();
      const target = await this.repo.revision(rev);
      if (target === null) throw problems.notFound(`revision ${rev} does not exist`);
      const c = await this.repo.candidate();
      checkLock(c, user, new Date(), this.env.VRX_LOCK_TTL_SEC);
      if (c.payload !== null) {
        throw problems.conflict(
          'candidate-dirty',
          'the candidate has uncommitted changes; commit or discard them first',
        );
      }
      const running = await this.repo.latestRevision();
      return this.applyDocument(user, target.payload, {
        ...opts,
        comment: opts.comment || `rollback to revision ${rev}`,
        kind: 'rollback',
        parentId: running?.id ?? null,
        restoreSecrets: target.secretVersions ?? {},
      });
    });
  }

  async pendingInfo(): Promise<PendingInfo | null> {
    const p = await this.repo.pending();
    return p === null ? null : pendingJson(p);
  }

  /** `POST /config/commit/confirm`: confirm the pending transaction, persist its revision. */
  confirm(user: Principal): Promise<CommitResult> {
    return this.mutex.run(async () => {
      const p = await this.repo.pending();
      if (p === null)
        throw problems.conflict('no-pending-commit', 'there is no commit waiting for confirmation');
      let res: ApplyResponse;
      try {
        res = await this.agent.apply({
          txnId: '',
          confirmTxnId: p.txnId,
          subsystems: [],
          confirmTimeoutSec: 0,
          desiredState: undefined,
        });
      } catch (e) {
        if (e instanceof ProblemError && e.extra['grpcCode'] === 'FAILED_PRECONDITION') {
          await this.resolveReverted(p.txnId, 'confirm');
          throw problems.conflict(
            'commit-reverted',
            `transaction ${p.txnId} is no longer pending (it was reverted)`,
          );
        }
        throw e;
      }
      if (res.status !== ApplyStatus.APPLY_STATUS_CONFIRMED) {
        throw new ProblemError(
          502,
          'agent-error',
          'Unexpected agent answer',
          `confirm returned ${statusName(res.status)}`,
        );
      }
      const meta = {
        authorId: p.authorId,
        comment: p.comment,
        kind: p.kind,
        clearPending: true,
      };
      const revision = await this.promoteOrLoseTrack(p.txnId, p.payload, meta);
      this.bus.publish('commit.events', {
        type: 'confirmed',
        txnId: p.txnId,
        revision: revision.id,
        by: user.username,
      });
      await this.events.record(
        'info',
        'commit',
        'COMMIT_CONFIRMED',
        `revision ${revision.id} confirmed`,
        {
          txnId: p.txnId,
          revision: revision.id,
        },
      );
      return {
        status: 'confirmed',
        txnId: p.txnId,
        revision,
        results: [],
        warnings: [],
        notApplied: [],
        sync: syncJson(this.sync),
      };
    });
  }

  private async assertNoPending(): Promise<void> {
    const p = await this.repo.pending();
    if (p !== null) {
      throw problems.conflict(
        'commit-pending',
        `commit ${p.txnId} waits for confirmation until ${p.deadline.toISOString()}; confirm it or let it revert`,
        { pending: pendingJson(p) },
      );
    }
  }

  /**
   * Review M1: the COMMIT is authorised on running → document, whoever staged the changes. Users, AAA and secret
   * references need an admin.
   */
  private async assertMayApply(user: Principal, running: Doc, doc: Doc): Promise<void> {
    if (user.role === 'admin') return;
    const hashes = await this.repo.userHashes();
    const denied = privilegedChanges(hydrateHashes(running, hashes), hydrateHashes(doc, hashes));
    if (denied.length > 0) {
      throw problems.forbidden(
        `role '${user.role}' may not apply changes to users, AAA or secret references (${denied.join(', ')}); an admin must commit them`,
        denied.map((p) => ({ pointer: p, message: 'admin only' })),
      );
    }
  }

  private async applyDocument(
    user: Principal,
    doc: Doc,
    opts: CommitOptions & {
      kind: string;
      parentId: number | null;
      restoreSecrets?: Record<string, number>;
    },
  ): Promise<CommitResult> {
    const running = await this.repo.latestRevision();
    const runningDoc = running?.payload ?? emptyDocument();
    await this.assertMayApply(user, runningDoc, doc);
    const txnId = randomUUID();
    const v = await this.validation.validate(doc, txnId);
    if (!v.ok || v.config === undefined || v.desired === undefined) {
      throw problems.validation(v.errors, `${v.tier} validation failed`, {
        tier: v.tier,
        warnings: v.warnings,
      });
    }
    const notApplied = notAppliedChanges(runningDoc, v.config, v.notApplied);
    // D-097 (review H1): what is stored/promoted carries only hashes the raw document staged, never hydrated ones
    const stored = stagedHashesOnly(v.config as Doc, doc);
    const confirmSec = opts.confirmSec ?? 0;
    const meta = {
      authorId: user.id,
      comment: opts.comment ?? '',
      kind: opts.kind,
      clearPending: false,
      ...(opts.restoreSecrets ? { restoreSecrets: opts.restoreSecrets } : {}),
    };
    let res: ApplyResponse;
    try {
      res = await this.agent.apply({
        txnId,
        desiredState: v.desired,
        subsystems: v.subsystems,
        confirmTimeoutSec: confirmSec,
        confirmTxnId: '',
      });
    } catch (e) {
      if (!outcomeUnknown(e)) throw e;
      const p = e as ProblemError;
      const sync = await this.lostTrack(
        'unknown',
        `no answer to Apply ${txnId}: ${p.detail ?? String(e)}`,
        confirmSec > 0 ? undefined : { txnId, config: stored, meta },
      );
      throw new ProblemError(
        p instanceof ProblemError ? p.getStatus() : 502,
        'running-unknown',
        'Outcome unknown',
        `the agent did not answer Apply ${txnId}; it may have been applied. Running is marked UNKNOWN and a reconcile has started (GET /api/v1/state/system)`,
        undefined,
        { txnId, sync },
      );
    }
    const results = res.results.map(resultJson);
    const summary = res.summary ? { ...res.summary } : undefined;
    if (res.status !== ApplyStatus.APPLY_STATUS_APPLIED) {
      const degraded = res.status === ApplyStatus.APPLY_STATUS_DEGRADED;
      await this.events.record(
        degraded ? 'error' : 'warning',
        'commit',
        `COMMIT_${statusName(res.status).toUpperCase()}`,
        res.message || `apply ${statusName(res.status)}`,
        { txnId, results },
      );
      this.bus.publish('commit.events', { type: 'failed', txnId, status: statusName(res.status) });
      const errors: ProblemIssue[] = [
        ...(res.validation?.errors ?? []).map((i) => ({
          pointer: i.pointer,
          message: i.message,
          ...(i.rule ? { rule: i.rule } : {}),
        })),
        ...res.results
          .filter((r) => r.code !== 1 && r.code !== 4)
          .map((r) => ({
            pointer: r.pointer,
            message: `${r.key}: ${r.message || CODE_NAMES[r.code]}`,
          })),
      ];
      const sync = degraded
        ? await this.lostTrack(
            'degraded',
            `Apply ${txnId} answered DEGRADED: the data plane is partially changed`,
            undefined,
          )
        : syncJson(this.sync);
      throw new ProblemError(
        422,
        'apply-failed',
        'Apply failed',
        degraded
          ? `the agent answered degraded${res.message ? `: ${res.message}` : ''}; the data plane is partially changed, running is marked DEGRADED and a reconcile (re-apply of running) has started`
          : `the agent answered ${statusName(res.status)}${res.message ? `: ${res.message}` : ''}; the data plane was rolled back, running is unchanged`,
        errors,
        { txnId, applyStatus: statusName(res.status), results, summary, sync },
      );
    }
    const base = {
      txnId,
      results,
      warnings: v.warnings,
      notApplied,
      ...(summary ? { summary } : {}),
    };
    if (confirmSec > 0) {
      const deadline = res.confirmDeadline ?? new Date(Date.now() + confirmSec * 1000);
      await this.repo.tx((tx) =>
        tx.setPending({
          txnId,
          payload: stored,
          hash: documentHash(redact(v.config)),
          authorId: user.id,
          comment: opts.comment ?? '',
          parentId: opts.parentId,
          kind: opts.kind,
          deadline,
        }),
      );
      this.watch(deadline);
      this.bus.publish('commit.events', {
        type: 'pending',
        txnId,
        deadline: deadline.toISOString(),
        by: user.username,
      });
      await this.events.record(
        'info',
        'commit',
        'COMMIT_PENDING',
        `commit ${txnId} applied, confirm by ${deadline.toISOString()}`,
        { txnId },
      );
      return {
        status: 'pending',
        confirmDeadline: deadline.toISOString(),
        ...base,
        sync: syncJson(this.sync),
      };
    }
    const revision = await this.promoteOrLoseTrack(txnId, stored, meta);
    const changedDomains = new Set(
      diff(redact(runningDoc), redact(v.config)).map((c) => parsePointer(c.pointer)[0]),
    );
    const status: CommitStatus =
      notApplied.length === 0
        ? 'applied'
        : notApplied.length === changedDomains.size
          ? 'not-applied'
          : 'partially-applied';
    this.bus.publish('commit.events', {
      type: 'applied',
      status,
      txnId,
      revision: revision.id,
      by: user.username,
    });
    await this.events.record(
      notApplied.length > 0 ? 'warning' : 'info',
      'commit',
      notApplied.length > 0 ? 'COMMIT_NOT_ENFORCED' : 'COMMIT_APPLIED',
      notApplied.length > 0
        ? `revision ${revision.id} (${opts.kind}) stored; not enforced by the agent: ${notApplied.join(', ')}`
        : `revision ${revision.id} (${opts.kind})`,
      { txnId, revision: revision.id, notApplied },
    );
    return { status, revision, ...base, sync: syncJson(this.sync) };
  }

  /** promote(); if the revision cannot be saved, the data plane is ahead of running → UNKNOWN + reconcile (M3). */
  private async promoteOrLoseTrack(
    txnId: string,
    config: Doc,
    meta: InFlight['meta'] & { restoreSecrets?: Record<string, number> },
  ): Promise<RevisionMeta> {
    try {
      const rev = await this.promote(config, { ...meta, txnId });
      if (this.sync.state !== 'in-sync')
        await this.setSync('in-sync', `revision ${rev.id} applied`, txnId);
      return rev;
    } catch (e) {
      const sync = await this.lostTrack(
        'unknown',
        `transaction ${txnId} was applied but its revision could not be saved: ${(e as Error).message}`,
        { txnId, config, meta },
      );
      throw new ProblemError(
        500,
        'running-unknown',
        'Revision not saved',
        `the agent applied ${txnId} but the revision could not be saved; running is marked UNKNOWN and a reconcile has started`,
        undefined,
        { txnId, sync },
      );
    }
  }

  /**
   * Persist the applied document as the new running revision (redacted, D-046) with the secret versions it uses,
   * make app_user follow management.users, and clear the candidate when it is what was applied. D-102: an existing
   * user whose hash the document changed gets admin-reset semantics — in this transaction (generation, keys) and,
   * once it committed, sessions ended and one audit row per user (`via: config`).
   */
  private async promote(
    config: Doc,
    meta: {
      authorId: number | null;
      comment: string;
      kind: string;
      txnId: string;
      clearPending: boolean;
      restoreSecrets?: Record<string, number>;
    },
  ): Promise<RevisionMeta> {
    const { rev, resets } = await this.repo.tx(async (tx) => {
      const running = await tx.latestRevision();
      const payload = redact(config);
      if (meta.restoreSecrets && Object.keys(meta.restoreSecrets).length > 0) {
        const restored = await tx.restoreSecretVersions(meta.restoreSecrets);
        if (restored.length > 0) {
          void this.events.record(
            'info',
            'secrets',
            'SECRETS_RESTORED',
            `rollback restored ${restored.join(', ')}`,
          );
        }
      }
      const refs = [...new Set(secretRefs(payload).map((r) => r.ref))];
      const secretVersions = refs.length > 0 ? await tx.secretVersions(refs) : null;
      // TD-2 #6: the (redacted) revision records WHICH secret leaves it changed, before app_user follows it
      const hashes = await tx.userHashes();
      const changedSecrets = secretChanges(
        hydrateHashes(running?.payload ?? emptyDocument(), hashes),
        hydrateHashes(config, hashes),
      );
      const revision = await tx.insertRevision({
        secretChanges: changedSecrets,
        authorId: meta.authorId,
        comment: meta.comment,
        parentId: running?.id ?? null,
        payload,
        hash: documentHash(payload),
        txnId: meta.txnId,
        kind: meta.kind,
        secretVersions:
          meta.restoreSecrets && refs.length > 0
            ? { ...secretVersions, ...pick(meta.restoreSecrets, refs) }
            : secretVersions,
      });
      const management = config['management'] as { users?: UserConfig[] } | undefined;
      const resets = await tx.syncUsers(management?.users ?? []);
      if (meta.clearPending) await tx.setPending(null);
      const c = await tx.lockCandidate();
      if (c.payload === null || deepEqual(redact(c.payload), payload)) {
        await tx.saveCandidate({
          ownerId: null,
          ownerKeyId: null,
          lockedAt: null,
          payload: null,
          baseRevisionId: null,
        });
      } else {
        await tx.saveCandidate({
          ownerId: c.ownerId,
          ownerKeyId: c.ownerKeyId,
          lockedAt: c.lockedAt,
          payload: c.payload,
          baseRevisionId: revision.id,
        });
      }
      const { payload: stored, ...metaOut } = revision;
      void stored;
      this.bus.sessions({ usersChanged: true });
      return { rev: metaOut, resets };
    });
    if (resets.length > 0) await this.configResets(resets, rev, meta.txnId);
    return rev;
  }

  /**
   * D-102 after the promote committed: the sessions of every user whose hash the config API changed end (no kept
   * session, no keepApiKeys — the committer's own session too when they changed their own hash this way), and each
   * reset is audited on its own row. D-100 (3), TD-4: the same for a user the promote disabled (access tokens,
   * refresh chains and WebSockets end now, not at TTL; API keys stay and are refused while disabled), audited as
   * `config.user-disabled`. Hash change + disable in one promote: one revocation, two rows. Never throws: the
   * revision is saved, and the generation in PostgreSQL already refuses the old refresh chains and key creation.
   */
  private async configResets(
    resets: PasswordReset[],
    rev: RevisionMeta,
    txnId: string,
  ): Promise<void> {
    for (const r of resets) {
      try {
        const revoked = await this.tokens.revokeUser(r.userId, r.gen);
        if (r.reasons.includes('password'))
          await this.audit.write({
            userId: rev.authorId,
            username: rev.author,
            sourceIp: null,
            action: 'config.password-reset',
            resource: `user/${r.username}`,
            after: {
              passwordSet: true,
              self: rev.authorId === r.userId,
              via: 'config',
              revision: rev.id,
              txnId,
              apiKeysRevoked: r.apiKeysRevoked,
              ...(r.discardedCandidate ? { discardedCandidate: true } : {}),
              ...(revoked.persisted ? {} : { revocationPersisted: false }),
            },
            result: 'success',
            status: null,
          });
        if (r.reasons.includes('disabled'))
          await this.audit.write({
            userId: rev.authorId,
            username: rev.author,
            sourceIp: null,
            action: 'config.user-disabled',
            resource: `user/${r.username}`,
            after: {
              disabled: true,
              via: 'config',
              revision: rev.id,
              txnId,
              ...(revoked.persisted ? {} : { revocationPersisted: false }),
            },
            result: 'success',
            status: null,
          });
      } catch (e) {
        this.log.error(
          `config-path revocation (${r.reasons.join('+')}) of '${r.username}': ${(e as Error).message}`,
        );
      }
    }
  }

  /** Re-arm the revert watcher for the pending commit (also after an API restart). */
  async resumePending(): Promise<void> {
    const p = await this.repo.pending();
    if (p !== null) this.watch(p.deadline);
  }

  private watch(deadline: Date): void {
    if (this.watchTimer) clearTimeout(this.watchTimer);
    const delay = Math.max(0, deadline.getTime() - Date.now()) + REVERT_GRACE_MS;
    this.watchTimer = setTimeout(() => void this.checkPending(), delay);
    this.watchTimer.unref();
  }

  /** After the deadline: ask the agent whether the pending transaction is still pending; if not, it was reverted. */
  private async checkPending(): Promise<void> {
    this.watchTimer = undefined;
    const p = await this.repo.pending().catch(() => null);
    if (p === null) return;
    try {
      const h = await this.agent.health();
      if (h.pendingConfirmTxnId !== p.txnId) await this.onReverted(p.txnId, 'health');
      else this.watch(new Date(Date.now() + 1000));
    } catch {
      // agent unreachable: keep watching; the agent reverts on its own and we learn it when it is back
      this.watch(new Date(Date.now() + 5000));
    }
  }

  private onReverted(txnId: string, via: string): Promise<void> {
    return this.mutex.run(() => this.resolveReverted(txnId, via));
  }

  /** The agent reverted the pending transaction: drop it; running and the candidate stay as they were. */
  private async resolveReverted(txnId: string, via: string): Promise<void> {
    const dropped = await this.repo.tx(async (tx) => {
      const p = await tx.pending();
      if (p === null || p.txnId !== txnId) return false;
      await tx.setPending(null);
      return true;
    });
    if (!dropped) return;
    this.log.warn(`commit ${txnId} was not confirmed and has been reverted by the agent (${via})`);
    this.bus.publish('commit.events', { type: 'reverted', txnId });
    await this.events.record(
      'warning',
      'commit',
      'CONFIRM_REVERTED',
      `commit ${txnId} was not confirmed and was reverted`,
      { txnId, via },
    );
  }
}

function pick(m: Record<string, number>, keys: readonly string[]): Record<string, number> {
  return Object.fromEntries(Object.entries(m).filter(([k]) => keys.includes(k)));
}

function pendingJson(p: PendingCommit): PendingInfo {
  return {
    txnId: p.txnId,
    author: p.authorId,
    comment: p.comment,
    kind: p.kind,
    deadline: p.deadline.toISOString(),
    createdAt: p.createdAt.toISOString(),
  };
}
