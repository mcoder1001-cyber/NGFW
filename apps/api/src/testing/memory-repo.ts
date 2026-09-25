import type { UserConfig } from '@ngfw/schema';
import type {
  CandidateState,
  ConfigRepo,
  ConfigTx,
  NewRevision,
  PasswordReset,
  PendingCommit,
  Revision,
  RevisionMeta,
  SyncStatus,
} from '../datastore/repo.js';

/**
 * In-memory ConfigRepo for unit tests: transactions are serialised by a promise chain (the `FOR UPDATE` of the
 * PostgreSQL implementation) and roll back on error by working on a copy of the state.
 */
interface State {
  revisions: Revision[];
  candidate: Omit<CandidateState, 'owner' | 'ownerKey'>;
  pending: PendingCommit | null;
  users: Map<
    string,
    {
      id: number;
      role: string;
      disabled: boolean;
      hash: string | null;
      source: string;
      /** credential generation (D-102) */
      gen?: number;
    }
  >;
  secrets: Set<string>;
  /** ref → current version, and every stored version. */
  secretVersion: Map<string, number>;
  secretHistory: Set<string>;
  sync: SyncStatus;
}

export class MemoryConfigRepo implements ConfigRepo {
  state: State = {
    revisions: [],
    candidate: {
      ownerId: null,
      ownerKeyId: null,
      lockedAt: null,
      payload: null,
      baseRevisionId: null,
      updatedAt: new Date(0),
    },
    pending: null,
    users: new Map(),
    secrets: new Set(),
    secretVersion: new Map(),
    secretHistory: new Set(),
    sync: { state: 'in-sync', reason: '', txnId: null, since: new Date(0) },
  };
  /** Make the next N transactions fail (simulates a database outage during promote). */
  failNextTx = 0;
  private chain: Promise<unknown> = Promise.resolve();
  private nextUserId = 1;

  addUser(
    username: string,
    role: string,
    hash: string | null = null,
    source = 'bootstrap',
  ): number {
    const id = this.nextUserId++;
    this.state.users.set(username, { id, role, disabled: false, hash, source });
    return id;
  }

  private usernameOf(s: State, id: number | null): string | null {
    for (const [name, u] of s.users) if (u.id === id) return name;
    return null;
  }

  private reads(s: State) {
    return {
      latestRevision: async () => structuredClone(s.revisions.at(-1) ?? null),
      revision: async (id: number) => structuredClone(s.revisions.find((r) => r.id === id) ?? null),
      candidate: async (): Promise<CandidateState> => ({
        ...structuredClone(s.candidate),
        owner: this.usernameOf(s, s.candidate.ownerId),
        ownerKey: s.candidate.ownerKeyId === null ? null : `key ${s.candidate.ownerKeyId}`,
      }),
      pending: async () => structuredClone(s.pending),
    };
  }

  latestRevision() {
    return this.reads(this.state).latestRevision();
  }
  revision(id: number) {
    return this.reads(this.state).revision(id);
  }
  candidate() {
    return this.reads(this.state).candidate();
  }
  pending() {
    return this.reads(this.state).pending();
  }

  tx<T>(fn: (tx: ConfigTx) => Promise<T>): Promise<T> {
    const run = async (): Promise<T> => {
      if (this.failNextTx > 0) {
        this.failNextTx -= 1;
        throw new Error('simulated database failure');
      }
      const s: State = structuredClone(this.state);
      const reads = this.reads(s);
      const tx: ConfigTx = {
        ...reads,
        lockCandidate: reads.candidate,
        saveCandidate: async (c) => {
          s.candidate = { ...structuredClone(c), updatedAt: new Date() };
        },
        insertRevision: async (r: NewRevision) => {
          const rev: Revision = {
            ...structuredClone(r),
            secretChanges: structuredClone(r.secretChanges ?? []),
            id: (s.revisions.at(-1)?.id ?? 0) + 1,
            createdAt: new Date(),
            author: this.usernameOf(s, r.authorId),
          };
          s.revisions.push(rev);
          return structuredClone(rev);
        },
        setPending: async (p) => {
          s.pending = p === null ? null : { ...structuredClone(p), createdAt: new Date() };
        },
        setSync: async (x) => {
          s.sync = { ...x, since: new Date() };
        },
        restoreSecretVersions: async (versions) => {
          const out: string[] = [];
          for (const [ref, v] of Object.entries(versions)) {
            if (s.secretHistory.has(`${ref}@${v}`) && s.secretVersion.get(ref) !== v) {
              s.secretVersion.set(ref, v);
              out.push(`${ref}@${v}`);
            }
          }
          return out;
        },
        syncUsers: async (users: readonly UserConfig[]) => {
          const names = new Set(users.map((u) => u.username));
          for (const [name, u] of s.users)
            if (u.source === 'config' && !names.has(name)) s.users.delete(name);
          const resets: PasswordReset[] = [];
          for (const u of users) {
            const prev = s.users.get(u.username);
            // D-102: an existing user's changed hash bumps the generation (no API keys in this repo)
            const password =
              prev !== undefined && u.passwordHash !== undefined && u.passwordHash !== prev.hash;
            // D-100 (3): so does an existing user this promote disables (once, when both happen)
            const disabled = prev !== undefined && !prev.disabled && u.disabled === true;
            const reset = password || disabled;
            const id = prev?.id ?? this.nextUserId++;
            const gen = (prev?.gen ?? 0) + (reset ? 1 : 0);
            s.users.set(u.username, {
              id,
              role: u.role,
              disabled: u.disabled,
              hash: u.passwordHash ?? prev?.hash ?? null,
              source: prev?.source ?? 'config',
              gen,
            });
            if (reset)
              resets.push({
                userId: id,
                username: u.username,
                gen,
                apiKeysRevoked: [],
                discardedCandidate: false,
                reasons: [
                  ...(password ? ['password' as const] : []),
                  ...(disabled ? ['disabled' as const] : []),
                ],
              });
          }
          return resets;
        },
      };
      const result = await fn(tx);
      this.state = s;
      return result;
    };
    const p = this.chain.then(run, run);
    this.chain = p.catch(() => undefined);
    return p;
  }

  async listRevisions(
    limit: number,
    offset: number,
  ): Promise<{ items: RevisionMeta[]; total: number }> {
    const all = [...this.state.revisions].reverse();
    const items = all.slice(offset, offset + limit).map(({ payload: _p, ...meta }) => meta);
    return { items, total: all.length };
  }

  async userHashes(): Promise<Map<string, string>> {
    const m = new Map<string, string>();
    for (const [name, u] of this.state.users) if (u.hash !== null) m.set(name, u.hash);
    return m;
  }

  /** Store a new version of a secret (tests). */
  putSecret(ref: string): number {
    const v = (this.state.secretVersion.get(ref) ?? 0) + 1;
    this.state.secrets.add(ref);
    this.state.secretVersion.set(ref, v);
    this.state.secretHistory.add(`${ref}@${v}`);
    return v;
  }

  async secretVersions(refs: readonly string[]): Promise<Record<string, number>> {
    const out: Record<string, number> = {};
    for (const r of refs) {
      const v = this.state.secretVersion.get(r);
      if (v !== undefined) out[r] = v;
    }
    return out;
  }

  async getSync(): Promise<SyncStatus> {
    return structuredClone(this.state.sync);
  }

  async setSync(x: Omit<SyncStatus, 'since'>): Promise<void> {
    this.state.sync = { ...x, since: new Date() };
  }

  async existingSecretRefs(refs: readonly string[]): Promise<Set<string>> {
    return new Set(refs.filter((r) => this.state.secrets.has(r)));
  }
}
