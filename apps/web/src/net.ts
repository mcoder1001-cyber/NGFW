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

/** Deadlines by kind of request (ms). Commit/rollback/validate wait for the agent (P06 default 60 s). */
export const TIMEOUTS = { poll: 6_000, write: 15_000, auth: 10_000, apply: 75_000 } as const;

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
