import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';

/** `dataplane` as the candidate / running configuration holds it (packages/schema domains/dataplane.ts). */
export interface DataplaneConfig {
  workers?: number;
  corelist?: number[];
  mainCore?: number;
  rxQueues?: number;
  txQueues?: number;
  hugepagesGb?: number;
  buffersPerNuma?: number;
  pciWhitelist?: string[];
  managementPci?: string[];
  devices?: Record<
    string,
    { name?: string; rxQueues?: number; txQueues?: number; rxDesc?: number; txDesc?: number }
  >;
  plugins?: { switches?: Record<string, boolean> };
}

const PATH = 'dataplane';
export const DATAPLANE_POLL_MS = 5_000;
export const dataplaneKeys = {
  running: ['config', 'running', PATH] as const,
  state: ['state', 'dataplane'] as const,
};

export function useCandidateDataplane() {
  return useQuery({
    queryKey: qk.candidate(PATH),
    queryFn: async ({ signal }) =>
      ((
        await call(
          api.GET('/api/v1/config/candidate/{path}', { params: { path: { path: PATH } }, signal }),
        )
      ).data ?? {}) as DataplaneConfig,
  });
}

export function useRunningDataplane() {
  return useQuery({
    queryKey: dataplaneKeys.running,
    queryFn: async ({ signal }) =>
      ((await call(api.GET('/api/v1/config/{path}', { params: { path: { path: PATH } }, signal })))
        .data ?? {}) as DataplaneConfig,
    refetchInterval: DATAPLANE_POLL_MS,
  });
}

/** The installed VPP start-up file + host facts (GET /api/v1/state/dataplane). */
export function useDataplaneState() {
  return useQuery({
    queryKey: dataplaneKeys.state,
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/dataplane', { signal }))).data,
    refetchInterval: 30_000,
  });
}

/** Read-only: renders startup.conf for the candidate and diffs it against the installed file. */
export function usePreviewDataplane() {
  return useMutation({
    mutationFn: async () => (await call(api.POST('/api/v1/actions/dataplane/preview'))).data,
  });
}

/** A merge patch of the candidate's `dataplane`; the pending-change bar commits it. */
export function usePatchDataplane() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (patch: Record<string, unknown>) =>
      call(api.PATCH('/api/v1/config/{path}', { params: { path: { path: PATH } }, body: patch })),
    onSettled: () => invalidateConfig(qc),
  });
}
