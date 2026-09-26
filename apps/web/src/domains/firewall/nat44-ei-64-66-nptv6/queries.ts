import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import type { SessionFilter } from '../nat44-ed-sessions/model';
import { filterQuery } from '../nat44-ed-sessions/model';
import type { EiKillBody } from './model';

/**
 * D-132: a view that makes the agent walk a VPP table (NAT session tables, the Retrieve behind /state/drift) is
 * refreshed at most every 30 s — and by hand (Refresh) — never faster; a session-level filter stops the timer.
 */
export const POLL_MS = 30_000;

/** Query keys mirror the API paths. */
export const keys = {
  eiSessions: ['state', 'nat', 'ei', 'sessions'] as const,
  nat64Sessions: ['state', 'nat', 'nat64', 'sessions'] as const,
  nptv6: ['state', 'nat', 'nptv6'] as const,
  drift: ['state', 'drift'] as const,
};

/** One server page of NAT44-EI sessions (the grid's 0-based page → the API's 1-based `page`). */
export async function fetchEiSessions(
  page: number,
  pageSize: number,
  filter: SessionFilter,
  signal?: AbortSignal,
) {
  const query = { page: page + 1, pageSize, ...filterQuery(filter) };
  return (
    await call(
      api.GET('/api/v1/state/nat/ei/sessions', {
        params: { query },
        ...(signal ? { signal } : {}),
      }),
    )
  ).data;
}

/** One server page of NAT64 sessions (protocol filter only). */
export async function fetchNat64Sessions(
  page: number,
  pageSize: number,
  protocol: string,
  signal?: AbortSignal,
) {
  const query = { page: page + 1, pageSize, ...(protocol ? { protocol } : {}) };
  return (
    await call(
      api.GET('/api/v1/state/nat/nat64/sessions', {
        params: { query },
        ...(signal ? { signal } : {}),
      }),
    )
  ).data;
}

/** Delete one NAT44-EI session (`POST /actions/nat/ei/sessions/kill`, audited by the API). */
export function useEiKill() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: EiKillBody) =>
      (await call(api.POST('/api/v1/actions/nat/ei/sessions/kill', { body }))).data,
    onSettled: () => qc.invalidateQueries({ queryKey: keys.eiSessions }),
  });
}

/** The running NPTv6 bindings with the write-only marker (npt66 has no dump). */
export function useNptv6State() {
  return useQuery({
    queryKey: keys.nptv6,
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/nat/nptv6', { signal }))).data,
    refetchInterval: POLL_MS,
    refetchIntervalInBackground: false,
  });
}

/** Running configuration vs what the agent retrieves (the live status of NAT66 rows). */
export function useDrift() {
  return useQuery({
    queryKey: keys.drift,
    queryFn: async ({ signal }) => (await call(api.GET('/api/v1/state/drift', { signal }))).data,
    refetchInterval: POLL_MS,
    refetchIntervalInBackground: false,
  });
}
