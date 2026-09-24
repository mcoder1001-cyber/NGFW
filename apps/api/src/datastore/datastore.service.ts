import { Inject, Injectable } from '@nestjs/common';
import { diff, mergePatchAt, type Change } from '@ngfw/schema';
import { ENV, type Env } from '../config.js';
import { getAt, PointerNotFoundError, removeAt, setAt } from '../common/json.js';
import { problems } from '../common/problem.js';
import { newUnsafeTextIssues, UNSAFE_TEXT_MESSAGE } from '../common/text.js';
import type { Principal } from '../common/principal.js';
import {
  hydrateHashes,
  secretChanges,
  type SecretChange,
  withoutPasswordHashes,
  privilegedChanges,
  emptyDocument,
  parseDocument,
  preserveSecrets,
  redact,
} from './documents.js';
import { checkLock, lockInfo, lockOwnerOf, type LockInfo } from './lock.js';
import type { ConfigRepo, Doc, Revision } from './repo.js';

export const CONFIG_REPO = Symbol('VRX_CONFIG_REPO');

export interface Running {
  revision: Revision | null;
  /** Redacted (revisions never carry secrets). */
  doc: Doc;
}

/** A diff entry: a redacted-document change, or a value-free secret-leaf change (TD-2 #6). */
export type DiffChange = Change | SecretChange;

/** Both lists merged in pointer order (a secret entry follows the redacted change of its container). */
export function withSecretChanges(changes: Change[], secrets: SecretChange[]): DiffChange[] {
  return [...changes, ...secrets].sort((a, b) =>
    a.pointer < b.pointer ? -1 : a.pointer > b.pointer ? 1 : 0,
  );
}

export interface EditResult {
  pointer: string;
  /** Redacted subtree before/after — for the response and the audit log. */
  before: unknown;
  after: unknown;
  /** Secret leaves this edit changed, without values (TD-2 #6). */
  secretChanges?: SecretChange[];
  /** Import only: password hashes in the imported document that were ignored (D-097). */
  ignoredSecrets?: string[];
  /** Set when a stale candidate with admin-only changes was discarded on takeover (review M1). */
  discardedStaleCandidateOf?: string;
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

  /** What revision `id` changed against its parent: redacted diff + its recorded secret changes (TD-2 #6). */
  async revisionDiff(
    id: number,
  ): Promise<{ revision: number; parent: number | null; changes: DiffChange[] }> {
    const r = await this.getRevision(id);
    const parent = r.parentId === null ? null : await this.repo.revision(r.parentId);
    return {
      revision: r.id,
      parent: parent?.id ?? null,
      changes: withSecretChanges(
        diff(parent?.payload ?? emptyDocument(), r.payload),
        r.secretChanges ?? [],
      ),
    };
  }

  async lock(): Promise<LockInfo> {
    return lockInfo(await this.repo.candidate(), this.ttl);
  }

  /**
   * Structured candidate ↔ running diff of the redacted documents (D-046), plus one value-free
   * `{op, pointer, redacted: true}` per changed secret leaf (TD-2 #6) — a password-hash-only edit is a change too.
   */
  async diff(): Promise<{ baseRevision: number | null; changes: DiffChange[] }> {
    const [running, c] = await Promise.all([this.getRunning(), this.repo.candidate()]);
    if (c.payload === null) return { baseRevision: running.revision?.id ?? null, changes: [] };
    const hashes = await this.repo.userHashes();
    return {
      baseRevision: running.revision?.id ?? null,
      changes: withSecretChanges(
        diff(running.doc, redact(c.payload)),
        secretChanges(hydrateHashes(running.doc, hashes), hydrateHashes(c.payload, hashes)),
      ),
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

  /**
   * Import a whole document into the candidate (never applied, P06 §4). D-097: a snapshot never brings password
   * hashes — they are dropped (listed in `ignoredSecrets`); users get passwords through /users/{name}/password.
   */
  async importCandidate(user: Principal, doc: unknown): Promise<EditResult> {
    const { doc: clean, removed } = withoutPasswordHashes(doc);
    const r = await this.edit(user, '', () => clean);
    if (removed.length > 0) r.ignoredSecrets = removed;
    return r;
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
      const runningDoc = running?.payload ?? emptyDocument();
      let staged = c.payload;
      let discarded = false;
      if (staged !== null && decision === 'stale' && user.role !== 'admin') {
        // review M1: a lower role taking over a stale lock must not inherit staged admin-only changes
        const hashes = await this.repo.userHashes();
        if (
          privilegedChanges(hydrateHashes(runningDoc, hashes), hydrateHashes(staged, hashes))
            .length > 0
        ) {
          staged = null;
          discarded = true;
        }
      }
      const base = staged ?? runningDoc;
      const mutated = mutate(base);
      // TD-2 #4 (D-049): no control characters / bidi overrides in any string or member name the edit adds. Checked
      // before the schema so no problem message ever quotes such a key raw. Pre-existing ones do not block edits.
      const unsafe = newUnsafeTextIssues(base, mutated);
      if (unsafe.length > 0) throw problems.validation(unsafe, UNSAFE_TEXT_MESSAGE);
      const next = preserveSecrets(base, parseDocument(mutated));
      if (user.role !== 'admin') {
        const denied = privilegedChanges(base, next);
        if (denied.length > 0) {
          throw problems.forbidden(
            `role '${user.role}' may not change users, AAA or secret references (${denied.join(', ')})`,
            denied.map((p) => ({ pointer: p, message: 'admin only' })),
          );
        }
      }
      await tx.saveCandidate({
        ...lockOwnerOf(user),
        lockedAt: decision === 'own' ? c.lockedAt : this.now(),
        payload: next,
        baseRevisionId: staged === null ? (running?.id ?? null) : c.baseRevisionId,
      });
      const r: EditResult = {
        pointer,
        before: getAt(redact(base), pointer),
        after: getAt(redact(next), pointer),
      };
      const hashes = await this.repo.userHashes();
      const secrets = secretChanges(hydrateHashes(base, hashes), hydrateHashes(next, hashes));
      if (secrets.length > 0) r.secretChanges = secrets;
      if (discarded) r.discardedStaleCandidateOf = c.owner ?? `user #${c.ownerId}`;
      return r;
    });
  }

  /** Drop the candidate and release the lock (owner, a free lock, or a stale one). */
  async discard(user: Principal): Promise<{ discarded: boolean }> {
    return this.repo.tx(async (tx) => {
      const c = await tx.lockCandidate();
      checkLock(c, user, this.now(), this.ttl);
      await tx.saveCandidate({
        ownerId: null,
        ownerKeyId: null,
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
        ownerKeyId: null,
        lockedAt: null,
        payload: null,
        baseRevisionId: null,
      });
      return lockInfo(c, this.ttl);
    });
  }
}
