import { Inject, Injectable, Logger, type OnApplicationShutdown } from '@nestjs/common';
import { ApplyStatus, EventKind, type ApplyResponse, type ObjectResult } from '@ngfw/proto';
import { deepEqual, type UserConfig } from '@ngfw/schema';
import { randomUUID } from 'node:crypto';
import { AgentClient } from '../agent/agent.client.js';
import { SystemEventsService } from '../audit/system-events.service.js';
import { ENV, type Env } from '../config.js';
import { documentHash } from '../common/json.js';
import { Mutex } from '../common/mutex.js';
import { ProblemError, problems, type ProblemIssue } from '../common/problem.js';
import type { Principal } from '../common/principal.js';
import { CONFIG_REPO } from '../datastore/datastore.service.js';
import { adminOnlyChanges, hydrateHashes, redact } from '../datastore/documents.js';
import { checkLock } from '../datastore/lock.js';
import type { ConfigRepo, Doc, PendingCommit, RevisionMeta } from '../datastore/repo.js';
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

export interface CommitResult {
  status: 'applied' | 'pending' | 'unchanged' | 'confirmed';
  txnId?: string;
  revision?: RevisionMeta;
  confirmDeadline?: string;
  results: ObjectResultJson[];
  summary?: Record<string, number>;
  warnings: ProblemIssue[];
  /** Top-level keys the agent does not implement yet: stored in running, not applied. */
  notApplied: string[];
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

/** How long after a confirm deadline the API waits before asking the agent whether it reverted. */
const REVERT_GRACE_MS = 1500;

/**
 * The commit engine (P06 §4): validate (3 tiers) → agent Apply(txn) → on APPLIED persist a revision (full redacted
 * snapshot, sha256, author, comment, parent) and promote the candidate; FAILED/ROLLED_BACK/DEGRADED → 422 with the
 * per-object results, running untouched. Confirmed commits are held in `config_pending` until CONFIRMED; the agent
 * reverts on its own (proto.md §4) and the API learns it from the CONFIRM_REVERTED event or from Health after the
 * deadline. Commit, confirm and rollback are serialised in-process and on the candidate row.
 */
@Injectable()
export class CommitService implements OnApplicationShutdown {
  private readonly log = new Logger('Commit');
  private readonly mutex = new Mutex();
  private watchTimer: NodeJS.Timeout | undefined;
  private readonly unsubscribe: () => void;

  constructor(
    @Inject(CONFIG_REPO) private readonly repo: ConfigRepo,
    private readonly validation: ValidationService,
    private readonly agent: AgentClient,
    private readonly events: SystemEventsService,
    private readonly bus: Bus,
    @Inject(ENV) private readonly env: Env,
  ) {
    this.unsubscribe = this.bus.onAgentEvent((e) => {
      if (e.kind === EventKind.EVENT_KIND_CONFIRM_REVERTED && e.txnId)
        void this.onReverted(e.txnId, 'event');
    });
  }

  onApplicationShutdown(): void {
    this.unsubscribe();
    if (this.watchTimer) clearTimeout(this.watchTimer);
  }

  /** Validate the candidate without applying: 200 with plan/warnings, 400 with pointers. */
  async validateCandidate(): Promise<{
    ok: true;
    warnings: ProblemIssue[];
    plan: PlanEntry[];
    notApplied: string[];
  }> {
    const c = await this.repo.candidate();
    const doc = c.payload ?? (await this.repo.latestRevision())?.payload;
    if (doc === undefined) return { ok: true, warnings: [], plan: [], notApplied: [] };
    const v = await this.validation.validate(doc, `validate-${randomUUID()}`);
    if (!v.ok)
      throw problems.validation(v.errors, `${v.tier} validation failed`, {
        tier: v.tier,
        warnings: v.warnings,
      });
    return { ok: true, warnings: v.warnings, plan: v.plan, notApplied: v.notApplied };
  }

