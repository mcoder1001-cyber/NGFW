import type { paths } from '@ngfw/api-client';
import { useQuery, type QueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig } from '../../../config/queries';
import { asResult, longRequest } from '../../firewall/acl/queries';

type Ok<P extends keyof paths, M extends 'get' | 'post'> = paths[P][M] extends {
  responses: { 200: { content: { 'application/json': infer T } } };
}
  ? T
  : never;

export type GbStatus = Ok<'/api/v1/security/global-blocking', 'get'>;
export type GbListStatus = GbStatus['lists'][number];
export type GbPreview = Ok<'/api/v1/security/global-blocking/lists/{name}/import', 'post'>;

export const gbKeys = { status: ['state', 'global-blocking'] as const };

/** D-132: no UI timer below 30 s on live state; the page has a Refresh button. */
export const STATE_POLL_MS = 30_000;

const BASE = '/api/v1/security/global-blocking';

export function useGbStatus() {
  return useQuery({
    queryKey: gbKeys.status,
    queryFn: async ({ signal }) => (await call(api.GET(BASE, { signal }))).data as GbStatus,
    refetchInterval: STATE_POLL_MS,
  });
}

export function invalidateGb(qc: QueryClient) {
  return Promise.all([invalidateConfig(qc), qc.invalidateQueries({ queryKey: gbKeys.status })]);
}

/** Upload a file: a preview unless `dryRun` is false (then the entries are staged into the candidate). */
export async function importList(name: string, text: string, dryRun: boolean): Promise<GbPreview> {
  const r = await call(
    longRequest(`${BASE}/lists/${encodeURIComponent(name)}/import?dryRun=${String(dryRun)}`, {
      method: 'POST',
      body: text,
      headers: { 'content-type': 'text/plain; charset=utf-8' },
    }).then(asResult),
  );
  return r.data as GbPreview;
}

/** Download the list from its server URL now (preview, or stage with `dryRun` false). */
export async function fetchListNow(name: string, dryRun: boolean): Promise<GbPreview> {
  const r = await call(
    longRequest(`${BASE}/lists/${encodeURIComponent(name)}/fetch?dryRun=${String(dryRun)}`, {
      method: 'POST',
    }).then(asResult),
  );
  return r.data as GbPreview;
}

/** Save a list as a text file (authenticated fetch → blob → object URL). */
export async function exportList(name: string, source: 'running' | 'candidate'): Promise<void> {
  const res = await call(
    longRequest(`${BASE}/lists/${encodeURIComponent(name)}/export?source=${source}`, {
      method: 'GET',
    }).then(async (response) =>
      response.ok ? { data: await response.blob(), response } : asResult(response),
    ),
  );
  const url = URL.createObjectURL(res.data as Blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `blocklist-${name}-${source}.txt`;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 0);
}
