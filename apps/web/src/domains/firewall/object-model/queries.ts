import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';
import type { ObjectKind, ObjectsConfig } from './model';

export const objectKeys = {
  fqdn: ['state', 'objects', 'fqdn'] as const,
  usage: (name: string, source: 'running' | 'candidate') => ['state', 'objects', 'usage', { name, source }] as const,
  running: ['config', 'running', 'objects'] as const,
};

/** FQDN resolution changes on the agent's schedule (≥ 30 s): a 5 s poll keeps the column current without a WS topic. */
export const FQDN_POLL_MS = 5_000;

async function fetchCandidateObjects(signal?: AbortSignal): Promise<Partial<ObjectsConfig>> {
  const r = await call(api.GET('/api/v1/config/candidate/{path}', { params: { path: { path: 'objects' } }, ...(signal ? { signal } : {}) }));
  return (r.data ?? {}) as Partial<ObjectsConfig>;
}

/** `objects` of the candidate (equals running when nobody edits). */
export function useCandidateObjects() {
  return useQuery({ queryKey: qk.candidate('objects'), queryFn: ({ signal }) => fetchCandidateObjects(signal) });
}

/** The candidate's objects right now (never write back a snapshot up to one poll old). */
export function useFreshCandidateObjects() {
  const qc = useQueryClient();
  return () => qc.fetchQuery({ queryKey: qk.candidate('objects'), queryFn: ({ signal }) => fetchCandidateObjects(signal), staleTime: 0 });
}

/** `objects` of the running configuration (pending-change marks). */
export function useRunningObjects() {
  return useQuery({
    queryKey: objectKeys.running,
    queryFn: async ({ signal }) => {
      const r = await call(api.GET('/api/v1/config/{path}', { params: { path: { path: 'objects' } }, signal }));
      return (r.data ?? {}) as Partial<ObjectsConfig>;
    },
  });
}

/** A merge patch of the candidate's `objects` node (the generic P06 pointer route). */
export function usePatchObjects() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (patch: Partial<Record<ObjectKind, Record<string, unknown>>>) =>
      call(api.PATCH('/api/v1/config/{path}', { params: { path: { path: 'objects' } }, body: patch })),
    onSettled: () => Promise.all([invalidateConfig(qc), qc.invalidateQueries({ queryKey: ['state', 'objects'] })]),
  });
}

export function useFqdnState() {
  return useQuery({
    queryKey: objectKeys.fqdn,
    queryFn: async ({ signal }) => (await call(api.GET('/api/v1/state/objects/fqdn', { signal }))).data,
    refetchInterval: FQDN_POLL_MS,
    retry: false,
  });
}

export function useUsage(name: string | null, source: 'running' | 'candidate') {
  return useQuery({
    queryKey: objectKeys.usage(name ?? '', source),
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/objects/usage', { params: { query: { name: name!, source } }, signal }))).data,
    enabled: name !== null,
  });
}
