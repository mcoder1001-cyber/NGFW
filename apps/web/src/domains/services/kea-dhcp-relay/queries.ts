import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';
import type { LeasesQuery, ServicesCfg } from './model';

/** Query keys mirror API paths (docs/05). */
export const dhcpKeys = {
  leases: ['state', 'dhcp', 'leases'] as const,
  status: ['state', 'dhcp', 'leases', 'status'] as const,
  relays: ['state', 'dhcp', 'relays'] as const,
};

/**
 * Live status refresh (daemon state, pool usage, relays, lease page). D-132: nothing that walks VPP (the relays come
 * from Retrieve = dhcp_proxy_dump) polls faster than every 30 s; the page's Refresh button reads on demand.
 */
export const DHCP_POLL_MS = 30_000;

/** Every DHCP state query (Refresh button, after an edit). */
export const DHCP_STATE_KEY = ['state', 'dhcp'] as const;

export async function fetchLeases(query: LeasesQuery, signal?: AbortSignal) {
  return (
    await call(
      api.GET('/api/v1/state/dhcp/leases', { params: { query }, ...(signal ? { signal } : {}) }),
    )
  ).data;
}

/** kea-dhcp4/6 status and per-subnet pool usage (a one-lease page: the servers[] part is what matters here). */
export function useDhcpStatus() {
  return useQuery({
    queryKey: dhcpKeys.status,
    queryFn: ({ signal }) => fetchLeases({ page: 1, pageSize: 1 }, signal),
    refetchInterval: DHCP_POLL_MS,
  });
}

export function useRelayState() {
  return useQuery({
    queryKey: dhcpKeys.relays,
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/dhcp/relays', { signal }))).data,
    refetchInterval: DHCP_POLL_MS,
  });
}

async function fetchCandidateServices(signal?: AbortSignal): Promise<ServicesCfg> {
  const r = await call(
    api.GET('/api/v1/config/candidate/{path}', {
      params: { path: { path: 'services' } },
      ...(signal ? { signal } : {}),
    }),
  );
  return (r.data ?? {}) as ServicesCfg;
}

/** `services` of the candidate (equals running when nobody edits). */
export function useCandidateServices() {
  return useQuery({
    queryKey: qk.candidate('services'),
    queryFn: ({ signal }) => fetchCandidateServices(signal),
  });
}

/** The candidate's services right now (never write back a snapshot up to one poll old). */
export function useFreshServices() {
  const qc = useQueryClient();
  return () =>
    qc.fetchQuery({
      queryKey: qk.candidate('services'),
      queryFn: ({ signal }) => fetchCandidateServices(signal),
      staleTime: 0,
    });
}

/** A merge patch of the candidate's `services` node (the generic pointer route, nothing DHCP-specific on the server). */
export function usePatchServices() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (patch: Record<string, unknown>) =>
      call(
        api.PATCH('/api/v1/config/{path}', { params: { path: { path: 'services' } }, body: patch }),
      ),
    onSettled: () =>
      Promise.all([invalidateConfig(qc), qc.invalidateQueries({ queryKey: DHCP_STATE_KEY })]),
  });
}
