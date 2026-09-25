import { Inject, Injectable } from '@nestjs/common';
import type { UserConfig } from '@ngfw/schema';
import { and, count, desc, eq, inArray, notInArray, sql } from 'drizzle-orm';
import { DB, type Db, type DbTx } from '../db/db.js';
import {
  apiKey,
  appUser,
  configCandidate,
  configPending,
  configRevision,
  configSync,
  secret,
  secretVersion,
  type Role,
} from '../db/schema.js';
import type {
  CandidateState,
  ConfigRepo,
  ConfigTx,
  Doc,
  NewRevision,
  PasswordReset,
  PendingCommit,
  Revision,
  RevisionMeta,
  SyncStatus,
} from './repo.js';

const CANDIDATE_ID = 1;

/**
 * TD-2 review L4: when API keys are deleted, a candidate lock one of them holds is released at once. The candidate
 * is discarded with it (as a lock break does) — it must never become the user's interactive lock, and nobody else may
 * inherit staged edits of a revoked credential. Returns whether a lock was released.
 */
export async function releaseKeyLocks(db: Db | DbTx, keyIds: readonly string[]): Promise<boolean> {
  if (keyIds.length === 0) return false;
  const rows = await db
    .update(configCandidate)
    .set({ ownerId: null, ownerKeyId: null, lockedAt: null, payload: null, baseRevisionId: null })
    .where(inArray(configCandidate.ownerKeyId, [...keyIds]))
    .returning({ id: configCandidate.id });
  return rows.length > 0;
}
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
  secretChanges: configRevision.secretChanges,
};

async function readRevision(db: Exec, id?: number): Promise<Revision | null> {
  const q = db
    .select({
      ...revisionMetaColumns,
      payload: configRevision.payload,
      secretVersions: configRevision.secretVersions,
    })
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
      ownerKeyId: configCandidate.ownerKeyId,
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
  let ownerKey: string | null = null;
  if (row.ownerKeyId !== null) {
    const k = await db
      .select({ name: apiKey.name })
      .from(apiKey)
      .where(eq(apiKey.id, row.ownerKeyId));
    ownerKey = k[0]?.name ?? null;
  }
  return { ...row, payload: (row.payload as Doc | null) ?? null, owner, ownerKey };
}

async function readSync(db: Exec): Promise<SyncStatus> {
  const [r] = await db.select().from(configSync).where(eq(configSync.id, 1));
  return r === undefined
    ? { state: 'in-sync', reason: '', txnId: null, since: new Date(0) }
    : { state: r.state as SyncStatus['state'], reason: r.reason, txnId: r.txnId, since: r.since };
}

async function writeSync(db: Exec, s: Omit<SyncStatus, 'since'>): Promise<void> {
  const v = { state: s.state, reason: s.reason, txnId: s.txnId, since: sql`now()` };
  await db
    .insert(configSync)
    .values({ id: 1, ...v })
    .onConflictDoUpdate({ target: configSync.id, set: v });
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
    restoreSecrets: r.restoreSecrets ?? null,
    warnings: r.warnings ?? null,
  };
}

/** Stored password hashes by username. Shared by the repo and the transaction (ARCH-11). */
async function readUserHashes(q: Db | DbTx): Promise<Map<string, string>> {
  const rows = await q
    .select({ username: appUser.username, hash: appUser.passwordHash })
    .from(appUser);
  return new Map(rows.filter((r) => r.hash !== null).map((r) => [r.username, r.hash as string]));
}

/** Current version of each existing secret ref among `refs`. */
async function readSecretVersions(
  q: Db | DbTx,
  refs: readonly string[],
): Promise<Record<string, number>> {
  if (refs.length === 0) return {};
  const rows = await q
    .select({ ref: secret.ref, version: secret.version })
    .from(secret)
    .where(inArray(secret.ref, [...refs]));
  return Object.fromEntries(rows.map((r) => [r.ref, r.version]));
}

class PgConfigTx implements ConfigTx {
  constructor(private readonly t: DbTx) {}

  /** ARCH-11 (TD-15): read on this transaction's connection — never a second pool client while holding one. */
  userHashes() {
    return readUserHashes(this.t);
  }
  secretVersions(refs: readonly string[]) {
    return readSecretVersions(this.t, refs);
  }

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

