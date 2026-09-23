import type { UserConfig } from '@ngfw/schema';
import type {
  CandidateState,
  ConfigRepo,
  ConfigTx,
  NewRevision,
  PendingCommit,
  Revision,
  RevisionMeta,
} from '../datastore/repo.js';

/**
 * In-memory ConfigRepo for unit tests: transactions are serialised by a promise chain (the `FOR UPDATE` of the
 * PostgreSQL implementation) and roll back on error by working on a copy of the state.
 */
interface State {
  revisions: Revision[];
  candidate: Omit<CandidateState, 'owner'>;
  pending: PendingCommit | null;
  users: Map<
    string,
    { id: number; role: string; disabled: boolean; hash: string | null; source: string }
  >;
  secrets: Set<string>;
}

export class MemoryConfigRepo implements ConfigRepo {
  state: State = {
    revisions: [],
    candidate: {
      ownerId: null,
      lockedAt: null,
      payload: null,
      baseRevisionId: null,
      updatedAt: new Date(0),
    },
    pending: null,
    users: new Map(),
    secrets: new Set(),
  };
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
        syncUsers: async (users: readonly UserConfig[]) => {
          const names = new Set(users.map((u) => u.username));
          for (const [name, u] of s.users)
            if (u.source === 'config' && !names.has(name)) s.users.delete(name);
          for (const u of users) {
            const prev = s.users.get(u.username);
            s.users.set(u.username, {
              id: prev?.id ?? this.nextUserId++,
              role: u.role,
              disabled: u.disabled,
              hash: u.passwordHash ?? prev?.hash ?? null,
              source: prev?.source ?? 'config',
            });
          }
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

  async existingSecretRefs(refs: readonly string[]): Promise<Set<string>> {
    return new Set(refs.filter((r) => this.state.secrets.has(r)));
  }
}
