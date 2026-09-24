import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';
import { ifaceKeys } from '../queries';
import type { BridgeL2Config, InterfaceConfig, MacsPage } from './model';

export const bridgeKeys = {
  state: ['state', 'l2', 'bridge-domains'] as const,
  macs: (id: number) => ['state', 'l2', 'bridge-domains', id, 'macs'] as const,
};

/** Live table refresh (the API merges running + candidate on every answer). */
export const BRIDGE_POLL_MS = 3_000;

export async function fetchBridgeDomains(signal?: AbortSignal) {
  return (await call(api.GET('/api/v1/state/l2/bridge-domains', signal ? { signal } : {}))).data;
}

export function useBridgeDomains() {
  return useQuery({
    queryKey: bridgeKeys.state,
    queryFn: ({ signal }) => fetchBridgeDomains(signal),
    refetchInterval: BRIDGE_POLL_MS,
    retry: false,
  });
}

export async function fetchMacs(
  id: number,
  page: number,
  pageSize: number,
  signal?: AbortSignal,
): Promise<MacsPage> {
  return (
    await call(
      api.GET('/api/v1/state/l2/bridge-domains/{id}/macs', {
        params: { path: { id }, query: { page, pageSize } },
        ...(signal ? { signal } : {}),
      }),
    )
  ).data;
}

async function fetchCandidate<T>(path: string, signal?: AbortSignal): Promise<T | undefined> {
  const r = await call(
    api.GET('/api/v1/config/candidate/{path}', {
      params: { path: { path } },
      ...(signal ? { signal } : {}),
    }),
  );
  return (r.data ?? undefined) as T | undefined;
}

/** `routing.l2` of the candidate. */
export function useCandidateL2() {
  return useQuery({
    queryKey: qk.candidate('routing'),
    queryFn: async ({ signal }): Promise<Partial<BridgeL2Config>> =>
      ((await fetchCandidate<{ l2?: BridgeL2Config }>('routing', signal)) ?? {}).l2 ?? {},
  });
}

/** `interfaces` of the candidate (members live on interfaces.<if>.l2). */
export function useCandidatePorts() {
  return useQuery({
    queryKey: qk.candidate('interfaces'),
    queryFn: async ({ signal }) =>
      (await fetchCandidate<Record<string, InterfaceConfig>>('interfaces', signal)) ?? {},
  });
}

/** A merge patch of a candidate domain node through the generic P06 pointer route. */
function usePatch(path: 'routing' | 'interfaces') {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (patch: Record<string, unknown>) =>
      call(api.PATCH('/api/v1/config/{path}', { params: { path: { path } }, body: patch })),
    onSettled: () =>
      Promise.all([
        invalidateConfig(qc),
        qc.invalidateQueries({ queryKey: bridgeKeys.state }),
        qc.invalidateQueries({ queryKey: ifaceKeys.state }),
      ]),
  });
}

export const usePatchRouting = () => usePatch('routing');
export const usePatchPorts = () => usePatch('interfaces');
