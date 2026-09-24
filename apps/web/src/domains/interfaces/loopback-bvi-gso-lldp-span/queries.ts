import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';
import { ifaceKeys } from '../queries';
import type { LldpConfig, NeighborsPage, NsimConfig } from './model';

export const lldpKeys = {
  neighbors: ['state', 'lldp', 'neighbors'] as const,
};

/** Live table refresh of the LLDP table (VPP hears a peer every tx interval, 30 s by default). */
export const LLDP_POLL_MS = 5_000;

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
        qc.invalidateQueries({ queryKey: ifaceKeys.state }),
      ]),
  });
}
