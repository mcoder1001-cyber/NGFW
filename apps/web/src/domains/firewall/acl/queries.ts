import type { paths } from '@ngfw/api-client';
import { useMutation, useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query';
import { api, authMiddleware } from '../../../api';
import { ApiError, call } from '../../../api-problem';
import { session } from '../../../auth/session';
import { invalidateConfig, qk } from '../../../config/queries';
import { fetchWithTimeout } from '../../../net';
import {
  ACL_POLL_MS,
  type BulkBody,
  type BulkResult,
  type ImportMode,
  type ImportResult,
  type RuleSource,
  type RulesPage,
  type RulesQuery,
} from './model';

type Ok<O> = O extends { responses: { 200: { content: { 'application/json': infer T } } } }
  ? T
  : never;
export type EditResult = Ok<NonNullable<paths['/api/v1/config/{path}']['put']>>;

/** Query keys mirror API paths (docs/05). Everything under `['state', 'acl']` is refetched after an edit. */
export const aclKeys = {
  all: ['state', 'acl'] as const,
  lists: ['state', 'acl', 'lists'] as const,
  rules: (list: string) => ['state', 'acl', 'rules', list] as const,
  attachments: ['state', 'acl', 'attachments'] as const,
  running: (path: string) => ['config', 'running', path] as const,
};

/**
 * Lists with pending marks and live VPP status; polled every 30 s at most (D-132) — the page has a Refresh button.
 * `poll: false` for screens that only need the names (the attachments tab has no timer at all).
 */
export function useAclLists({ poll = true }: { poll?: boolean } = {}) {
  return useQuery({
    queryKey: aclKeys.lists,
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/acl/lists', { signal }))).data,
    refetchInterval: poll ? ACL_POLL_MS : false,
  });
}

/** Per-interface bindings as VPP holds them: no timer at all (Refresh button only). */
export function useAclAttachments() {
  return useQuery({
    queryKey: aclKeys.attachments,
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/acl/attachments', { signal }))).data,
  });
}

/** One page of a list's rules, searched and paged by the server (never the whole list). */
export async function fetchRules(
  list: string,
  query: RulesQuery,
  signal?: AbortSignal,
): Promise<RulesPage> {
  return (
    await call(
      api.GET('/api/v1/state/acl/lists/{name}/rules', {
        params: { path: { name: list }, query },
        ...(signal ? { signal } : {}),
      }),
    )
  ).data;
}

// ---------------------------------------------------------------------------------------------------------------
// Generic pointer routes (P06) for nodes below the domain root
// ---------------------------------------------------------------------------------------------------------------

/**
 * The pointer routes take a multi-segment pointer (`acl/lists/web/rules/3`). openapi-fetch percent-encodes a path
 * parameter as ONE segment (`acl%2Flists%2F…`, which the API reads as a single member name), so these URLs are built
 * from the generated path templates here, one encoded segment per pointer segment, and sent through the same auth
 * middleware as the typed client (api.ts).
 */
const CONFIG_AT = '/api/v1/config/{path}' satisfies keyof paths;
const CANDIDATE_AT = '/api/v1/config/candidate/{path}' satisfies keyof paths;
const IMPORT = '/api/v1/actions/acl/import' satisfies keyof paths;
const EXPORT = '/api/v1/actions/acl/export.csv' satisfies keyof paths;

export type PointerPath = readonly (string | number)[];

export function pointerUrl(
  template: typeof CONFIG_AT | typeof CANDIDATE_AT,
  path: PointerPath,
): string {
  return template.replace('{path}', path.map((s) => encodeURIComponent(String(s))).join('/'));
}

const auth = authMiddleware(session);
const absolute = (url: string) => `${globalThis.location?.origin ?? ''}${url}`;

async function asResult(
  response: Response,
): Promise<{ data?: unknown; error?: unknown; response: Response }> {
  const text = await response.text();
  let body: unknown;
  try {
    body = text ? JSON.parse(text) : undefined;
  } catch {
    body = undefined;
  }
  return response.ok ? { data: body, response } : { error: body, response };
}

async function send(url: string, init: RequestInit): Promise<unknown> {
  const r = await call(
    auth.onRequest({ request: new Request(absolute(url), init) }).then(asResult),
  );
  return r.data;
}

/** Node of the candidate at `path`; `fallback` when there is nothing there yet (404). */
export async function candidateAt<T>(
  path: PointerPath,
  fallback: T,
  signal?: AbortSignal,
): Promise<T> {
  try {
    return ((await send(pointerUrl(CANDIDATE_AT, path), signal ? { signal } : {})) ??
      fallback) as T;
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) return fallback;
    throw e;
  }
}

