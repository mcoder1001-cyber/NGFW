import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';
import type { LbCandidate } from './model';

export const lbKeys = {
  vips: ['state', 'lb', 'vips'] as const,
};

/**
 * The agent walks VPP's lb tables for this view, one walk at a time (D-132): refresh at most every 30 s, plus the
 * Refresh button.
 */
export const LB_POLL_MS = 30_000;

export function useLbState() {
  return useQuery({
    queryKey: lbKeys.vips,
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/lb/vips', signal ? { signal } : {}))).data,
    refetchInterval: LB_POLL_MS,
  });
}

/** `services.lb` of the candidate (equals running when nobody edits); undefined = no load balancer configured. */
export function useCandidateLb() {
  return useQuery({
    queryKey: qk.candidate('services'),
    queryFn: async ({ signal }) => {
      const r = await call(
        api.GET('/api/v1/config/candidate/{path}', {
          params: { path: { path: 'services' } },
          ...(signal ? { signal } : {}),
        }),
      );
      return (r.data ?? {}) as { lb?: LbCandidate };
    },
    select: (s) => s.lb,
  });
}

/** A merge patch of the candidate's `services` node (`{ lb: … }`) through the generic P06 pointer route. */
export function usePatchLb() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (lbPatch: unknown) =>
      call(
        api.PATCH('/api/v1/config/{path}', {
          params: { path: { path: 'services' } },
          body: { lb: lbPatch },
        }),
      ),
    onSettled: () =>
      Promise.all([invalidateConfig(qc), qc.invalidateQueries({ queryKey: lbKeys.vips })]),
  });
}

/** `POST /api/v1/actions/lb/vips/{name}/flush`. */
export function useFlushVip() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (name: string) =>
      (await call(api.POST('/api/v1/actions/lb/vips/{name}/flush', { params: { path: { name } } })))
        .data,
    onSettled: () => qc.invalidateQueries({ queryKey: lbKeys.vips }),
  });
}
