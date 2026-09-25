import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';

export interface HostStackRule {
  tag: string;
  scope?: 'global' | 'local';
  transport: 'tcp' | 'udp';
  local: string;
  localPort?: number;
  remote: string;
  remotePort?: number;
  action: 'allow' | 'deny' | 'redirect';
  redirectAppIndex?: number;
  appNamespace?: string;
}
export interface HostStackNamespace {
  vrf?: string;
  interface?: string;
  secretRef?: string;
}
export interface HostStackConfig {
  enabled?: boolean;
  namespaces?: Record<string, HostStackNamespace>;
  sessionRules?: HostStackRule[];
}

export const hostStackKeys = { state: ['state', 'host-stack'] as const };

export function useHostStackState() {
  return useQuery({
    queryKey: hostStackKeys.state,
    queryFn: async ({ signal }) => (await call(api.GET('/api/v1/state/host-stack', { signal }))).data,
    refetchInterval: 5_000,
  });
}

/** `services.hostStack` of the candidate (undefined when unset). */
export function useCandidateHostStack() {
  return useQuery({
    queryKey: qk.candidate('services'),
    queryFn: async ({ signal }) => {
      const r = await call(api.GET('/api/v1/config/candidate/{path}', { params: { path: { path: 'services' } }, signal }));
      return r.data as { hostStack?: HostStackConfig } | undefined;
    },
    select: (s) => s?.hostStack,
  });
}

/** Replaces `services.hostStack` in the candidate (merge patch of `services`; arrays and records replace). */
export function usePutHostStack() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (hs: HostStackConfig) =>
      call(api.PATCH('/api/v1/config/{path}', { params: { path: { path: 'services' } }, body: { hostStack: hs } })),
    onSettled: () => Promise.all([invalidateConfig(qc), qc.invalidateQueries({ queryKey: hostStackKeys.state })]),
  });
}
