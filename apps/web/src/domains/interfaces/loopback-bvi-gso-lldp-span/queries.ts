import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';
import { fetchInterfacesState, ifaceKeys as ifaceStateKeys } from '../queries';
import type { LldpConfig, NeighborsPage, NsimConfig } from './model';

export const lldpKeys = {
  neighbors: ['state', 'lldp', 'neighbors'] as const,
};

/**
 * Live refresh of the tables that walk VPP (the LLDP table; `/state/interfaces` on the mirroring page): at most every
 * 30 s, plus a Refresh button (D-132: a walk holds VPP's worker barrier; one walk at a time in the agent).
 */
export const WALK_POLL_MS = 30_000;

export async function fetchNeighbors(
  page: number,
  pageSize: number,
  signal?: AbortSignal,
): Promise<NeighborsPage> {
  return (
    await call(
      api.GET('/api/v1/state/lldp/neighbors', {
        params: { query: { page, pageSize } },
        ...(signal ? { signal } : {}),
      }),
    )
  ).data;
}

/** `services` of the candidate (equals running when nobody edits). */
export function useCandidateServices() {
  return useQuery({
    queryKey: qk.candidate('services'),
    queryFn: async ({ signal }) => {
      const r = await call(
        api.GET('/api/v1/config/candidate/{path}', {
          params: { path: { path: 'services' } },
          ...(signal ? { signal } : {}),
        }),
      );
      return (r.data ?? {}) as { lldp?: LldpConfig; nsim?: NsimConfig };
    },
  });
}

/** A merge patch of the candidate's `services` node through the generic P06 pointer route. */
export function usePatchServices() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (patch: Record<string, unknown>) =>
      call(
        api.PATCH('/api/v1/config/{path}', { params: { path: { path: 'services' } }, body: patch }),
      ),
    onSettled: () =>
      Promise.all([
        invalidateConfig(qc),
        qc.invalidateQueries({ queryKey: lldpKeys.neighbors }),
        qc.invalidateQueries({ queryKey: ifaceStateKeys.state }),
      ]),
  });
}

/** `/state/interfaces` (the agent's Retrieve view carries the mirror sessions VPP has), polled every 30 s (D-132). */
export function useInterfacesStateSlow() {
  return useQuery({
    queryKey: ifaceStateKeys.state,
    queryFn: ({ signal }) => fetchInterfacesState(signal),
    refetchInterval: WALK_POLL_MS,
  });
}
