import type { paths } from '@ngfw/api-client';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';

type Ok<P extends keyof paths, M extends 'get' | 'post'> = paths[P][M] extends {
  responses: { 200: { content: { 'application/json': infer T } } };
}
  ? T
  : never;

export type DashboardSummary = Ok<'/api/v1/state/dashboard', 'get'>;
export type AlarmsList = Ok<'/api/v1/state/alarms', 'get'>;
export type Alarm = AlarmsList['items'][number];

export const alarmKeys = {
  dashboard: ['state', 'dashboard'] as const,
  list: (active?: boolean) => ['state', 'alarms', active ?? 'all'] as const,
};

/** D-132: no live UI timer below 30 s; the pages have a Refresh button. */
export const POLL_MS = 30_000;

export function useDashboardSummary() {
  return useQuery({
    queryKey: alarmKeys.dashboard,
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/dashboard', { signal }))).data as DashboardSummary,
    refetchInterval: POLL_MS,
  });
}

export function useAlarms(active?: boolean) {
  return useQuery({
    queryKey: alarmKeys.list(active),
    queryFn: async ({ signal }) =>
      (
        await call(
          api.GET('/api/v1/state/alarms', {
            params: { query: active === undefined ? {} : { active } },
            signal,
          }),
        )
      ).data as AlarmsList,
    refetchInterval: POLL_MS,
  });
}

export function useAckAlarm() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: number) =>
      call(api.POST('/api/v1/actions/alarms/{id}/ack', { params: { path: { id } } })),
    onSettled: () => qc.invalidateQueries({ queryKey: ['state', 'alarms'] }),
  });
}

/** `management.alarms` of the candidate (for the rules/targets editor). */
export function useCandidateAlarms() {
  return useQuery({
    queryKey: qk.candidate('management'),
    queryFn: async ({ signal }) => {
      const r = await call(
        api.GET('/api/v1/config/candidate/{path}', {
          params: { path: { path: 'management' } },
          ...(signal ? { signal } : {}),
        }),
      );
      const mgmt = (r.data ?? {}) as { alarms?: unknown };
      return (mgmt.alarms ?? {}) as Record<string, unknown>;
    },
  });
}

export function usePatchAlarms() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (patch: Record<string, unknown>) =>
      call(
        api.PATCH('/api/v1/config/{path}', {
          params: { path: { path: 'management' } },
          body: { alarms: patch },
        }),
      ),
    onSettled: () =>
      Promise.all([
        invalidateConfig(qc),
        qc.invalidateQueries({ queryKey: qk.candidate('management') }),
      ]),
  });
}
