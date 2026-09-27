import type { paths } from '@ngfw/api-client';
import { useQuery } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';

type Ok<P extends keyof paths> = paths[P]['get'] extends {
  responses: { 200: { content: { 'application/json': infer T } } };
}
  ? T
  : never;

export type LdpNeighbors = Ok<'/api/v1/state/routing/mpls/ldp/neighbors'>;
export type LdpBindings = Ok<'/api/v1/state/routing/mpls/ldp/bindings'>;
export type LdpSync = Ok<'/api/v1/state/routing/mpls/ldp/sync'>;

export const ldpKeys = { all: ['state', 'mpls-ldp'] as const };

/** D-132: no live UI timer below 30 s; the tab has a Refresh button. */
export const POLL_MS = 30_000;

export function useLdpNeighbors() {
  return useQuery({
    queryKey: [...ldpKeys.all, 'neighbors'],
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/routing/mpls/ldp/neighbors', { signal }))).data as LdpNeighbors,
    refetchInterval: POLL_MS,
  });
}

export function useLdpBindings(page: number, pageSize: number) {
  return useQuery({
    queryKey: [...ldpKeys.all, 'bindings', page, pageSize],
    queryFn: async ({ signal }) =>
      (
        await call(
          api.GET('/api/v1/state/routing/mpls/ldp/bindings', { params: { query: { page, pageSize } }, signal }),
        )
      ).data as LdpBindings,
    refetchInterval: POLL_MS,
  });
}

export function useLdpSync() {
  return useQuery({
    queryKey: [...ldpKeys.all, 'sync'],
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/routing/mpls/ldp/sync', { signal }))).data as LdpSync,
    refetchInterval: POLL_MS,
  });
}
