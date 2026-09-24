import { problems } from '../common/problem.js';
import type { Principal } from '../common/principal.js';
import type { CandidateState } from './repo.js';

/** The single-writer lock on the candidate as clients see it. */
export interface LockInfo {
  locked: boolean;
  owner: string | null;
  ownerId: number | null;
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
    lockedAt: c.lockedAt?.toISOString() ?? null,
    lastActivity: locked ? c.updatedAt.toISOString() : null,
    expiresAt: locked ? new Date(c.updatedAt.getTime() + ttlSec * 1000).toISOString() : null,
  };
}

export type LockDecision = 'free' | 'own' | 'stale';

/**
 * Who may write the candidate: the lock owner, anybody when it is free, anybody when the owner has been idle for
 * longer than the TTL (the takeover discards nothing — the candidate stays, the new owner continues from it).
 * Otherwise 409 problem+json with the owner (P06 §2).
 */
export function checkLock(
  c: CandidateState,
  user: Principal,
  now: Date,
  ttlSec: number,
): LockDecision {
  if (c.ownerId === null) return 'free';
  if (c.ownerId === user.id) return 'own';
  if (now.getTime() - c.updatedAt.getTime() > ttlSec * 1000) return 'stale';
  throw problems.conflict(
    'candidate-locked',
    `the candidate configuration is locked by '${c.owner ?? `user #${c.ownerId}`}' since ${c.lockedAt?.toISOString() ?? '?'}`,
    { lock: lockInfo(c, ttlSec) },
  );
}
