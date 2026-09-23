import type { UserConfig } from '@ngfw/schema';

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
}

export interface Revision extends RevisionMeta {
  /** Redacted (no secret leaves, D-046). */
  payload: Doc;
}

export interface NewRevision {
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
  saveCandidate(c: Omit<CandidateState, 'owner' | 'updatedAt'>): Promise<void>;
  insertRevision(r: NewRevision): Promise<Revision>;
  setPending(p: Omit<PendingCommit, 'createdAt'> | null): Promise<void>;
  /** Make app_user follow `management.users` (hashes: only where the document carries one). */
  syncUsers(users: readonly UserConfig[]): Promise<void>;
}

export interface ConfigRepo extends ConfigReads {
  tx<T>(fn: (tx: ConfigTx) => Promise<T>): Promise<T>;
  listRevisions(limit: number, offset: number): Promise<{ items: RevisionMeta[]; total: number }>;
  /** Stored password hashes by username (hydration of redacted documents before validation). */
  userHashes(): Promise<Map<string, string>>;
  /** Refs (`<kind>/<name>`) present in the secret store among `refs`. */
  existingSecretRefs(refs: readonly string[]): Promise<Set<string>>;
}
