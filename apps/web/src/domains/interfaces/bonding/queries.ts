import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { fetchInterfacesState, ifaceKeys } from '../queries';

export const bondKeys = {
  state: ['state', 'interfaces', 'bonds'] as const,
};

/**
 * D-132: a list view never auto-polls a VPP walk faster than every 30 s. The grid is the page's ONE timer; the drawer reads
 * its cache, and the Refresh button (or a saved change) refreshes on demand.
 */
export const BONDS_POLL_MS = 30_000;

export async function fetchBondsState(signal?: AbortSignal) {
  return (await call(api.GET('/api/v1/state/interfaces/bonds', signal ? { signal } : {}))).data;
}

/** The bond table the grid fetched (no timer and no fetch of its own: `enabled: false` only reads and follows the cache). */
export function useBondsCache() {
  return useQuery({ queryKey: bondKeys.state, queryFn: ({ signal }) => fetchBondsState(signal), enabled: false });
}

/** One live interface table when the drawer opens (the member picker); no timer (D-132). */
export function useInterfacesOnce() {
  return useQuery({ queryKey: ifaceKeys.state, queryFn: ({ signal }) => fetchInterfacesState(signal), staleTime: Infinity, refetchOnWindowFocus: false });
}

/** Refresh now: the grid refetches once (one BondState walk); the drawer follows the cache. */
export function useRefreshBonds() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: bondKeys.state });
}
