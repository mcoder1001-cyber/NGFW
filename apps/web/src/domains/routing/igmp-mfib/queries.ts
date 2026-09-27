import type { paths } from '@ngfw/api-client';
import { useQuery } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';

type Ok<P extends keyof paths> = paths[P]['get'] extends {
  responses: { 200: { content: { 'application/json': infer T } } };
}
  ? T
  : never;

export type GroupsState = Ok<'/api/v1/state/routing/multicast/groups'>;
export type MroutesState = Ok<'/api/v1/state/routing/multicast/mroutes'>;
export type PimNeighborsState = Ok<'/api/v1/state/routing/multicast/pim-neighbors'>;

export const multicastKeys = { all: ['state', 'multicast'] as const };

/** D-132: no live UI timer below 30 s; the page has a Refresh button. */
export const POLL_MS = 30_000;

export function useMulticastGroups() {
  return useQuery({
    queryKey: [...multicastKeys.all, 'groups'],
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/routing/multicast/groups', { signal }))).data as GroupsState,
    refetchInterval: POLL_MS,
  });
}

export function useMroutes() {
  return useQuery({
    queryKey: [...multicastKeys.all, 'mroutes'],
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/routing/multicast/mroutes', { signal }))).data as MroutesState,
    refetchInterval: POLL_MS,
  });
}

export function usePimNeighbors() {
  return useQuery({
    queryKey: [...multicastKeys.all, 'pim'],
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/routing/multicast/pim-neighbors', { signal }))).data as PimNeighborsState,
    refetchInterval: POLL_MS,
  });
}
