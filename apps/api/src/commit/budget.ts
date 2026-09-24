import type { Env } from '../config.js';

/**
 * TD-10a (review 2.4a): the server's time budget for one commit, rollback or confirm, made explicit so that clients
 * can wait LONGER than the server works. Before, the web gave up after 75 s and the CLI after 90 s while a commit
 * could take health 5 s + DryRun 60 s + Apply 60 s + an unbounded queue on the commit mutex; a client then reported
 * an error for a commit that went through.
 *
 * - lock: a section that is busy is not queued — after `lockWaitMs` the answer is 409 `commit-busy`.
 * - health: 5 s (AgentClient default), DryRun ≤ 30 s, Apply ≤ 60 s — both capped even if VRX_AGENT_TIMEOUT_MS is
 *   larger (the gRPC deadline the agent sees).
 * - db: margin for the PostgreSQL work of a commit (candidate, validation reads, the promote transaction).
 *
 * Worst case with any environment: `COMMIT_BUDGET_MAX_MS` (111 s). The web waits `TIMEOUTS.apply` = 130 s
 * (apps/web/src/net.ts) and the CLI 150 s (apps/cli/internal/api/client.go ApplyTimeout); after that they look up the
 * real outcome (pending commit, sync state, newest revision) instead of reporting a failure.
 */
export const LOCK_WAIT_MS = 1_000;
export const HEALTH_MS = 5_000;
export const DRY_RUN_MAX_MS = 30_000;
export const APPLY_MAX_MS = 60_000;
export const DB_MARGIN_MS = 15_000;

export const COMMIT_BUDGET_MAX_MS =
  LOCK_WAIT_MS + HEALTH_MS + DRY_RUN_MAX_MS + APPLY_MAX_MS + DB_MARGIN_MS;

export interface CommitBudget {
  lockWaitMs: number;
  healthMs: number;
  dryRunMs: number;
  applyMs: number;
  dbMs: number;
  totalMs: number;
}

export function commitBudget(env: Pick<Env, 'VRX_AGENT_TIMEOUT_MS'>): CommitBudget {
  const dryRunMs = Math.min(env.VRX_AGENT_TIMEOUT_MS, DRY_RUN_MAX_MS);
  const applyMs = Math.min(env.VRX_AGENT_TIMEOUT_MS, APPLY_MAX_MS);
  return {
    lockWaitMs: LOCK_WAIT_MS,
    healthMs: HEALTH_MS,
    dryRunMs,
    applyMs,
    dbMs: DB_MARGIN_MS,
    totalMs: LOCK_WAIT_MS + HEALTH_MS + dryRunMs + applyMs + DB_MARGIN_MS,
  };
}
