import { ApiResponse } from '@nestjs/swagger';
import { problems, type ProblemError } from '../common/problem.js';
import { ref } from '../common/zod.js';

/** OpenAPI: the 409 of the password routes. */
export const CommitBusyDoc = () =>
  ApiResponse({
    status: 409,
    description:
      '`commit-busy` (TD-10b): a commit, rollback, confirm or reconcile held the commit lock for more than 1 s — nothing was changed; retry (`retryAfterSec`)',
    content: { 'application/problem+json': { schema: ref('Problem') } },
  });

/**
 * TD-10b (manager addendum from the TD-10a review, M2): how long a password set waits for the commit lock. The same
 * rule as TD-10a's user sections (`commit/budget.ts` LOCK_WAIT_MS = 1 s): no queue behind a commit that may take the
 * whole server budget (up to 111 s) while the web gives the call 15 s.
 */
export const PASSWORD_LOCK_WAIT_MS = 1_000;

/** 409 `commit-busy` — the body TD-10a's commit/rollback/confirm answer when the lock is held in this process. */
export function commitBusy(): ProblemError {
  return problems.conflict(
    'commit-busy',
    'another commit, rollback, confirm or reconcile is in progress; retry in a moment',
    { retryAfterSec: 2 },
  );
}

type Exclusive = <R>(fn: () => Promise<R>) => Promise<R>;

/**
 * Run `fn` under the commit engine's lock (`exclusive`) only if the lock is had within `waitMs`; otherwise 409
 * `commit-busy`, and the queued section is abandoned: when its turn comes it does nothing (the caller already got the
 * 409, so nothing may change after it). A section that started before the deadline is awaited to its end.
 * LOCAL BOUND: task/TD-10a replaces the in-process Mutex with `CommitLock` (`tryRun(fn, LOCK_WAIT_MS)`, plus the
 * cross-process advisory lock); when both are on main the merger points this at it (TD-10b questions).
 */
export async function withinCommitLock<T>(
  exclusive: Exclusive,
  fn: () => Promise<T>,
  waitMs = PASSWORD_LOCK_WAIT_MS,
): Promise<T> {
  const TIMEOUT = Symbol('timeout');
  let state: 'waiting' | 'running' | 'abandoned' = 'waiting';
  const section = exclusive(async (): Promise<T | typeof TIMEOUT> => {
    if (state === 'abandoned') return TIMEOUT;
    state = 'running';
    return fn();
  });
  let timer: NodeJS.Timeout | undefined;
  const deadline = new Promise<typeof TIMEOUT>((resolve) => {
    timer = setTimeout(() => resolve(TIMEOUT), waitMs);
  });
  try {
    const first = await Promise.race([section, deadline]);
    if (first !== TIMEOUT) return first;
    if (state === 'waiting') {
      state = 'abandoned';
      throw commitBusy();
    }
    // it started just before the deadline: its outcome is the answer
    const r = await section;
    if (r === TIMEOUT) throw commitBusy();
    return r;
  } finally {
    clearTimeout(timer);
  }
}
