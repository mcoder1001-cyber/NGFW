import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';
import type { LispConfig, LispState } from './model';

export const lispKeys = { state: ['state', 'lisp'] as const };

/** Refresh of the live LISP state. */
export const LISP_POLL_MS = 5_000;

export function useLispState() {
  return useQuery({
    queryKey: lispKeys.state,
    queryFn: async ({ signal }): Promise<LispState> => (await call(api.GET('/api/v1/state/lisp', { signal }))).data,
    refetchInterval: LISP_POLL_MS,
    retry: false,
  });
}

/** `tunnels.lisp` of the candidate (the generic P06 pointer route on `tunnels`). */
export function useCandidateLisp() {
  return useQuery({
    queryKey: qk.candidate('tunnels'),
    queryFn: async ({ signal }) => {
      const r = await call(api.GET('/api/v1/config/candidate/{path}', { params: { path: { path: 'tunnels' } }, signal }));
      return ((r.data ?? {}) as { lisp?: LispConfig }).lisp;
    },
  });
}

/** A merge patch of the candidate's `tunnels` node. */
export function usePatchTunnels() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (patch: Record<string, unknown>) =>
      call(api.PATCH('/api/v1/config/{path}', { params: { path: { path: 'tunnels' } }, body: patch })),
    onSettled: () => Promise.all([invalidateConfig(qc), qc.invalidateQueries({ queryKey: lispKeys.state })]),
  });
}
