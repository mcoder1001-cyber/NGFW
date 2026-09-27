import type { paths } from '@ngfw/api-client';
import { useQuery } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';

type Ok<P extends keyof paths> = paths[P]['get'] extends {
  responses: { 200: { content: { 'application/json': infer T } } };
}
  ? T
  : never;

export type WanState = Ok<'/api/v1/state/wan'>;
export type WanGroupState = WanState['groups'][number];

/** D-132: no live UI timer below 30 s; a Refresh button is provided. */
export const POLL_MS = 30_000;

export function useWanState() {
  return useQuery({
    queryKey: ['state', 'wan'],
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/wan', { signal }))).data as WanState,
    refetchInterval: POLL_MS,
  });
}
