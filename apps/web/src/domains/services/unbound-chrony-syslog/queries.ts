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

/** A candidate node by path (`services/dns`, `services/ntp`, `management`). */
export function useCandidate<T>(path: string) {
  return useQuery({
    queryKey: qk.candidate(path),
    queryFn: async ({ signal }) =>
      (
        await call(
          api.GET('/api/v1/config/candidate/{path}', { params: { path: { path } }, signal }),
        )
      ).data as T,
  });
}

/** The candidate node right now (never write back a snapshot up to one poll old). */
export function useFreshCandidate<T>(path: string) {
  const qc = useQueryClient();
  return () =>
    qc.fetchQuery({
      queryKey: qk.candidate(path),
      queryFn: async ({ signal }) =>
        (
          await call(
            api.GET('/api/v1/config/candidate/{path}', { params: { path: { path } }, signal }),
          )
        ).data as T,
      staleTime: 0,
    });
}

/** A merge patch of one candidate node (the generic pointer route; RFC 7396 — `null` deletes a member). */
export function usePatchCandidate(path: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (patch: Record<string, unknown>) =>
      call(api.PATCH('/api/v1/config/{path}', { params: { path: { path } }, body: patch })),
    onSettled: () => invalidateConfig(qc),
  });
}

/** Replace one candidate node (PUT on the pointer route: lists such as management/syslog are replaced whole). */
export function usePutCandidate(path: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (value: unknown) =>
      call(api.PUT('/api/v1/config/{path}', { params: { path: { path } }, body: value as never })),
    onSettled: () => invalidateConfig(qc),
  });
}
