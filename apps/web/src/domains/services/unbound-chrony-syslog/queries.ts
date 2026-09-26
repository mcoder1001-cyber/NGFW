import type { paths } from '@ngfw/api-client';
import type { ManagementConfig, ServicesDnsConfig, ServicesNtpConfig } from '@ngfw/schema';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';

/**
 * F-unbound-chrony-syslog data access: the live state routes (`/api/v1/state/dns|ntp|syslog|logs`), the DNS lookup
 * action and the candidate's `services.dns`, `services.ntp` and `management.syslog` through the generic pointer routes.
 * D-132: nothing here polls faster than every 30 s; every panel has a Refresh button.
 */

type Ok<O> = O extends { responses: { 200: { content: { 'application/json': infer T } } } }
  ? T
  : never;
type Query<O> = O extends { parameters: { query?: infer Q } } ? NonNullable<Q> : never;

/** The live state shapes as generated from the OpenAPI document (never hand-written). */
export type DnsState = Ok<NonNullable<paths['/api/v1/state/dns']['get']>>;
export type NtpState = Ok<NonNullable<paths['/api/v1/state/ntp']['get']>>;
export type SyslogState = Ok<NonNullable<paths['/api/v1/state/syslog']['get']>>;
export type LogsPage = Ok<NonNullable<paths['/api/v1/state/logs']['get']>>;
export type LogEntry = LogsPage['items'][number];
export type LogsQuery = Query<NonNullable<paths['/api/v1/state/logs']['get']>>;
export type LookupResult = Ok<NonNullable<paths['/api/v1/actions/dns-lookup']['post']>>;

/** The configuration shapes (packages/schema — the one schema). */
export type DnsResolver = ServicesDnsConfig['resolvers'][string];
export type DnsVppCache = NonNullable<ServicesDnsConfig['vppCache']>;
export type { ServicesDnsConfig, ServicesNtpConfig };
export type SyslogTarget = ManagementConfig['syslog'][number];

/** D-132: at most one state poll per 30 s. */
export const STATE_POLL_MS = 30_000;

export const ucsKeys = {
  dns: ['state', 'dns'] as const,
  ntp: ['state', 'ntp'] as const,
  syslog: ['state', 'syslog'] as const,
  logs: ['state', 'logs'] as const,
};

export function useDnsState() {
  return useQuery({
    queryKey: ucsKeys.dns,
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/dns', { signal }))).data as DnsState,
    refetchInterval: STATE_POLL_MS,
  });
}

export function useNtpState() {
  return useQuery({
    queryKey: ucsKeys.ntp,
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/ntp', { signal }))).data as NtpState,
    refetchInterval: STATE_POLL_MS,
  });
}

export function useSyslogState() {
  return useQuery({
    queryKey: ucsKeys.syslog,
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/syslog', { signal }))).data as SyslogState,
    refetchInterval: STATE_POLL_MS,
  });
}

export async function fetchLogs(query: LogsQuery, signal: AbortSignal): Promise<LogsPage> {
  return (await call(api.GET('/api/v1/state/logs', { params: { query }, signal }))).data;
}

export function useDnsLookup() {
  return useMutation({
    mutationFn: async (body: { name: string; timeoutMs?: number }) =>
      (await call(api.POST('/api/v1/actions/dns-lookup', { body }))).data as LookupResult,
  });
}

type Json = Record<string, unknown>;

async function fetchDomain(domain: string, signal: AbortSignal): Promise<Json> {
  const r = await call(
    api.GET('/api/v1/config/candidate/{path}', { params: { path: { path: domain } }, signal }),
  );
  return (r.data ?? {}) as Json;
}

/**
 * One key of a candidate domain (`services` → `dns`, `ntp`; `management` → `syslog`). The pointer routes take one path
 * segment through openapi-fetch (a nested pointer would be URL-encoded, D-P07b-5), so reads and writes go through the
 * domain node; the query key is the domain's, shared with every other screen of that domain.
 */
export function useCandidateNode<T>(domain: 'services' | 'management', key: string) {
  return useQuery({
    queryKey: qk.candidate(domain),
    queryFn: ({ signal }) => fetchDomain(domain, signal),
    select: (d: Json) => d[key] as T | undefined,
  });
}

/** The same node right now (never write back a snapshot up to one poll old). */
export function useFreshNode<T>(domain: 'services' | 'management', key: string) {
  const qc = useQueryClient();
  return async (): Promise<T | undefined> =>
    (
      await qc.fetchQuery({
        queryKey: qk.candidate(domain),
        queryFn: ({ signal }) => fetchDomain(domain, signal),
        staleTime: 0,
      })
    )[key] as T | undefined;
}

/** An RFC 7396 merge patch of one candidate domain (the generic pointer route). */
export function usePatchDomain(domain: 'services' | 'management') {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (patch: Json) =>
      call(api.PATCH('/api/v1/config/{path}', { params: { path: { path: domain } }, body: patch })),
    onSettled: () => invalidateConfig(qc),
  });
}

const isObj = (v: unknown): v is Json => typeof v === 'object' && v !== null && !Array.isArray(v);

/** The merge patch that turns `before` into exactly `after` (members `after` dropped become `null`; arrays replace). */
export function replacePatch(before: unknown, after: unknown): unknown {
  if (!isObj(before) || !isObj(after)) return after;
  const out: Json = {};
  for (const [k, v] of Object.entries(after)) out[k] = k in before ? replacePatch(before[k], v) : v;
  for (const k of Object.keys(before)) if (!(k in after)) out[k] = null;
  return out;
}
