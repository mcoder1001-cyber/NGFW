import type { UserConfig } from '@ngfw/schema';
import type { ProblemIssue } from '../common/problem.js';
import type { SecretChange } from './documents.js';

/**
 * Persistence port of the datastore and the commit engine. `PgConfigRepo` is the product implementation;
 * `MemoryConfigRepo` backs the unit tests (same semantics, one process). Documents are plain JSON objects.
 */
export type Doc = Record<string, unknown>;

export interface RevisionMeta {
  id: number;
  createdAt: Date;
  authorId: number | null;
  author: string | null;
  comment: string;
  parentId: number | null;
  hash: string;
  txnId: string | null;
  kind: string;
  /** Secret leaves changed against the parent, without values (TD-2 #6). */
  secretChanges: SecretChange[];
}

export interface Revision extends RevisionMeta {
  /** Redacted (no secret leaves, D-046). */
  payload: Doc;
  /** Secret versions pinned at commit time (review M2). */
  secretVersions?: Record<string, number> | null;
}

export interface NewRevision {
  /** Secret leaves changed against the parent, without values (TD-2 #6). */
  secretChanges?: SecretChange[];
  /** Secret versions pinned by this revision (review M2). */
  secretVersions?: Record<string, number> | null;
  authorId: number | null;
  comment: string;
  parentId: number | null;
  payload: Doc;
  hash: string;
  txnId: string | null;
  kind: string;
}

export interface CandidateState {
  ownerId: number | null;
  owner: string | null;
  /**
   * The API key that holds the lock (TD-2 #5, D-093): an API-key session owns the candidate on its own; null = an
   * interactive (JWT) session of `ownerId` — all of that user's interactive sessions share it, as before.
   */
  ownerKeyId: string | null;
  /** Name of that key (null when the key was deleted meanwhile). */
  ownerKey: string | null;
  lockedAt: Date | null;
  /** null = equals running. May carry secret leaves (write path only, never returned). */
  payload: Doc | null;
  baseRevisionId: number | null;
  updatedAt: Date;
}

export interface PendingCommit {
  txnId: string;
  /** Unredacted (hashes of users added by this commit); internal only. */
  payload: Doc;
  hash: string;
  authorId: number | null;
  comment: string;
  parentId: number | null;
  kind: string;
  deadline: Date;
  createdAt: Date;
  /**
   * A rollback's secret versions (`{ "<kind>/<name>": version }`) that become active when the commit is confirmed —
   * by the API's confirm, the deadline watcher or a reconcile after an API restart (TD-10a, review 2.2). null = none.
   */
  restoreSecrets?: Record<string, number> | null;
  /** The agent's warnings of this commit; confirm returns them again (TD-10a, review 2.5). */
  warnings?: ProblemIssue[] | null;
}

/**
 * D-102 (TD-2 verify V2): an existing user whose password hash a promoted snapshot changed. `syncUsers` applies
 * admin-reset semantics in the promote transaction (credential generation bumped, failed logins/lock cleared, API
 * keys deleted with any candidate lock they held); the commit engine ends the sessions once it committed and audits.
 */
export interface PasswordReset {
  userId: number;
  username: string;
  /** The credential generation after the bump. */
  gen: number;
  apiKeysRevoked: { id: string; name: string }[];
  /** A candidate locked by one of those keys was discarded with its lock. */
  discardedCandidate: boolean;
}

/** Does running (PostgreSQL) match the data plane? (review M3) */
export type SyncState = 'in-sync' | 'unknown' | 'degraded';
export interface SyncStatus {
  state: SyncState;
  reason: string;
  txnId: string | null;
  since: Date;
}

export interface ConfigReads {
  latestRevision(): Promise<Revision | null>;
  revision(id: number): Promise<Revision | null>;
  candidate(): Promise<CandidateState>;
  pending(): Promise<PendingCommit | null>;
}

/** Operations inside one transaction. `lockCandidate()` serialises writers (SELECT … FOR UPDATE). */
export interface ConfigTx extends ConfigReads {
  lockCandidate(): Promise<CandidateState>;
  saveCandidate(c: Omit<CandidateState, 'owner' | 'ownerKey' | 'updatedAt'>): Promise<void>;
  insertRevision(r: NewRevision): Promise<Revision>;
  setPending(p: Omit<PendingCommit, 'createdAt'> | null): Promise<void>;
  /**
   * Make app_user follow `management.users` (hashes: only where the document carries one). An EXISTING user whose
   * hash changes gets admin-reset semantics in this transaction (D-102); those users are returned.
   */
  syncUsers(users: readonly UserConfig[]): Promise<PasswordReset[]>;
  /** Re-activate the given secret versions (rollback, review M2); unknown refs/versions are skipped. */
  restoreSecretVersions(versions: Record<string, number>): Promise<string[]>;
  setSync(s: Omit<SyncStatus, 'since'>): Promise<void>;
}

export interface ConfigRepo extends ConfigReads {
  tx<T>(fn: (tx: ConfigTx) => Promise<T>): Promise<T>;
  listRevisions(limit: number, offset: number): Promise<{ items: RevisionMeta[]; total: number }>;
  /** Stored password hashes by username (hydration of redacted documents before validation). */
  userHashes(): Promise<Map<string, string>>;
  /** Refs (`<kind>/<name>`) present in the secret store among `refs`. */
  existingSecretRefs(refs: readonly string[]): Promise<Set<string>>;
  /** Current version of each existing ref among `refs`. */
  secretVersions(refs: readonly string[]): Promise<Record<string, number>>;
  getSync(): Promise<SyncStatus>;
  setSync(s: Omit<SyncStatus, 'since'>): Promise<void>;
}
