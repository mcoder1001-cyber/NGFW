/**
 * Every request has a deadline (review P07b M3). A commit that cuts the operator's own route usually makes packets
 * vanish silently (no RST), and `fetch` would then wait for minutes. With a deadline the UI notices within seconds and
 * shows "Reconnecting…", and the cross-tab refresh lock cannot hang.
 */
export class RequestTimeoutError extends Error {
  constructor(readonly ms: number) {
    super(`no answer within ${ms} ms`);
    this.name = 'TimeoutError';
  }
}

type Fetch = (input: Request) => Promise<Response>;

/**
 * Deadlines by kind of request (ms). Commit/rollback/confirm/validate wait for the agent: `apply` is ABOVE the server's
 * worst-case commit (111 s: lock wait 1 + health 5 + DryRun 30 + Apply 60 + database 15, apps/api/src/commit/budget.ts —
 * TD-10a, review 2.4a), so the UI never reports a failure for a commit the server is still finishing. If even that
 * passes, the caller looks the outcome up (`applyOutcome`).
 */
export const TIMEOUTS = { poll: 6_000, write: 15_000, auth: 10_000, apply: 130_000 } as const;

const APPLY_PATHS = /\/api\/v1\/config\/(commit|rollback\/\d+|validate|commit\/confirm)$/;

export function timeoutFor(req: Request): number {
  const path = new URL(req.url, 'http://localhost').pathname;
  if (path.startsWith('/api/v1/auth/')) return TIMEOUTS.auth;
  if (APPLY_PATHS.test(path)) return TIMEOUTS.apply;
  return req.method === 'GET' || req.method === 'HEAD' ? TIMEOUTS.poll : TIMEOUTS.write;
}

/**
 * `fetchImpl(req)` with a deadline. The request is aborted when the deadline passes or the caller's signal aborts
 * (when the runtime allows re-signalling the Request); the returned promise rejects with `RequestTimeoutError` either way.
 */
export async function fetchWithTimeout(fetchImpl: Fetch, req: Request, ms: number = timeoutFor(req)): Promise<Response> {
  const ctrl = new AbortController();
  let r = req;
  try {
    r = new Request(req, { signal: ctrl.signal });
  } catch {
    // some test runtimes reject a foreign AbortSignal; the race below still enforces the deadline
  }
  const onAbort = () => ctrl.abort();
  req.signal?.addEventListener('abort', onAbort);
  let timer: ReturnType<typeof setTimeout> | undefined;
  const deadline = new Promise<never>((_, reject) => {
    timer = setTimeout(() => {
      ctrl.abort();
      reject(new RequestTimeoutError(ms));
    }, ms);
  });
  try {
    return await Promise.race([fetchImpl(r), deadline]);
  } finally {
    clearTimeout(timer);
    req.signal?.removeEventListener('abort', onAbort);
  }
}

/** The server's worst-case commit, lock wait included (apps/api/src/commit/budget.ts COMMIT_BUDGET_MAX_MS). */
export const COMMIT_BUDGET_MS = 111_000;

/**
 * Review M1: a lost answer is followed up until the server's budget (+ margin) has surely passed — an early network
 * error or a proxy 502/504 comes back while the server may still be applying — polling every `everyMs`. Only after
 * `untilMs` may "nothing new" mean "not applied". Mutable for tests only.
 */
export const OUTCOME_WAIT = { untilMs: COMMIT_BUDGET_MS + 5_000, everyMs: 3_000 };

/** What the API reports after a commit-like request got no answer (GET /state/system + newest revision). */
export interface ApplyFacts {
  /** Client time the request was sent (ms). */
  sentAt: number;
  /** Client time of this lookup (ms); before `waitUntil` "nothing new" means "still running". */
  now: number;
  waitUntil: number;
  /** The running revision before the request (null: unknown). */
  beforeRevision: number | null;
  pending: { txnId?: unknown; deadline?: unknown; createdAt?: unknown } | null;
  sync: { state: string; reason: string } | null;
  newest: { id: number; createdAt: string } | null;
}

export type ApplyOutcome =
  | { kind: 'pending'; txnId: string; deadlineMs: number }
  | { kind: 'applied'; revision: number }
  | { kind: 'unknown'; reason: string }
  | { kind: 'running' }
  | { kind: 'not-applied' };

/** Allowed client/server clock difference when a server time is compared with the request's. */
const SKEW_MS = 5_000;

/**
 * TD-10a (review 2.4a, M1, L3): the outcome of a commit, rollback or confirm whose answer was lost, from the API's own
 * state — a pending commit created since the request (applied, waiting for confirmation; an older one is someone
 * else's), a revision newer than the one running before (applied), running not known to match the data plane
 * (unknown: a reconcile runs); otherwise "running" while the server may still be working, and only after that "not
 * applied".
 */
export function applyOutcome(f: ApplyFacts): ApplyOutcome {
  const p = f.pending;
  if (p && typeof p.txnId === 'string' && typeof p.deadline === 'string') {
    const created = typeof p.createdAt === 'string' ? Date.parse(p.createdAt) : Number.NaN;
    if (Number.isNaN(created) || created >= f.sentAt - SKEW_MS) return { kind: 'pending', txnId: p.txnId, deadlineMs: Date.parse(p.deadline) };
  }
  if (f.newest) {
    const newer = f.beforeRevision !== null ? f.newest.id > f.beforeRevision : Date.parse(f.newest.createdAt) >= f.sentAt - SKEW_MS;
    if (newer) return { kind: 'applied', revision: f.newest.id };
  }
  if (f.sync && f.sync.state !== 'in-sync') return { kind: 'unknown', reason: f.sync.reason };
  return f.now < f.waitUntil ? { kind: 'running' } : { kind: 'not-applied' };
}
