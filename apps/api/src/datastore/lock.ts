import { problems } from '../common/problem.js';
import type { Principal } from '../common/principal.js';
import type { CandidateState } from './repo.js';

/** The single-writer lock on the candidate as clients see it. */
export interface LockInfo {
  locked: boolean;
  owner: string | null;
  ownerId: number | null;
  /** API key holding the lock (TD-2 #5); null = the interactive sessions of `owner`. */
  ownerKeyId: string | null;
  ownerKey: string | null;
  lockedAt: string | null;
  /** Last edit; the lock may be taken over when this is older than the lock TTL. */
  lastActivity: string | null;
  expiresAt: string | null;
}

export function lockInfo(c: CandidateState, ttlSec: number): LockInfo {
  const locked = c.ownerId !== null;
  return {
    locked,
    owner: locked ? c.owner : null,
    ownerId: c.ownerId,
    ownerKeyId: locked ? c.ownerKeyId : null,
    ownerKey: locked ? c.ownerKey : null,
    lockedAt: c.lockedAt?.toISOString() ?? null,
    lastActivity: locked ? c.updatedAt.toISOString() : null,
    expiresAt: locked ? new Date(c.updatedAt.getTime() + ttlSec * 1000).toISOString() : null,
  };
}

/**
 * The lock-owner identity of a caller (D-093, TD-2 #5): an API-key session is its own owner (two pipelines with keys
 * of one user never share a candidate); every interactive (JWT) session of a user is the same owner, as before.
 */
export function lockOwnerOf(user: Principal): { ownerId: number; ownerKeyId: string | null } {
  return { ownerId: user.id, ownerKeyId: user.via === 'apikey' ? (user.keyId ?? null) : null };
}

export function isLockOwner(c: Pick<CandidateState, 'ownerId' | 'ownerKeyId'>, user: Principal) {
  const me = lockOwnerOf(user);
  return c.ownerId === me.ownerId && (c.ownerKeyId ?? null) === me.ownerKeyId;
}

/** `'admin'` or `'admin' (API key 'ci')` — for problem details. */
export function ownerLabel(c: CandidateState): string {
  const user = `'${c.owner ?? `user #${c.ownerId}`}'`;
  if (c.ownerKeyId === null) return user;
  return `${user} (API key ${c.ownerKey === null ? c.ownerKeyId : `'${c.ownerKey}'`})`;
}

export type LockDecision = 'free' | 'own' | 'stale';

/**
 * Who may write the candidate: the lock owner, anybody when it is free, anybody when the owner has been idle for
 * longer than the TTL (the takeover discards nothing — the candidate stays, the new owner continues from it).
 * Otherwise 409 problem+json with the owner (P06 §2). The owner is a user's interactive sessions or one API key.
 */
export function checkLock(
  c: CandidateState,
  user: Principal,
  now: Date,
  ttlSec: number,
): LockDecision {
  if (c.ownerId === null) return 'free';
  if (isLockOwner(c, user)) return 'own';
  if (now.getTime() - c.updatedAt.getTime() > ttlSec * 1000) return 'stale';
  throw problems.conflict(
    'candidate-locked',
    `the candidate configuration is locked by ${ownerLabel(c)} since ${c.lockedAt?.toISOString() ?? '?'}`,
    { lock: lockInfo(c, ttlSec) },
  );
}
