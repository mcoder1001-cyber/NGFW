import type { paths } from '@ngfw/api-client';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
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

/** Live table refresh of the FIB views. */
export const FIB_POLL_MS = 5_000;

export async function fetchRoutes(query: RoutesQuery, signal?: AbortSignal): Promise<RoutesPage> {
  return (await call(api.GET('/api/v1/state/routes', { params: { query }, ...(signal ? { signal } : {}) }))).data;
}

/** One page of the live FIB (the agent pages it: ListRoutes). */
export function useRoutes(query: RoutesQuery, enabled = true) {
  return useQuery({
    queryKey: [...routeKeys.all, query],
    queryFn: ({ signal }) => fetchRoutes(query, signal),
    refetchInterval: FIB_POLL_MS,
    enabled,
  });
}

/** A subtree of the candidate (`vrfs`, `routing/static`, `interfaces`); equals running when nobody edits. */
export function useCandidate<T>(path: string) {
  return useQuery({
    queryKey: qk.candidate(path),
    queryFn: async ({ signal }) =>
      ((await call(api.GET('/api/v1/config/candidate/{path}', { params: { path: { path } }, signal }))).data ?? undefined) as T | undefined,
  });
}

/** Replace a candidate subtree (the generic P06 pointer route) — nothing VRF- or route-specific in the API. */
export function usePutConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ path, body }: { path: string; body: unknown }) =>
      call(api.PUT('/api/v1/config/{path}', { params: { path: { path } }, body: body as never })),
    onSettled: () => invalidateConfig(qc),
  });
}

/** Delete a candidate subtree. */
export function useDeleteConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (path: string) => call(api.DELETE('/api/v1/config/{path}', { params: { path: { path } } })),
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
