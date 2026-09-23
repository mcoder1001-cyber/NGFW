import { Inject, Injectable } from '@nestjs/common';
import type { UserConfig } from '@ngfw/schema';
import { and, count, desc, eq, inArray, notInArray, sql } from 'drizzle-orm';
import { DB, type Db, type DbTx } from '../db/db.js';
import {
  appUser,
  configCandidate,
  configPending,
  configRevision,
  secret,
  type Role,
} from '../db/schema.js';
import type {
  CandidateState,
  ConfigRepo,
  ConfigTx,
  Doc,
  NewRevision,
  PendingCommit,
  Revision,
  RevisionMeta,
} from './repo.js';

const CANDIDATE_ID = 1;
const PENDING_ID = 1;

type Exec = Db | DbTx;

const revisionMetaColumns = {
  id: configRevision.id,
  createdAt: configRevision.createdAt,
  authorId: configRevision.authorId,
  author: appUser.username,
  comment: configRevision.comment,
  parentId: configRevision.parentId,
  hash: configRevision.hash,
  txnId: configRevision.txnId,
  kind: configRevision.kind,
};

async function readRevision(db: Exec, id?: number): Promise<Revision | null> {
  const q = db
    .select({ ...revisionMetaColumns, payload: configRevision.payload })
    .from(configRevision)
    .leftJoin(appUser, eq(appUser.id, configRevision.authorId));
  const rows = await (id === undefined
    ? q.orderBy(desc(configRevision.id)).limit(1)
    : q.where(eq(configRevision.id, id)).limit(1));
  const r = rows[0];
  return r === undefined ? null : { ...r, payload: r.payload as Doc };
}

async function readCandidate(db: Exec, forUpdate: boolean): Promise<CandidateState> {
  // the singleton row always exists (created on first use); FOR UPDATE serialises every writer on it
  await db.insert(configCandidate).values({ id: CANDIDATE_ID }).onConflictDoNothing();
  const base = db
    .select({
      ownerId: configCandidate.ownerId,
      lockedAt: configCandidate.lockedAt,
      payload: configCandidate.payload,
      baseRevisionId: configCandidate.baseRevisionId,
      updatedAt: configCandidate.updatedAt,
    })
    .from(configCandidate)
    .where(eq(configCandidate.id, CANDIDATE_ID));
  const rows = await (forUpdate ? base.for('update') : base);
  const row = rows[0];
  if (row === undefined) throw new Error('config_candidate row missing');
  let owner: string | null = null;
  if (row.ownerId !== null) {
    const u = await db
      .select({ username: appUser.username })
      .from(appUser)
      .where(eq(appUser.id, row.ownerId));
    owner = u[0]?.username ?? null;
  }
  return { ...row, payload: (row.payload as Doc | null) ?? null, owner };
}

async function readPending(db: Exec): Promise<PendingCommit | null> {
  const rows = await db.select().from(configPending).where(eq(configPending.id, PENDING_ID));
  const r = rows[0];
  if (r === undefined) return null;
  return {
    txnId: r.txnId,
    payload: r.payload as Doc,
    hash: r.hash,
    authorId: r.authorId,
    comment: r.comment,
    parentId: r.parentId,
    kind: r.kind,
    deadline: r.deadline,
    createdAt: r.createdAt,
  };
}

class PgConfigTx implements ConfigTx {
  constructor(private readonly t: DbTx) {}

  latestRevision() {
    return readRevision(this.t);
  }
  revision(id: number) {
    return readRevision(this.t, id);
  }
  candidate() {
    return readCandidate(this.t, false);
  }
  lockCandidate() {
    return readCandidate(this.t, true);
  }
  pending() {
    return readPending(this.t);
  }

  async saveCandidate(c: Omit<CandidateState, 'owner' | 'updatedAt'>): Promise<void> {
    await this.t
      .update(configCandidate)
      .set({
        ownerId: c.ownerId,
        lockedAt: c.lockedAt,
        payload: c.payload,
        baseRevisionId: c.baseRevisionId,
        updatedAt: sql`now()`,
      })
      .where(eq(configCandidate.id, CANDIDATE_ID));
  }

  async insertRevision(r: NewRevision): Promise<Revision> {
    const [row] = await this.t
      .insert(configRevision)
      .values(r)
      .returning({ id: configRevision.id });
    const rev = await readRevision(this.t, row!.id);
    return rev!;
  }

  async setPending(p: Omit<PendingCommit, 'createdAt'> | null): Promise<void> {
    await this.t.delete(configPending).where(eq(configPending.id, PENDING_ID));
    if (p !== null) await this.t.insert(configPending).values({ id: PENDING_ID, ...p });
  }

  async syncUsers(users: readonly UserConfig[]): Promise<void> {
    const names = users.map((u) => u.username);
    // users that came from the configuration and are gone from it are deleted; the bootstrap admin stays (D-048)
    await this.t
      .delete(appUser)
      .where(
        names.length > 0
          ? and(eq(appUser.source, 'config'), notInArray(appUser.username, names))
          : eq(appUser.source, 'config'),
      );
    for (const u of users) {
      const set: { role: Role; disabled: boolean; passwordHash?: string } = {
        role: u.role,
        disabled: u.disabled,
      };
      if (u.passwordHash !== undefined) set.passwordHash = u.passwordHash;
      await this.t
        .insert(appUser)
        .values({ username: u.username, source: 'config', ...set })
        .onConflictDoUpdate({ target: appUser.username, set });
    }
  }
}

@Injectable()
export class PgConfigRepo implements ConfigRepo {
  constructor(@Inject(DB) private readonly db: Db) {}

  tx<T>(fn: (tx: ConfigTx) => Promise<T>): Promise<T> {
    return this.db.transaction((t) => fn(new PgConfigTx(t)));
  }
  latestRevision() {
    return readRevision(this.db);
  }
  revision(id: number) {
    return readRevision(this.db, id);
  }
  candidate() {
    return readCandidate(this.db, false);
  }
  pending() {
    return readPending(this.db);
  }

  async listRevisions(
    limit: number,
    offset: number,
  ): Promise<{ items: RevisionMeta[]; total: number }> {
    const items = await this.db
      .select(revisionMetaColumns)
      .from(configRevision)
      .leftJoin(appUser, eq(appUser.id, configRevision.authorId))
      .orderBy(desc(configRevision.id))
      .limit(limit)
      .offset(offset);
    const [c] = await this.db.select({ n: count() }).from(configRevision);
    return { items, total: c?.n ?? 0 };
  }

  async userHashes(): Promise<Map<string, string>> {
    const rows = await this.db
      .select({ username: appUser.username, hash: appUser.passwordHash })
      .from(appUser);
    return new Map(rows.filter((r) => r.hash !== null).map((r) => [r.username, r.hash as string]));
  }

  async existingSecretRefs(refs: readonly string[]): Promise<Set<string>> {
    if (refs.length === 0) return new Set();
    const rows = await this.db
      .select({ ref: secret.ref })
      .from(secret)
      .where(inArray(secret.ref, [...refs]));
    return new Set(rows.map((r) => r.ref));
  }
}
