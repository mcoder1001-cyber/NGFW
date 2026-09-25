import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';
import type { ServicesCfg } from './model';

/** Query keys mirror API paths (docs/05). */
export const qosKeys = {
  policers: ['state', 'services', 'qos', 'policers'] as const,
};

/**
 * Live policer state (configuration, token buckets and counters). D-132: the agent walks VPP's policer pool for it, so
 * it is never polled faster than every 30 s; the page's Refresh button reads on demand.
 */
export const QOS_POLL_MS = 30_000;

/** Every QoS state query (Refresh button, after an edit or a reset). */
export const QOS_STATE_KEY = ['state', 'services', 'qos'] as const;

export async function fetchQosState(signal?: AbortSignal) {
  return (
    await call(api.GET('/api/v1/state/services/qos/policers', signal ? { signal } : {}))
  ).data;
}

export function useQosState() {
  return useQuery({
    queryKey: qosKeys.policers,
    queryFn: ({ signal }) => fetchQosState(signal),
    refetchInterval: QOS_POLL_MS,
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

/** A merge patch of the candidate's `services` node (the generic pointer route, nothing QoS-specific on the server). */
export function usePatchServices() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (patch: Record<string, unknown>) =>
      call(
        api.PATCH('/api/v1/config/{path}', { params: { path: { path: 'services' } }, body: patch }),
      ),
    onSettled: () =>
      Promise.all([invalidateConfig(qc), qc.invalidateQueries({ queryKey: QOS_STATE_KEY })]),
  });
}

/** `POST /api/v1/actions/qos/policers/{name}/reset` (policer_reset: refills the token buckets; VPP keeps the counters). */
export function useResetPolicer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (vppName: string) =>
      (
        await call(
          api.POST('/api/v1/actions/qos/policers/{name}/reset', {
            params: { path: { name: vppName } },
          }),
        )
      ).data,
    onSettled: () => qc.invalidateQueries({ queryKey: QOS_STATE_KEY }),
  });
}
