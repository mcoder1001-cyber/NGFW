import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';
import { srv6Of, type Srv6View } from './model';

export const srv6Keys = { state: ['state', 'srv6'] as const };

/** D-132: the state walks VPP (local SIDs with counters, policies, steering) — never faster than every 30 s. */
export const SRV6_STATE_POLL_MS = 30_000;

/** The state query: refetched every 30 s and on Refresh only — never on focus or remount within 30 s. */
export const srv6StateQuery = {
  queryKey: srv6Keys.state,
  queryFn: async ({ signal }: { signal: AbortSignal }) =>
    (await call(api.GET('/api/v1/state/srv6', { signal }))).data,
  refetchInterval: SRV6_STATE_POLL_MS,
  staleTime: SRV6_STATE_POLL_MS,
  refetchOnWindowFocus: false,
} as const;

export function useSrv6State() {
  return useQuery(srv6StateQuery);
}

async function fetchCandidate(path: string, signal?: AbortSignal): Promise<unknown> {
  const r = await call(
    api.GET('/api/v1/config/candidate/{path}', {
      params: { path: { path } },
      ...(signal ? { signal } : {}),
    }),
  );
  return r.data ?? {};
}

/** The candidate's `routing` node (raw, under the shared candidate key) → `routing.srv6`. */
export function useCandidateSrv6() {
  return useQuery({
    queryKey: qk.candidate('routing'),
    queryFn: ({ signal }) => fetchCandidate('routing', signal),
    select: (routing): Srv6View => srv6Of(routing),
  });
}

/** Names of the candidate's VRFs (`default` first) for the pickers. */
export function useCandidateVrfNames() {
  return useQuery({
    queryKey: qk.candidate('vrfs'),
    queryFn: ({ signal }) => fetchCandidate('vrfs', signal),
    select: (vrfs): string[] => [
      'default',
      ...Object.keys((vrfs ?? {}) as Record<string, unknown>)
        .filter((n) => n !== 'default')
        .sort(),
    ],
  });
}

/** Names of the candidate's interfaces and sub-interfaces for the pickers. */
export function useCandidateInterfaceNames() {
  return useQuery({
    queryKey: qk.candidate('interfaces'),
    queryFn: ({ signal }) => fetchCandidate('interfaces', signal),
    select: (ifs): string[] => {
      const out: string[] = [];
      for (const [name, v] of Object.entries((ifs ?? {}) as Record<string, unknown>)) {
        out.push(name);
        const subs = (v as { subinterfaces?: Record<string, unknown> } | null)?.subinterfaces ?? {};
        for (const id of Object.keys(subs)) out.push(`${name}.${id}`);
      }
      return out.sort();
    },
  });
}

/** `routing.srv6` of the candidate right now (never write back a snapshot). */
export function useFreshSrv6() {
  const qc = useQueryClient();
  return async (): Promise<Srv6View> =>
    srv6Of(
      await qc.fetchQuery({
        queryKey: qk.candidate('routing'),
        queryFn: ({ signal }) => fetchCandidate('routing', signal),
        staleTime: 0,
      }),
    );
}

/** A merge patch of `routing.srv6` through the generic pointer route (`PATCH /config/routing`). */
export function usePatchSrv6() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (srv6: Record<string, unknown>) =>
      call(
        api.PATCH('/api/v1/config/{path}', {
          params: { path: { path: 'routing' } },
          body: { srv6 },
        }),
      ),
    onSettled: () =>
      Promise.all([invalidateConfig(qc), qc.invalidateQueries({ queryKey: srv6Keys.state })]),
  });
}
