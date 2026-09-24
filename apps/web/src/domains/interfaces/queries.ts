import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../api';
import { call } from '../../api-problem';
import { invalidateConfig, qk } from '../../config/queries';
import type { InterfacesConfig } from './model';

export const ifaceKeys = {
  state: ['state', 'interfaces'] as const,
};

/** Live table refresh; counters/rates come from the WS topic in between. */
export const STATE_POLL_MS = 3_000;

export async function fetchInterfacesState(signal?: AbortSignal) {
  return (await call(api.GET('/api/v1/state/interfaces', signal ? { signal } : {}))).data;
}

export function useInterfacesState() {
  return useQuery({ queryKey: ifaceKeys.state, queryFn: ({ signal }) => fetchInterfacesState(signal), refetchInterval: STATE_POLL_MS });
}

async function fetchCandidateInterfaces(signal?: AbortSignal): Promise<InterfacesConfig> {
  const r = await call(api.GET('/api/v1/config/candidate/{path}', { params: { path: { path: 'interfaces' } }, ...(signal ? { signal } : {}) }));
  return (r.data ?? {}) as InterfacesConfig;
}

/** `interfaces` of the candidate (equals running when nobody edits). */
export function useCandidateInterfaces() {
  return useQuery({ queryKey: qk.candidate('interfaces'), queryFn: ({ signal }) => fetchCandidateInterfaces(signal) });
}

/** A merge patch of the candidate's `interfaces` node (the generic P06 pointer route, nothing interface-specific). */
export function usePatchInterfaces() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (patch: Record<string, unknown>) =>
      call(api.PATCH('/api/v1/config/{path}', { params: { path: { path: 'interfaces' } }, body: patch })),
    onSettled: () => Promise.all([invalidateConfig(qc), qc.invalidateQueries({ queryKey: ifaceKeys.state })]),
  });
}

/** The candidate's interfaces right now (never write back a snapshot up to one poll old). */
export function useFreshCandidate() {
  const qc = useQueryClient();
  return () => qc.fetchQuery({ queryKey: qk.candidate('interfaces'), queryFn: ({ signal }) => fetchCandidateInterfaces(signal), staleTime: 0 });
}
