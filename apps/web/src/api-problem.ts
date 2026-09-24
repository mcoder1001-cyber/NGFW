import type { ProblemDetails } from '@ngfw/ui-kit/schema-form';

/** One `errors[]` entry of a P06 problem (RFC 9457 + `pointer`, D-070). */
export interface ProblemIssue {
  pointer: string;
  message: string;
  rule?: string;
}

/** RFC 9457 body as P06 sends it (extension members: `lock`, `results`, `sync`, `pending`, `tier`, `warnings`, …). */
export interface ApiProblemBody {
  type?: string;
  title?: string;
  status?: number;
  detail?: string;
  errors?: ProblemIssue[];
  [extension: string]: unknown;
}

/**
 * A failed API call. `unreachable` = no answer from the API itself (network error, or a gateway/proxy 5xx that is not
 * `application/problem+json`) — the UI shows "reconnecting…" instead of an error for those.
 */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly body: ApiProblemBody,
    readonly unreachable: boolean,
  ) {
    super(body.detail ?? body.title ?? `HTTP ${status}`);
    this.name = 'ApiError';
  }

  /** Problem slug (`candidate-locked`, `commit-pending`, `validation`, …) from `type`. */
  get slug(): string | undefined {
    return this.body.type?.split('/').pop();
  }

  /** For `<SchemaForm problem>`: P06 names the text `message`, the form renderer `detail`. */
  toFormProblem(): ProblemDetails {
    const { errors, ...rest } = this.body;
    return { ...rest, ...(errors ? { errors: errors.map((e) => ({ pointer: e.pointer, detail: e.message })) } : {}) } as ProblemDetails;
  }
}

export function isUnreachable(e: unknown): boolean {
  return e instanceof ApiError && e.unreachable;
}

type FetchResult = { data?: unknown; error?: unknown; response: Response };
/** The success member's `data` of an openapi-fetch result union. */
type DataOf<R> = R extends { data: infer D } ? Exclude<D, undefined> : never;

/**
 * Unwrap an openapi-fetch call: resolves with `data` (+ the response for headers such as `Date`), rejects with an
 * `ApiError`. Every screen goes through this, so server problems surface unchanged (UI honesty rule).
 */
export async function call<R extends FetchResult>(p: Promise<R>): Promise<{ data: DataOf<R>; response: Response }> {
  let r: R;
  try {
    r = await p;
  } catch {
    throw new ApiError(0, { title: 'unreachable' }, true);
  }
  const { response } = r;
  if (!response.ok) {
    const isProblem = (response.headers.get('content-type') ?? '').includes('json');
    const body: ApiProblemBody = isProblem && r.error && typeof r.error === 'object' ? (r.error as ApiProblemBody) : { status: response.status };
    throw new ApiError(response.status, body, !isProblem && response.status >= 500);
  }
  return { data: r.data as DataOf<R>, response };
}

/** Server clock offset (server − client, ms) from a response's `Date` header; 0 when absent. */
export function serverOffsetMs(response: Response, clientNow: number = Date.now()): number {
  const d = response.headers.get('date');
  const t = d ? Date.parse(d) : NaN;
  return Number.isFinite(t) ? t - clientNow : 0;
}
