import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';
import type { Profile } from './model';
export const raKeys = {
  capability: ['state', 'ra-vpn', 'capabilities'] as const,
  sessions: ['state', 'ra-vpn', 'sessions'] as const,
};
export function useCapabilities() {
  return useQuery({
    queryKey: raKeys.capability,
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/vpn/remote-access/capabilities', { signal }))).data,
    staleTime: 30_000,
    refetchInterval: 30_000,
    refetchOnWindowFocus: false,
  });
}
export function useProfiles() {
  return useQuery({
    queryKey: qk.candidate('vpn'),
    queryFn: async ({ signal }) => {
      const result = await call(
        api.GET('/api/v1/config/candidate/{path}', { params: { path: { path: 'vpn' } }, signal }),
      );
      return ((result.data ?? {}) as { remoteAccess?: Record<string, Profile> }).remoteAccess ?? {};
    },
  });
}
export function useSessions(profile: string, cursor: string, enabled: boolean) {
  return useQuery({
    queryKey: [...raKeys.sessions, profile, cursor],
    enabled: enabled && !!profile,
    queryFn: async ({ signal }) =>
      (
        await call(
          api.GET('/api/v1/state/vpn/remote-access/sessions', {
            params: { query: { profile, cursor, limit: 50 } },
            signal,
          }),
        )
      ).data,
    staleTime: 30_000,
    refetchInterval: 30_000,
    refetchOnWindowFocus: false,
  });
}
export function useSaveProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ name, profile }: { name: string; profile: Partial<Profile> }) =>
      call(
        api.PATCH('/api/v1/config/{path}', {
          params: { path: { path: 'vpn' } },
          body: { remoteAccess: { [name]: profile } },
        }),
      ),
    onSettled: () => invalidateConfig(qc),
  });
}
export function useDisconnect() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ profile, id }: { profile: string; id: string }) =>
      (
        await call(
          api.POST('/api/v1/actions/vpn/remote-access/sessions/{id}/disconnect', {
            params: { path: { id }, query: { profile } },
          }),
        )
      ).data,
    onSettled: () => qc.invalidateQueries({ queryKey: raKeys.sessions }),
  });
}
