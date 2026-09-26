import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';
import type { WgInterfacesConfig } from './model';

export const wgKeys = { state: ['state', 'vpn', 'wireguard'] as const };

/** D-132: the state walks VPP — never faster than every 30 s; live peer status comes from `wireguard.events`. */
export const WG_STATE_POLL_MS = 30_000;

/**
 * The state query (review F9, D-132): refetched every 30 s and on Refresh only — never on focus or remount within 30 s
 * (the app-wide staleTime is 5 s).
 */
export const wireguardStateQuery = {
  queryKey: wgKeys.state,
  queryFn: async ({ signal }: { signal: AbortSignal }) =>
    (await call(api.GET('/api/v1/state/vpn/wireguard', { signal }))).data,
  refetchInterval: WG_STATE_POLL_MS,
  staleTime: WG_STATE_POLL_MS,
  refetchOnWindowFocus: false,
} as const;

export function useWireguardState() {
  return useQuery(wireguardStateQuery);
}

async function fetchCandidate(signal?: AbortSignal): Promise<WgInterfacesConfig> {
  const r = await call(
    api.GET('/api/v1/config/candidate/{path}', {
      params: { path: { path: 'vpn' } },
      ...(signal ? { signal } : {}),
    }),
  );
  const vpn = (r.data ?? {}) as { wireguard?: { interfaces?: WgInterfacesConfig } };
  return vpn.wireguard?.interfaces ?? {};
}

/** `vpn.wireguard.interfaces` of the candidate. */
export function useCandidateWireguard() {
  return useQuery({
    queryKey: qk.candidate('vpn'),
    queryFn: ({ signal }) => fetchCandidate(signal),
  });
}

/** The candidate right now (never write back a snapshot). */
export function useFreshWireguard() {
  const qc = useQueryClient();
  return () =>
    qc.fetchQuery({
      queryKey: qk.candidate('vpn'),
      queryFn: ({ signal }) => fetchCandidate(signal),
      staleTime: 0,
    });
}

/** A merge patch of `vpn.wireguard.interfaces` through the generic pointer route (`PATCH /config/vpn`). */
export function usePatchWireguard() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (interfaces: Record<string, unknown>) =>
      call(
        api.PATCH('/api/v1/config/{path}', {
          params: { path: { path: 'vpn' } },
          body: { wireguard: { interfaces } },
        }),
      ),
    onSettled: () =>
      Promise.all([invalidateConfig(qc), qc.invalidateQueries({ queryKey: wgKeys.state })]),
  });
}

/** Server-side key pair: the private key is stored as `key/<name>`; only the reference and the public key come back. */
export function useKeypair() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (name: string) =>
      (await call(api.POST('/api/v1/actions/vpn/wireguard/keypair', { body: { name } }))).data,
    onSettled: () => qc.invalidateQueries({ queryKey: ['secrets'] }),
  });
}
