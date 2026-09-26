import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';
import type { PbrConfig } from './model';

/** Live PBR view refresh (the pending-change bar polls the diff at the same rate). */
export const PBR_POLL_MS = 5_000;

export const pbrKeys = {
  state: ['state', 'pbr'] as const,
  running: (path: string) => ['config', 'running', path] as const,
};

async function candidateAt<T>(path: string, signal?: AbortSignal): Promise<T | undefined> {
  const r = await call(
    api.GET('/api/v1/config/candidate/{path}', {
      params: { path: { path } },
      ...(signal ? { signal } : {}),
    }),
  );
  return r.data as T | undefined;
}

async function runningAt<T>(path: string, signal?: AbortSignal): Promise<T | undefined> {
  const r = await call(
    api.GET('/api/v1/config/{path}', { params: { path: { path } }, ...(signal ? { signal } : {}) }),
  );
  return r.data as T | undefined;
}

/** A top-level domain of the candidate (`routing`, `interfaces`, `acl`, `vrfs`, `services`). */
export function useCandidate<T>(domain: string) {
  return useQuery({
    queryKey: qk.candidate(domain),
    queryFn: ({ signal }) => candidateAt<T>(domain, signal),
  });
}

/** A top-level domain of running. */
export function useRunning<T>(domain: string) {
  return useQuery({
    queryKey: pbrKeys.running(domain),
    queryFn: ({ signal }) => runningAt<T>(domain, signal),
    refetchInterval: PBR_POLL_MS,
  });
}

/** `GET /api/v1/state/pbr` (all attachments on one page: the schema caps them at 4096). */
export function usePbrState() {
  return useQuery({
    queryKey: pbrKeys.state,
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/pbr', { params: { query: { pageSize: 1000 } }, signal })))
        .data,
    refetchInterval: PBR_POLL_MS,
  });
}

/** The candidate's routing.pbr right now (never write back a snapshot up to one poll old). */
export function useFreshPbr() {
  const qc = useQueryClient();
  return async (): Promise<PbrConfig> => {
    const routing = await qc.fetchQuery({
      queryKey: qk.candidate('routing'),
      queryFn: ({ signal }) => candidateAt<{ pbr?: PbrConfig }>('routing', signal),
      staleTime: 0,
    });
    return routing?.pbr ?? {};
  };
}

/** A merge patch at a candidate pointer path (`routing`, `interfaces`, `services`) — the generic P06 route. */
export function usePatch(path: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (patch: Record<string, unknown>) =>
      call(api.PATCH('/api/v1/config/{path}', { params: { path: { path } }, body: patch })),
    onSettled: () =>
      Promise.all([invalidateConfig(qc), qc.invalidateQueries({ queryKey: pbrKeys.state })]),
  });
}
