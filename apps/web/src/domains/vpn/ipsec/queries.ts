import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';

export const ipsecKeys = {
  tunnels: ['state', 'ipsec', 'tunnels'] as const,
  sas: ['state', 'ipsec', 'sas'] as const,
};

/** The state walks charon over VICI; refresh on demand and every 15 s; `ipsec.events` invalidates earlier. */
export const IPSEC_STATE_POLL_MS = 15_000;

export function useIpsecTunnels() {
  return useQuery({
    queryKey: ipsecKeys.tunnels,
    queryFn: async ({ signal }: { signal: AbortSignal }) =>
      (await call(api.GET('/api/v1/state/ipsec/tunnels', { signal }))).data,
    refetchInterval: IPSEC_STATE_POLL_MS,
    staleTime: IPSEC_STATE_POLL_MS,
    refetchOnWindowFocus: false,
  });
}

/**
 * The IKE_SAs and CHILD_SAs of one tunnel (SA inspection drawer); loaded on demand when a drawer opens. The API
 * filters by tunnel, so a tunnel's SAs are never cut off by the page limit of an unfiltered walk (verify r1 #6).
 */
export function useIpsecSas(tunnel: string, enabled = true) {
  return useQuery({
    queryKey: [...ipsecKeys.sas, tunnel],
    queryFn: async ({ signal }: { signal: AbortSignal }) =>
      (
        await call(
          api.GET('/api/v1/state/ipsec/sas', { params: { query: { tunnel } }, signal }),
        )
      ).data,
    refetchInterval: IPSEC_STATE_POLL_MS,
    staleTime: IPSEC_STATE_POLL_MS,
    refetchOnWindowFocus: false,
    enabled,
  });
}

type IpsecCandidate = {
  proposals?: Record<string, unknown>;
  tunnels?: Record<string, unknown>;
};

async function fetchCandidate(signal?: AbortSignal): Promise<IpsecCandidate> {
  const r = await call(
    api.GET('/api/v1/config/candidate/{path}', {
      params: { path: { path: 'vpn' } },
      ...(signal ? { signal } : {}),
    }),
  );
  const vpn = (r.data ?? {}) as { ipsec?: IpsecCandidate };
  return vpn.ipsec ?? {};
}

/**
 * The candidate query key of this tab. It must differ from WireGuard's `qk.candidate('vpn')`: both read the vpn
 * subtree but cache different shapes (P11 review R6 B1). It stays under ['config', …] so invalidateConfig reaches it.
 */
export const ipsecCandidateKey = qk.candidate('vpn/ipsec');

/** `vpn.ipsec` (proposals + tunnels) of the candidate. */
export function useCandidateIpsec() {
  return useQuery({
    queryKey: ipsecCandidateKey,
    queryFn: ({ signal }) => fetchCandidate(signal),
  });
}

/** The candidate right now (never write back a snapshot). */
export function useFreshIpsec() {
  const qc = useQueryClient();
  return () =>
    qc.fetchQuery({
      queryKey: ipsecCandidateKey,
      queryFn: ({ signal }) => fetchCandidate(signal),
      staleTime: 0,
    });
}

/** A merge patch of `vpn.ipsec` through the generic pointer route (`PATCH /config/vpn`). */
export function usePatchIpsec() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (ipsec: Record<string, unknown>) =>
      call(
        api.PATCH('/api/v1/config/{path}', {
          params: { path: { path: 'vpn' } },
          body: { ipsec },
        }),
      ),
    onSettled: () =>
      Promise.all([
        invalidateConfig(qc),
        qc.invalidateQueries({ queryKey: ipsecCandidateKey }),
        qc.invalidateQueries({ queryKey: ipsecKeys.tunnels }),
      ]),
  });
}
