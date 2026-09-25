import type { paths } from '@ngfw/api-client';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useCallback } from 'react';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';
import { mergePatchFor, type MplsConfig } from './model';

type Ok<O> = O extends { responses: { 200: { content: { 'application/json': infer T } } } }
  ? T
  : never;

/** `GET /api/v1/state/routing/mpls/fib` / `…/tunnels`, as generated from the OpenAPI document. */
export type MplsFibPage = Ok<NonNullable<paths['/api/v1/state/routing/mpls/fib']['get']>>;
export type MplsFibItem = MplsFibPage['items'][number];
export type MplsFibQuery = NonNullable<
  NonNullable<paths['/api/v1/state/routing/mpls/fib']['get']>['parameters']['query']
>;
export type MplsTunnels = Ok<NonNullable<paths['/api/v1/state/routing/mpls/tunnels']['get']>>;
export type MplsTunnelItem = MplsTunnels['items'][number];

export const mplsStateKeys = { all: ['state', 'routing', 'mpls'] as const };

/**
 * Live MPLS state is read on demand (D-132): every FIB read is one full walk of the table in VPP under its worker
 * barrier, and the agent runs one walk at a time. Nothing polls faster than 30 s; the tabs have a Refresh button.
 */
export const MPLS_STATUS_POLL_MS = 60_000;
export const ON_DEMAND = {
  staleTime: Infinity,
  refetchOnWindowFocus: false,
  refetchOnReconnect: false,
  retry: 1,
} as const;

export async function fetchMplsFib(
  query: MplsFibQuery,
  signal?: AbortSignal,
): Promise<MplsFibPage> {
  return (
    await call(
      api.GET('/api/v1/state/routing/mpls/fib', {
        params: { query },
        ...(signal ? { signal } : {}),
      }),
    )
  ).data;
}

export function useMplsTunnels() {
  return useQuery({
    queryKey: [...mplsStateKeys.all, 'tunnels'],
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/routing/mpls/tunnels', { signal }))).data,
    ...ON_DEMAND,
    refetchInterval: MPLS_STATUS_POLL_MS,
  });
}

/** Re-reads every live MPLS query on screen (the Refresh buttons). */
export function useRefreshMpls() {
  const qc = useQueryClient();
  return useCallback(() => void qc.invalidateQueries({ queryKey: mplsStateKeys.all }), [qc]);
}

/** The candidate `routing` node; equals running when nobody edits. */
export function useRoutingCandidate() {
  return useQuery({
    queryKey: qk.candidate('routing'),
    queryFn: async ({ signal }) =>
      ((
        await call(
          api.GET('/api/v1/config/candidate/{path}', {
            params: { path: { path: 'routing' } },
            signal,
          }),
        )
      ).data ?? undefined) as { mpls?: MplsConfig } | undefined,
  });
}

/** Names of the candidate's interfaces (and sub-interfaces), for the interface pickers. */
export function useInterfaceNames() {
  const q = useQuery({
    queryKey: qk.candidate('interfaces'),
    queryFn: async ({ signal }) =>
      ((
        await call(
          api.GET('/api/v1/config/candidate/{path}', {
            params: { path: { path: 'interfaces' } },
            signal,
          }),
        )
      ).data ?? undefined) as
        Record<string, { subinterfaces?: Record<string, unknown> }> | undefined,
  });
  return Object.entries(q.data ?? {}).flatMap(([n, i]) => [
    n,
    ...Object.keys(i.subinterfaces ?? {}).map((s) => `${n}.${s}`),
  ]);
}

/** VRF names of the candidate (`default` first). */
export function useVrfNames() {
  const q = useQuery({
    queryKey: qk.candidate('vrfs'),
    queryFn: async ({ signal }) =>
      ((
        await call(
          api.GET('/api/v1/config/candidate/{path}', {
            params: { path: { path: 'vrfs' } },
            signal,
          }),
        )
      ).data ?? undefined) as Record<string, unknown> | undefined,
  });
  return ['default', ...Object.keys(q.data ?? {}).filter((v) => v !== 'default')];
}

/**
 * Writes a new `routing.mpls` into the candidate: one RFC 7386 merge patch of `routing` (the generic P06 pointer route,
 * nothing MPLS-specific) computed from the current node, so removed tunnels / policies / tables become `null` and
 * arrays are replaced whole. `undefined` removes MPLS.
 */
export function useWriteMpls() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({
      from,
      to,
    }: {
      from: MplsConfig | undefined;
      to: MplsConfig | undefined;
    }) => {
      const patch = to === undefined ? null : from === undefined ? to : mergePatchFor(from, to);
      if (patch === undefined) return undefined;
      return call(
        api.PATCH('/api/v1/config/{path}', {
          params: { path: { path: 'routing' } },
          body: { mpls: patch } as never,
        }),
      );
    },
    onSettled: () => invalidateConfig(qc),
  });
}

/** An empty MPLS section with the schema's defaults (the base the first edit starts from). */
export function emptyMpls(): MplsConfig {
  return {
    interfaces: [],
    tables: {},
    labelRoutes: [],
    ipBindings: [],
    tunnels: {},
    sr: { policies: {}, steering: [] },
  };
}
