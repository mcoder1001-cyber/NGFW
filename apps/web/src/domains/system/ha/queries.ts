import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';

/** `ha` as the candidate / running configuration holds it (packages/schema domains/ha.ts). */
export interface VrrpRouter {
  enabled?: boolean;
  description?: string;
  interface: string;
  vrId: number;
  addressFamily?: 'ipv4' | 'ipv6';
  priority?: number;
  advertisementIntervalMs?: number;
  preempt?: boolean;
  acceptMode?: boolean;
  unicast?: { peers: string[] };
  addresses: string[];
  vrf?: string;
  engine?: 'vpp' | 'keepalived';
  track?: { interface: string; priorityDecrement?: number }[];
}
export interface HaCluster {
  enabled?: boolean;
  nodeName: string;
  peers: { name: string; address: string }[];
  port?: number;
  secretRef: string;
  interface?: string;
  vrf?: string;
  configSync?: boolean;
  stateSync?: { nat?: boolean; ipsec?: boolean; acl?: boolean };
}
export interface HaConfig {
  vrrp?: Record<string, VrrpRouter>;
  cluster?: HaCluster;
}

const PATH = 'ha';
export const haKeys = { running: ['config', 'running', PATH] as const };

export function useCandidateHa() {
  return useQuery({
    queryKey: qk.candidate(PATH),
    queryFn: async ({ signal }) =>
      ((
        await call(
          api.GET('/api/v1/config/candidate/{path}', { params: { path: { path: PATH } }, signal }),
        )
      ).data ?? {}) as HaConfig,
  });
}

export function useRunningHa() {
  return useQuery({
    queryKey: haKeys.running,
    queryFn: async ({ signal }) =>
      ((await call(api.GET('/api/v1/config/{path}', { params: { path: { path: PATH } }, signal })))
        .data ?? {}) as HaConfig,
  });
}

/** A merge patch of the candidate's `ha`; the pending-change bar commits it. */
export function usePatchHa() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (patch: Record<string, unknown>) =>
      call(api.PATCH('/api/v1/config/{path}', { params: { path: { path: PATH } }, body: patch })),
    onSettled: () => invalidateConfig(qc),
  });
}

/** RFC 7396 patch for the `vrrp` record: new/changed entries as-is, removed names as null. */
export function vrrpPatch(
  before: Record<string, unknown>,
  after: Record<string, unknown>,
): Record<string, unknown> {
  const out: Record<string, unknown> = { ...after };
  for (const k of Object.keys(before)) if (!(k in after)) out[k] = null;
  return out;
}

export function useVrrpRuntime() {
  return useQuery({
    queryKey: ['ha', 'vrrp', 'live'],
    queryFn: async ({ signal }) => (await call(api.GET('/api/v1/state/ha/vrrp', { signal }))).data,
    refetchInterval: 5000,
  });
}
export function useClusterRuntime() {
  return useQuery({
    queryKey: ['ha', 'cluster', 'live'],
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/ha/cluster', { signal }))).data,
    refetchInterval: 5000,
  });
}
export function useForceClusterSync() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => call(api.POST('/api/v1/actions/ha/sync')),
    onSettled: () => qc.invalidateQueries({ queryKey: ['ha', 'cluster', 'live'] }),
  });
}
