import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';

/** D-132: views that make the agent walk VPP session tables refresh at most every 30 s. */
export const POLL_MS = 30_000;

export const keys = {
  det44Sessions: (user: string) => ['state', 'nat', 'det44', 'sessions', user] as const,
  cnatSessions: (page: number, pageSize: number) =>
    ['state', 'nat', 'cnat', 'sessions', page, pageSize] as const,
};

/** One DET44 user's sessions (first page) + the user's outside address and port block. */
export function useDet44Sessions(user: string) {
  return useQuery({
    queryKey: keys.det44Sessions(user),
    enabled: user !== '',
    queryFn: async ({ signal }) =>
      (
        await call(
          api.GET('/api/v1/state/nat/det44/sessions', {
            params: { query: { user, pageSize: 100 } },
            signal,
          }),
        )
      ).data,
  });
}

/** DET44 lookup (forward or reverse), read-only. */
export function useDet44Lookup() {
  return useMutation({
    mutationFn: async (body: { inside: string } | { outside: string; port: number }) =>
      (await call(api.POST('/api/v1/actions/nat/det44/lookup', { body }))).data,
  });
}

/** One page of the CNAT session table (0-based page in the UI). */
export function useCnatSessions(page: number, pageSize: number) {
  return useQuery({
    queryKey: keys.cnatSessions(page, pageSize),
    queryFn: async ({ signal }) =>
      (
        await call(
          api.GET('/api/v1/state/nat/cnat/sessions', {
            params: { query: { page: page + 1, pageSize } },
            signal,
          }),
        )
      ).data,
    refetchInterval: POLL_MS,
    refetchIntervalInBackground: false,
  });
}

/** Purge the CNAT session table (admin; the agent refuses unless it is the globals owner). */
export function useCnatPurge() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => (await call(api.POST('/api/v1/actions/nat/cnat/sessions/purge'))).data,
    onSettled: () => qc.invalidateQueries({ queryKey: ['state', 'nat', 'cnat', 'sessions'] }),
  });
}