  async saveCandidate(c: Omit<CandidateState, 'owner' | 'ownerKey' | 'updatedAt'>): Promise<void> {
    await this.t
      .update(configCandidate)
      .set({
        ownerId: c.ownerId,
        ownerKeyId: c.ownerKeyId,
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

  setSync(s: Omit<SyncStatus, 'since'>) {
    return writeSync(this.t, s);
  }

  async restoreSecretVersions(versions: Record<string, number>): Promise<string[]> {
    const restored: string[] = [];
    for (const [ref, version] of Object.entries(versions)) {
      const [v] = await this.t
        .select({ ciphertext: secretVersion.ciphertext })
        .from(secretVersion)
        .where(and(eq(secretVersion.ref, ref), eq(secretVersion.version, version)));
      if (v === undefined) continue;
      const updated = await this.t
        .update(secret)
        .set({ ciphertext: v.ciphertext, version })
        .where(and(eq(secret.ref, ref), sql`${secret.version} <> ${version}`))
        .returning({ id: secret.id });
      if (updated.length > 0) restored.push(`${ref}@${version}`);
    }
    return restored;
  }

  async syncUsers(users: readonly UserConfig[]): Promise<PasswordReset[]> {
    const names = users.map((u) => u.username);
    // users that came from the configuration and are gone from it are deleted; the bootstrap admin stays (D-048)
    await this.t
      .delete(appUser)
      .where(
        names.length > 0
          ? and(eq(appUser.source, 'config'), notInArray(appUser.username, names))
          : eq(appUser.source, 'config'),
      );
    // D-102: the hashes before this promote (the commit mutex serialises every hash writer of this process);
    // D-100 (3): and the disabled flags
    const before = new Map(
      names.length === 0
        ? []
        : (
            await this.t
              .select({
                username: appUser.username,
                hash: appUser.passwordHash,
                disabled: appUser.disabled,
              })
              .from(appUser)
              .where(inArray(appUser.username, names))
          ).map((r) => [r.username, r]),
    );
    const resets: PasswordReset[] = [];
    for (const u of users) {
      const set: { role: Role; disabled: boolean; passwordHash?: string } = {
        role: u.role,
        disabled: u.disabled,
      };
      if (u.passwordHash !== undefined) set.passwordHash = u.passwordHash;
      const [row] = await this.t
        .insert(appUser)
        .values({ username: u.username, source: 'config', ...set })
        .onConflictDoUpdate({ target: appUser.username, set })
        .returning({ id: appUser.id });
      const prev = before.get(u.username);
      if (row === undefined || prev === undefined) continue;
      const password = u.passwordHash !== undefined && prev.hash !== u.passwordHash;
      // D-100 (3): an existing user this promote disables (not a re-enable, a user created disabled, or the same
      // flag staged again) — the generation moves, so access tokens, refresh chains and WebSockets end now
      const disabled = !prev.disabled && u.disabled === true;
      if (!password && !disabled) continue;
      // D-102: an existing user's hash changed through the config API → admin-reset semantics, same transaction:
      // generation bumped (refresh and key creation check it), lockout cleared, every API key deleted (no
      // keepApiKeys on this path) with any candidate lock it held — keys before the candidate (verify V4 lock order).
      // A disable alone bumps the generation only; both at once bump it ONCE.
      const [g] = await this.t
        .update(appUser)
        .set(
          password
            ? {
                credentialGen: sql`${appUser.credentialGen} + 1`,
                failedLogins: 0,
                lockedUntil: null,
              }
            : { credentialGen: sql`${appUser.credentialGen} + 1` },
        )
        .where(eq(appUser.id, row.id))
        .returning({ gen: appUser.credentialGen });
      let keys: { id: string; name: string }[] = [];
      let discardedCandidate = false;
      if (password) {
        keys = await this.t
          .delete(apiKey)
          .where(eq(apiKey.userId, row.id))
          .returning({ id: apiKey.id, name: apiKey.name });
        discardedCandidate = await releaseKeyLocks(
          this.t,
          keys.map((k) => k.id),
        );
      }
      resets.push({
        userId: row.id,
        username: u.username,
        gen: g!.gen,
        apiKeysRevoked: keys,
        discardedCandidate,
        reasons: [
          ...(password ? ['password' as const] : []),
          ...(disabled ? ['disabled' as const] : []),
        ],
      });
    }
    return resets;
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

  userHashes() {
    return readUserHashes(this.db);
  }

  secretVersions(refs: readonly string[]) {
    return readSecretVersions(this.db, refs);
  }

  getSync() {
    return readSync(this.db);
  }

  setSync(s: Omit<SyncStatus, 'since'>) {
    return writeSync(this.db, s);
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
