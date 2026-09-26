import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';
import type { KillBody, SessionFilter } from './model';
import { filterQuery } from './model';

/** Query keys mirror the API paths. */
export const natKeys = {
  summary: ['state', 'nat', 'summary'] as const,
  sessions: ['state', 'nat', 'sessions'] as const,
};

/**
 * Refresh of the unfiltered session grid (no WS topic for NAT state). Every page walks VPP session tables under the
 * worker barrier, so it is never polled faster than every 30 s (D-132); a filtered grid is refreshed by hand only.
 */
export const NAT_POLL_MS = 30_000;

/**
 * Refresh of the summary (Outbound and Pools tabs only; paused in a background browser tab). The agent caches the
 * summary for 30 s, so polling faster would only re-read the same snapshot (review H1).
 */
export const NAT_SUMMARY_POLL_MS = 30_000;

async function fetchCandidateNat(signal?: AbortSignal): Promise<Record<string, unknown>> {
  const r = await call(
    api.GET('/api/v1/config/candidate/{path}', {
      params: { path: { path: 'nat' } },
      ...(signal ? { signal } : {}),
    }),
  );
  return (r.data ?? {}) as Record<string, unknown>;
}

/** `nat` of the candidate (equals running when nobody edits). */
export function useCandidateNat() {
  return useQuery({
    queryKey: qk.candidate('nat'),
    queryFn: ({ signal }) => fetchCandidateNat(signal),
  });
}

/** The candidate's `nat` right now (never diff against a snapshot up to one poll old). */
export function useFreshCandidateNat() {
  const qc = useQueryClient();
  return () =>
    qc.fetchQuery({
      queryKey: qk.candidate('nat'),
      queryFn: ({ signal }) => fetchCandidateNat(signal),
      staleTime: 0,
    });
}

/** A merge patch of the candidate's `nat` node through the generic P06 pointer route (arrays replace whole). */
export function usePatchNat() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (patch: Record<string, unknown>) =>
      call(api.PATCH('/api/v1/config/{path}', { params: { path: { path: 'nat' } }, body: patch })),
    onSettled: () =>
      Promise.all([invalidateConfig(qc), qc.invalidateQueries({ queryKey: natKeys.summary })]),
  });
}

export async function fetchSummary(signal?: AbortSignal) {
  return (await call(api.GET('/api/v1/state/nat/summary', signal ? { signal } : {}))).data;
}

/** NAT44-ED totals and per-pool utilisation from the agent, joined with the running pool names. */
export function useNatSummary() {
  return useQuery({
    queryKey: natKeys.summary,
    queryFn: ({ signal }) => fetchSummary(signal),
    refetchInterval: NAT_SUMMARY_POLL_MS,
    refetchIntervalInBackground: false,
  });
}

/** One server page of sessions: the grid's 0-based page becomes the API's 1-based `page`. */
export async function fetchSessions(
  page: number,
  pageSize: number,
  filter: SessionFilter,
  signal?: AbortSignal,
) {
  const query = { page: page + 1, pageSize, ...filterQuery(filter) };
  return (
    await call(
      api.GET('/api/v1/state/nat/sessions', { params: { query }, ...(signal ? { signal } : {}) }),
    )
  ).data;
}

/** Delete one session (`POST /actions/nat/sessions/kill`, audited by the API). */
export function useKillSession() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: KillBody) =>
      (await call(api.POST('/api/v1/actions/nat/sessions/kill', { body }))).data,
    onSettled: () =>
      Promise.all([
        qc.invalidateQueries({ queryKey: natKeys.sessions }),
        qc.invalidateQueries({ queryKey: natKeys.summary }),
      ]),
  });
}