/** Node of the running configuration at `path`; `fallback` when absent. */
export async function runningAt<T>(
  path: PointerPath,
  fallback: T,
  signal?: AbortSignal,
): Promise<T> {
  try {
    return ((await send(pointerUrl(CONFIG_AT, path), signal ? { signal } : {})) ?? fallback) as T;
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) return fallback;
    throw e;
  }
}

/** A node of the candidate (`acl/attachments`, `acl/macip`, …): never the whole `acl`, which holds up to 100 000 rules per list. */
export function useCandidateAt<T>(path: PointerPath, fallback: T) {
  return useQuery({
    queryKey: qk.candidate(path.join('/')),
    queryFn: ({ signal }) => candidateAt(path, fallback, signal),
  });
}

export function useRunningAt<T>(path: PointerPath, fallback: T) {
  return useQuery({
    queryKey: aclKeys.running(path.join('/')),
    queryFn: ({ signal }) => runningAt(path, fallback, signal),
  });
}

export interface ConfigEdit {
  method: 'PUT' | 'PATCH' | 'DELETE';
  path: PointerPath;
  body?: unknown;
}

/** PUT (replace; `…/rules/<size>` appends), PATCH (RFC 7386 merge patch) or DELETE on a candidate pointer. */
export async function editConfig({ method, path, body }: ConfigEdit): Promise<EditResult> {
  const init: RequestInit = { method };
  if (method !== 'DELETE') {
    init.body = JSON.stringify(body);
    init.headers = {
      'content-type': method === 'PATCH' ? 'application/merge-patch+json' : 'application/json',
    };
  }
  return (await send(pointerUrl(CONFIG_AT, path), init)) as EditResult;
}

export function invalidateAcl(qc: QueryClient) {
  return Promise.all([invalidateConfig(qc), qc.invalidateQueries({ queryKey: aclKeys.all })]);
}

/** A candidate edit; the lists, rules, attachments and the pending-change bar refresh afterwards. */
export function useAclEdit() {
  const qc = useQueryClient();
  return useMutation({ mutationFn: editConfig, onSettled: () => invalidateAcl(qc) });
}

/** Bulk edit of a list's rules in the candidate (enable/disable/delete/move/renumber) as one edit. */
export function useBulk(list: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: BulkBody): Promise<BulkResult> =>
      (
        await call(
          api.POST('/api/v1/actions/acl/lists/{name}/rules/bulk', {
            params: { path: { name: list } },
            body,
          }),
        )
      ).data,
    onSettled: () => invalidateAcl(qc),
  });
}

// ---------------------------------------------------------------------------------------------------------------
// CSV import / export
// ---------------------------------------------------------------------------------------------------------------

/** CSV import/export may move 100 000 rules: a longer deadline than the SPA's poll/write defaults (net.ts). */
export const CSV_TIMEOUT_MS = 300_000;

/** The token handling of api.ts's middleware (bearer + one refresh on 401) with the CSV deadline. */
async function longRequest(url: string, init: RequestInit, ms = CSV_TIMEOUT_MS): Promise<Response> {
  const make = (token: string | null) => {
    const headers = new Headers(init.headers);
    if (token !== null) headers.set('authorization', `Bearer ${token}`);
    return new Request(absolute(url), { ...init, headers });
  };
  const doFetch = (r: Request) => fetchWithTimeout((x) => globalThis.fetch(x), r, ms);
  const token = session.accessToken;
  const res = await doFetch(make(token));
  if (res.status !== 401 || token === null) return res;
  if (!(await session.handleUnauthorized(token)) || session.accessToken === null) return res;
  return doFetch(make(session.accessToken));
}

/** POST the CSV to the import action: a dry run (validate + preview) unless `dryRun` is false. */
export async function importCsv(
  list: string,
  csv: string,
  mode: ImportMode,
  dryRun: boolean,
): Promise<ImportResult> {
  const q = new URLSearchParams({ list, mode: mode ?? 'replace', dryRun: String(dryRun) });
  const r = await call(
    longRequest(`${IMPORT}?${q.toString()}`, {
      method: 'POST',
      body: csv,
      headers: { 'content-type': 'text/csv' },
    }).then(asResult),
  );
  return r.data as ImportResult;
}

/** Download a list as CSV (authenticated fetch → blob → object URL). */
export async function exportCsv(list: string, source: RuleSource): Promise<void> {
  const q = new URLSearchParams({ list, source });
  const res = await call(
    longRequest(`${EXPORT}?${q.toString()}`, { method: 'GET' }).then(async (response) =>
      response.ok ? { data: await response.blob(), response } : asResult(response),
    ),
  );
  const blob = res.data as Blob;
  const name =
    /filename="([^"]+)"/.exec(res.response.headers.get('content-disposition') ?? '')?.[1] ??
    `acl-${list}.csv`;
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = name.replace(/\.csv$/, `-${source}.csv`);
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 0);
}
