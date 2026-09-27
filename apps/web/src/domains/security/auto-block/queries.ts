import type { paths } from '@ngfw/api-client';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';

type Ok<P extends keyof paths, M extends 'get' | 'post'> = paths[P][M] extends {
  responses: { 200: { content: { 'application/json': infer T } } };
}
  ? T
  : never;

export type AutoBlockList = Ok<'/api/v1/state/auto-block', 'get'>;
export type BlockedEntry = AutoBlockList['items'][number];

export const autoBlockKeys = { list: ['state', 'auto-block'] as const };

/** D-132: no live UI timer below 30 s; the page has a Refresh button. */
export const POLL_MS = 30_000;

export function useAutoBlock() {
  return useQuery({
    queryKey: autoBlockKeys.list,
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/auto-block', { signal }))).data as AutoBlockList,
    refetchInterval: POLL_MS,
  });
}

export function useUnblock() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (source: string) =>
      call(api.POST('/api/v1/actions/auto-block/unblock', { body: { source } })),
    onSettled: () => qc.invalidateQueries({ queryKey: autoBlockKeys.list }),
  });
}

export interface ManualBlockArgs {
  source: string;
  blockSec?: number;
  note?: string;
}

export function useManualBlock() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ source, blockSec, note }: ManualBlockArgs) =>
      call(
        api.POST('/api/v1/actions/auto-block/block', {
          body: { source, ...(blockSec === undefined ? {} : { blockSec }), note: note ?? '' },
        }),
      ),
    onSettled: () => qc.invalidateQueries({ queryKey: autoBlockKeys.list }),
  });
}
