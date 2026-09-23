import { Inject, Injectable } from '@nestjs/common';
import { diff, mergePatchAt, type Change } from '@ngfw/schema';
import { ENV, type Env } from '../config.js';
import { getAt, PointerNotFoundError, removeAt, setAt } from '../common/json.js';
import { problems } from '../common/problem.js';
import type { Principal } from '../common/principal.js';
import {
  adminOnlyChanges,
  emptyDocument,
  parseDocument,
  preserveSecrets,
  redact,
} from './documents.js';
import { checkLock, lockInfo, type LockInfo } from './lock.js';
import type { ConfigRepo, Doc, Revision } from './repo.js';

export const CONFIG_REPO = Symbol('VRX_CONFIG_REPO');

export interface Running {
  revision: Revision | null;
  /** Redacted (revisions never carry secrets). */
  doc: Doc;
}

export interface EditResult {
  pointer: string;
  /** Redacted subtree before/after — for the response and the audit log. */
  before: unknown;
  after: unknown;
}

/**
 * Candidate/running datastore (P06 §2). One candidate document, one writer: the first edit takes the lock (owner +
 * timestamp), other users get 409 with the owner until the owner commits or discards, or the lock goes stale
 * (VRX_LOCK_TTL_SEC). Every stored candidate is schema-valid (tier a on the write path); semantic and agent
 * validation run on validate/commit. Nothing here talks to the agent.
 */
@Injectable()
export class DatastoreService {
  /** Clock (tests move it to age a lock). */
  now: () => Date = () => new Date();

  constructor(
    @Inject(CONFIG_REPO) private readonly repo: ConfigRepo,
    @Inject(ENV) private readonly env: Env,
  ) {}

  private get ttl(): number {
    return this.env.VRX_LOCK_TTL_SEC;
  }

  async getRunning(): Promise<Running> {
    const revision = await this.repo.latestRevision();
    return { revision, doc: revision?.payload ?? emptyDocument() };
  }

  /** The candidate as clients see it (redacted); equals running when nobody is editing. */
  async getCandidate(): Promise<Doc> {
    const c = await this.repo.candidate();
    if (c.payload !== null) return redact(c.payload);
    return (await this.getRunning()).doc;
  }

  listRevisions(limit: number, offset: number) {
    return this.repo.listRevisions(limit, offset);
  }

  async getRevision(id: number): Promise<Revision> {
    const r = await this.repo.revision(id);
    if (r === null) throw problems.notFound(`revision ${id} does not exist`);
    return r;
  }

  async lock(): Promise<LockInfo> {
    return lockInfo(await this.repo.candidate(), this.ttl);
  }

  /** Structured candidate ↔ running diff of the redacted documents (D-046). */
  async diff(): Promise<{ baseRevision: number | null; changes: Change[] }> {
    const [running, c] = await Promise.all([this.getRunning(), this.repo.candidate()]);
    if (c.payload === null) return { baseRevision: running.revision?.id ?? null, changes: [] };
    return {
      baseRevision: running.revision?.id ?? null,
      changes: diff(running.doc, redact(c.payload)),
    };
  }

  /** RFC 7386 merge patch at `pointer`. */
  patchCandidate(user: Principal, pointer: string, patch: unknown): Promise<EditResult> {
    return this.edit(user, pointer, (doc) => mergePatchAt(doc, pointer, patch));
  }

  /** Replace the node at `pointer` (created when absent). */
  putCandidate(user: Principal, pointer: string, value: unknown): Promise<EditResult> {
    return this.edit(user, pointer, (doc) => setAt(doc, pointer, value));
  }

  /** Remove the node at `pointer`; 404 when there is nothing. */
  deleteCandidate(user: Principal, pointer: string): Promise<EditResult> {
    return this.edit(user, pointer, (doc) => {
      try {
        return removeAt(doc, pointer);
      } catch (e) {
        if (e instanceof PointerNotFoundError)
          throw problems.notFound(`nothing at '${pointer}' in the candidate`);
        throw e;
      }
    });
  }

  /** Import a whole document into the candidate (never applied, P06 §4). */
  importCandidate(user: Principal, doc: unknown): Promise<EditResult> {
    return this.edit(user, '', () => doc);
  }

  private edit(
    user: Principal,
    pointer: string,
    mutate: (doc: Doc) => unknown,
  ): Promise<EditResult> {
    return this.repo.tx(async (tx) => {
      const c = await tx.lockCandidate();
      const decision = checkLock(c, user, this.now(), this.ttl);
      const running = await tx.latestRevision();
      const base = c.payload ?? running?.payload ?? emptyDocument();
      const next = preserveSecrets(base, parseDocument(mutate(base)));
      if (user.role !== 'admin') {
        const denied = adminOnlyChanges(base, next);
        if (denied.length > 0) {
          throw problems.forbidden(
            `role '${user.role}' may not change users or AAA (${denied.join(', ')})`,
            denied.map((p) => ({ pointer: p, message: 'admin only' })),
          );
        }
      }
      await tx.saveCandidate({
        ownerId: user.id,
        lockedAt: decision === 'own' ? c.lockedAt : this.now(),
        payload: next,
        baseRevisionId: c.payload === null ? (running?.id ?? null) : c.baseRevisionId,
      });
      return { pointer, before: getAt(redact(base), pointer), after: getAt(redact(next), pointer) };
    });
  }

  /** Drop the candidate and release the lock (owner, a free lock, or a stale one). */
  async discard(user: Principal): Promise<{ discarded: boolean }> {
    return this.repo.tx(async (tx) => {
      const c = await tx.lockCandidate();
      checkLock(c, user, this.now(), this.ttl);
      await tx.saveCandidate({
        ownerId: null,
        lockedAt: null,
        payload: null,
        baseRevisionId: null,
      });
      return { discarded: c.payload !== null };
    });
  }

  /** Admin: break somebody else's lock; the candidate is discarded with it. */
  async breakLock(): Promise<LockInfo> {
    return this.repo.tx(async (tx) => {
      const c = await tx.lockCandidate();
      await tx.saveCandidate({
        ownerId: null,
        lockedAt: null,
        payload: null,
        baseRevisionId: null,
      });
      return lockInfo(c, this.ttl);
    });
  }
}
