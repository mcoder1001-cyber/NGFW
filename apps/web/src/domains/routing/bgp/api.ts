import type { paths } from '@ngfw/api-client';
import { useTopic } from '@ngfw/ui-kit/ws';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';

type Ok<O> = O extends { responses: { 200: { content: { 'application/json': infer T } } } }
  ? T
  : never;

/** `GET /api/v1/state/bgp` as generated from the OpenAPI document. */
export type BgpState = Ok<NonNullable<paths['/api/v1/state/bgp']['get']>>;

export const bgpKeys = { state: ['state', 'bgp'] as const };

/**
 * Refresh of the live BGP table. RoutingState reads FRR and the linux-cp pairs from VPP, so it is polled slowly (D-132:
 * nothing that walks VPP below 30 s); neighbour changes arrive at once through the `routing.events` topic, and the
 * screen has a Refresh button.
 */
export const BGP_POLL_MS = 30_000;

export function useBgpState() {
  return useQuery({
    queryKey: bgpKeys.state,
    queryFn: async ({ signal }) => (await call(api.GET('/api/v1/state/bgp', { signal }))).data,
    refetchInterval: BGP_POLL_MS,
  });
}

/** Refetches the BGP state on every routing event (neighbour state, FRR RIB counts). */
export function useRoutingEvents() {
  const qc = useQueryClient();
  useTopic('routing.events', {
    onBatch: () => {
      void qc.invalidateQueries({ queryKey: bgpKeys.state });
    },
  });
}

/** A top-level candidate node (equals running when nobody edits). */
export function useCandidate<T>(key: 'routing' | 'interfaces') {
  return useQuery({
    queryKey: qk.candidate(key),
    queryFn: async ({ signal }) =>
      ((
        await call(
          api.GET('/api/v1/config/candidate/{path}', { params: { path: { path: key } }, signal }),
        )
      ).data ?? undefined) as T | undefined,
  });
}

/**
 * RFC 7386 merge patch of a top-level candidate node through the generic pointer route: `{ bgp: {…} }`,
 * `{ policy: { prefixLists: { name: null } } }` (null deletes), `{ loop0: { lcp: {…} } }`. Records merge key by key;
 * arrays are replaced whole.
 */
export function usePatch() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({
      key,
      patch,
    }: {
      key: 'routing' | 'interfaces';
      patch: Record<string, unknown>;
    }) =>
      call(
        api.PATCH('/api/v1/config/{path}', {
          params: { path: { path: key } },
          body: patch as never,
        }),
      ),
    onSettled: () => invalidateConfig(qc),
  });
}