  commit(user: Principal, opts: CommitOptions): Promise<CommitResult> {
    return this.mutex.run(async () => {
      await this.assertNoPending();
      const c = await this.repo.candidate();
      checkLock(c, user, new Date(), this.env.VRX_LOCK_TTL_SEC);
      if (c.payload === null)
        return { status: 'unchanged', results: [], warnings: [], notApplied: [] };
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
      if (user.role !== 'admin') {
        const hashes = await this.repo.userHashes();
        const denied = adminOnlyChanges(
          hydrateHashes(running?.payload ?? {}, hashes),
          hydrateHashes(target.payload, hashes),
        );
        if (denied.length > 0) {
          throw problems.forbidden(
            `role '${user.role}' may not roll back users or AAA (${denied.join(', ')})`,
          );
        }
      }
      return this.applyDocument(user, target.payload, {
        ...opts,
        comment: opts.comment || `rollback to revision ${rev}`,
        kind: 'rollback',
        parentId: running?.id ?? null,
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
      const revision = await this.promote(p.payload, {
        authorId: p.authorId,
        comment: p.comment,
        kind: p.kind,
        txnId: p.txnId,
        clearPending: true,
      });
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

  private async applyDocument(
    user: Principal,
    doc: Doc,
    opts: CommitOptions & { kind: string; parentId: number | null },
  ): Promise<CommitResult> {
    const txnId = randomUUID();
    const v = await this.validation.validate(doc, txnId);
    if (!v.ok || v.config === undefined || v.desired === undefined) {
      throw problems.validation(v.errors, `${v.tier} validation failed`, {
        tier: v.tier,
        warnings: v.warnings,
      });
    }
    const confirmSec = opts.confirmSec ?? 0;
    const res = await this.agent.apply({
      txnId,
      desiredState: v.desired,
      subsystems: v.subsystems,
      confirmTimeoutSec: confirmSec,
      confirmTxnId: '',
    });
    const results = res.results.map(resultJson);
    const summary = res.summary ? { ...res.summary } : undefined;
    if (res.status !== ApplyStatus.APPLY_STATUS_APPLIED) {
      await this.events.record(
        res.status === ApplyStatus.APPLY_STATUS_DEGRADED ? 'error' : 'warning',
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
      throw new ProblemError(
        422,
        'apply-failed',
        'Apply failed',
        `the agent answered ${statusName(res.status)}${res.message ? `: ${res.message}` : ''}; running is unchanged`,
        errors,
        { txnId, applyStatus: statusName(res.status), results, summary },
      );
    }
    const base = {
      txnId,
      results,
      warnings: v.warnings,
      notApplied: v.notApplied,
      ...(summary ? { summary } : {}),
    };
    if (confirmSec > 0) {
      const deadline = res.confirmDeadline ?? new Date(Date.now() + confirmSec * 1000);
      await this.repo.tx((tx) =>
        tx.setPending({
          txnId,
          payload: v.config as Doc,
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
      return { status: 'pending', confirmDeadline: deadline.toISOString(), ...base };
    }
    const revision = await this.promote(v.config, {
      authorId: user.id,
      comment: opts.comment ?? '',
      kind: opts.kind,
      txnId,
      clearPending: false,
    });
    this.bus.publish('commit.events', {
      type: 'applied',
      txnId,
      revision: revision.id,
      by: user.username,
    });
    await this.events.record(
      'info',
      'commit',
      'COMMIT_APPLIED',
      `revision ${revision.id} (${opts.kind})`,
      { txnId, revision: revision.id },
    );
    return { status: 'applied', revision, ...base };
  }

  /**
   * Persist the applied document as the new running revision (redacted, D-046), make app_user follow
   * management.users, and clear the candidate when it is what was applied (edits made meanwhile stay, rebased).
   */
  private promote(
    config: Doc,
    meta: {
      authorId: number | null;
      comment: string;
      kind: string;
      txnId: string;
      clearPending: boolean;
    },
  ): Promise<RevisionMeta> {
    return this.repo.tx(async (tx) => {
      const running = await tx.latestRevision();
      const payload = redact(config);
      const revision = await tx.insertRevision({
        authorId: meta.authorId,
        comment: meta.comment,
        parentId: running?.id ?? null,
        payload,
        hash: documentHash(payload),
        txnId: meta.txnId,
        kind: meta.kind,
      });
      const management = config['management'] as { users?: UserConfig[] } | undefined;
      await tx.syncUsers(management?.users ?? []);
      if (meta.clearPending) await tx.setPending(null);
      const c = await tx.lockCandidate();
      if (c.payload === null || deepEqual(redact(c.payload), payload)) {
        await tx.saveCandidate({
          ownerId: null,
          lockedAt: null,
          payload: null,
          baseRevisionId: null,
        });
      } else {
        await tx.saveCandidate({
          ownerId: c.ownerId,
          lockedAt: c.lockedAt,
          payload: c.payload,
          baseRevisionId: revision.id,
        });
      }
      const { payload: stored, ...metaOut } = revision;
      void stored;
      return metaOut;
    });
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
      {
        txnId,
        via,
      },
    );
  }
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
