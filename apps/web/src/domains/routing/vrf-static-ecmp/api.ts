import type { paths } from '@ngfw/api-client';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useCallback } from 'react';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';

type Ok<O> = O extends { responses: { 200: { content: { 'application/json': infer T } } } } ? T : never;

/** `GET /api/v1/state/routes` (the FIB browser), as generated from the OpenAPI document. */
export type RoutesPage = Ok<NonNullable<paths['/api/v1/state/routes']['get']>>;
export type RouteItem = RoutesPage['items'][number];
export type RoutesQuery = NonNullable<NonNullable<paths['/api/v1/state/routes']['get']>['parameters']['query']>;
/** `POST /api/v1/actions/{action}` answer (ping). */
export type ActionResult = Ok<NonNullable<paths['/api/v1/actions/{action}']['post']>>;

export const routeKeys = { all: ['state', 'routes'] as const };

/**
 * The FIB is read on demand (review H1): every `/state/routes` call is one full walk of the VRF's table in VPP, under its
 * worker barrier, and the agent runs one walk at a time. So nothing polls the FIB browser or the VRF counts (a refresh
 * button does), and the static-routes "installed" status refreshes once a minute.
 */
export const FIB_STATUS_POLL_MS = 60_000;

/** Query options of every FIB read: never refetched by focus, reconnect or remount — only by a refresh or its interval. */
export const FIB_ON_DEMAND = { staleTime: Infinity, refetchOnWindowFocus: false, refetchOnReconnect: false, retry: 1 } as const;

/** Makes FIB_ON_DEMAND the default of every query under `['state', 'routes']` (the grids' queries included). */
export function useFibQueryDefaults() {
  useQueryClient().setQueryDefaults(routeKeys.all, FIB_ON_DEMAND);
}

/** Re-reads every FIB query on screen (the refresh buttons). */
export function useRefreshRoutes() {
  const qc = useQueryClient();
  return useCallback(() => void qc.invalidateQueries({ queryKey: routeKeys.all }), [qc]);
}

export async function fetchRoutes(query: RoutesQuery, signal?: AbortSignal): Promise<RoutesPage> {
  return (await call(api.GET('/api/v1/state/routes', { params: { query }, ...(signal ? { signal } : {}) }))).data;
}

/** One page of the live FIB (the agent pages it: ListRoutes), read once and on refresh. */
export function useRoutes(query: RoutesQuery, enabled = true) {
  return useQuery({
    queryKey: [...routeKeys.all, query],
    queryFn: ({ signal }) => fetchRoutes(query, signal),
    ...FIB_ON_DEMAND,
    enabled,
  });
}

/**
 * A top-level node of the candidate (`vrfs`, `routing`, `interfaces`); equals running when nobody edits. Only root keys:
 * a percent-encoded `/` would be read as part of one pointer segment by the API (config/path.ts).
 */
export function useCandidate<T>(key: 'vrfs' | 'routing' | 'interfaces') {
  return useQuery({
    queryKey: qk.candidate(key),
    queryFn: async ({ signal }) =>
      ((await call(api.GET('/api/v1/config/candidate/{path}', { params: { path: { path: key } }, signal }))).data ?? undefined) as T | undefined,
  });
}

/**
 * RFC 7386 merge patch of a top-level candidate node (the generic P06 pointer route, nothing VRF- or route-specific):
 * `{ red: {…} }` / `{ red: null }` for VRFs, `{ static: [...] }` (arrays are replaced whole) for static routes.
 */
export function usePatchConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ key, patch }: { key: 'vrfs' | 'routing'; patch: Record<string, unknown> }) =>
      call(api.PATCH('/api/v1/config/{path}', { params: { path: { path: key } }, body: patch as never })),
    onSettled: () => invalidateConfig(qc),
  });
}

export interface PingInput {
  target: string;
  count?: number;
  intervalMs?: number;
}

/** `POST /api/v1/actions/ping`: runs in the data plane (VPP's ping plugin, default VRF). */
export function usePing() {
  return useMutation({
    mutationFn: async (body: PingInput): Promise<ActionResult> =>
      (await call(api.POST('/api/v1/actions/{action}', { params: { path: { action: 'ping' } }, body: body as never }))).data,
  });
}
