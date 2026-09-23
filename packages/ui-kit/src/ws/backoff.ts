export interface BackoffOptions {
  baseMs: number;
  maxMs: number;
  factor: number;
  /** 0..1 — proportion of random jitter added to each delay. */
  jitter: number;
}

export const DEFAULT_BACKOFF: BackoffOptions = { baseMs: 500, maxMs: 30_000, factor: 2, jitter: 0.2 };

/** Delay for reconnect attempt `attempt` (0-based), capped and jittered. */
export function backoffDelay(attempt: number, o: BackoffOptions = DEFAULT_BACKOFF, rnd = Math.random): number {
  const raw = Math.min(o.maxMs, o.baseMs * o.factor ** attempt);
  const jitter = raw * o.jitter * rnd();
  return Math.round(raw + jitter);
}
