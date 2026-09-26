import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';
import type { HostAclConfig } from './model';

export const hostAclKeys = {
  state: ['state', 'host-acl'] as const,
  running: ['config', 'running', 'acl'] as const,
};

/** D-132: no UI timer below 30 s on live state; the page has a Refresh button. */
export const STATE_POLL_MS = 30_000;

async function fetchCandidateAcl(signal?: AbortSignal): Promise<HostAclConfig> {
  const r = await call(
    api.GET('/api/v1/config/candidate/{path}', {
      params: { path: { path: 'acl' } },
      ...(signal ? { signal } : {}),
    }),
  );
  return (r.data ?? {}) as HostAclConfig;
}

/** `acl` of the candidate (equals running when nobody edits). */
export function useCandidateAcl() {
  return useQuery({
    queryKey: qk.candidate('acl'),
    queryFn: ({ signal }) => fetchCandidateAcl(signal),
  });
}

/** The candidate's `acl` right now (never write an array back from a snapshot up to one poll old). */
export function useFreshCandidateAcl() {
  const qc = useQueryClient();
  return () =>
    qc.fetchQuery({
      queryKey: qk.candidate('acl'),
      queryFn: ({ signal }) => fetchCandidateAcl(signal),
      staleTime: 0,
    });
}

/** `acl` of the running configuration (pending marks, banner). */
export function useRunningAcl() {
  return useQuery({
    queryKey: hostAclKeys.running,
    queryFn: async ({ signal }) => {
      const r = await call(
        api.GET('/api/v1/config/{path}', { params: { path: { path: 'acl' } }, signal }),
      );
      return (r.data ?? {}) as HostAclConfig;
    },
  });
}

/** A merge patch of the candidate's `acl` node (the generic P06 pointer route). */
export function usePatchAcl() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (patch: Record<string, unknown>) =>
      call(api.PATCH('/api/v1/config/{path}', { params: { path: { path: 'acl' } }, body: patch })),
    onSettled: () =>
      Promise.all([invalidateConfig(qc), qc.invalidateQueries({ queryKey: hostAclKeys.state })]),
  });
}

/** The rendered table and its counters (HostAclState). */
export function useHostAclState() {
  return useQuery({
    queryKey: hostAclKeys.state,
    queryFn: async ({ signal }) => (await call(api.GET('/api/v1/state/host-acl', { signal }))).data,
    refetchInterval: STATE_POLL_MS,
    retry: false,
  });
}
