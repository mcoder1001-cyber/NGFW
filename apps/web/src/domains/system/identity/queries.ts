import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';

/** `system` as the candidate / running configuration holds it (packages/schema domains/system.ts). */
export interface SystemConfig {
  hostname?: string;
  timezone?: string;
  banner?: { login?: string; motd?: string };
  dns?: { servers?: string[]; searchDomains?: string[]; vrf?: string };
}

const PATH = 'system';
/** The running column refreshes at the pending-change bar's rate. */
export const SYSTEM_POLL_MS = 5_000;
export const systemKeys = { running: ['config', 'running', PATH] as const };

export function useCandidateSystem() {
  return useQuery({
    queryKey: qk.candidate(PATH),
    queryFn: async ({ signal }) =>
      ((
        await call(
          api.GET('/api/v1/config/candidate/{path}', { params: { path: { path: PATH } }, signal }),
        )
      ).data ?? {}) as SystemConfig,
  });
}

export function useRunningSystem() {
  return useQuery({
    queryKey: systemKeys.running,
    queryFn: async ({ signal }) =>
      ((await call(api.GET('/api/v1/config/{path}', { params: { path: { path: PATH } }, signal })))
        .data ?? {}) as SystemConfig,
    refetchInterval: SYSTEM_POLL_MS,
  });
}

/** A merge patch of the candidate's `system` (the generic pointer route); the pending-change bar commits it. */
export function usePatchSystem() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (patch: Record<string, unknown>) =>
      call(api.PATCH('/api/v1/config/{path}', { params: { path: { path: PATH } }, body: patch })),
    onSettled: () => invalidateConfig(qc),
  });
}
