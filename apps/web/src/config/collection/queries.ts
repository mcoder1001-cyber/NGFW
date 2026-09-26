import type { RootKey } from '@ngfw/schema';
import { useMutation, useQuery, useQueryClient, type QueryKey } from '@tanstack/react-query';
import { useCallback } from 'react';
import { api } from '../../api';
import { call } from '../../api-problem';
import { invalidateConfig, POLL_MS, qk } from '../queries';

/**
 * One configuration domain from the API, candidate or running. The kit caches the WHOLE node here, while P08
 * (`qk.candidate('interfaces')` = `['config','candidate','interfaces']`) and Users (the same shape for 'management',
 * plus `['config','running','management']`) cache one member of it (`.users`, `.interfaces`) under that same key.
 * Review H1: sharing the exact key made Users read the kit's cached node object where it expected a `ConfigUser[]`
 * and crash ("running.map is not a function"). The kit therefore gets its own key space, one level below the same
 * `['config', which, domain]` prefix, so the shell's `['config']` invalidation and any `qk.candidate(domain)` /
 * `['config','running',domain]` prefix invalidation still reach it, but no exact key is ever shared.
 */
export const nodeKeys = {
  candidate: (domain: RootKey) => [...qk.candidate(domain), 'node'] as const,
  running: (domain: RootKey) => ['config', 'running', domain, 'node'] as const,
};

export type Which = 'candidate' | 'running';

export async function fetchDomainNode(domain: RootKey, which: Which, signal?: AbortSignal): Promise<unknown> {
  const opts = { params: { path: { path: domain } }, ...(signal ? { signal } : {}) };
  const r = which === 'candidate' ? await call(api.GET('/api/v1/config/candidate/{path}', opts)) : await call(api.GET('/api/v1/config/{path}', opts));
  return r.data ?? {};
}

/** `GET /api/v1/config[/candidate]/<domain>` (redacted by the API: write-only members never arrive). */
export function useDomainNode(domain: RootKey, which: Which, refetchInterval: number | false = POLL_MS) {
  return useQuery({
    queryKey: which === 'candidate' ? nodeKeys.candidate(domain) : nodeKeys.running(domain),
    queryFn: ({ signal }) => fetchDomainNode(domain, which, signal),
    refetchInterval,
  });
}

/** The candidate domain as it is NOW (review L4/N4: never write back a snapshot up to one poll old). */
export function useFreshCandidate(domain: RootKey): () => Promise<unknown> {
  const qc = useQueryClient();
  return useCallback(
    () => qc.fetchQuery({ queryKey: nodeKeys.candidate(domain), queryFn: ({ signal }) => fetchDomainNode(domain, 'candidate', signal), staleTime: 0 }),
    [qc, domain],
  );
}

/**
 * RFC 7386 merge patch of the candidate's `<domain>` node through the generic P06 pointer route. Settles by refreshing
 * `['config']` (diff, lock, candidate, running) and the extra keys a screen shows (its live state table).
 */
export function usePatchDomain(domain: RootKey, alsoInvalidate: readonly QueryKey[] = []) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (patch: unknown) => call(api.PATCH('/api/v1/config/{path}', { params: { path: { path: domain } }, body: patch })),
    onSettled: () => Promise.all([invalidateConfig(qc), ...alsoInvalidate.map((queryKey) => qc.invalidateQueries({ queryKey }))]),
  });
}
