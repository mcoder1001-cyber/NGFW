import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';

/** `services.ipfix` of the configuration document (packages/schema IpfixSchema; defaults filled by the API). */
export interface ExporterConfig {
  enabled?: boolean;
  description?: string;
  collector: { address: string; port?: number };
  sourceAddress: string;
  vrf?: string;
  pathMtu?: number;
  templateIntervalSec?: number;
  udpChecksum?: boolean;
}
export interface FlowprobeInterfaceConfig {
  interface: string;
  direction?: 'rx' | 'tx' | 'both';
  l2?: boolean;
  ip4?: boolean;
  ip6?: boolean;
}
export interface FlowprobeConfig {
  activeTimerSec?: number;
  passiveTimerSec?: number;
  recordL2?: boolean;
  recordL3?: boolean;
  recordL4?: boolean;
  interfaces?: FlowprobeInterfaceConfig[];
}
export interface SflowConfig {
  enabled?: boolean;
  samplingN?: number;
  pollingIntervalSec?: number;
  headerBytes?: number;
  collectors: { address: string; port?: number }[];
  agentAddress?: string;
  vrf?: string;
  interfaces?: string[];
}
export interface IpfixConfig {
  exporters?: Record<string, ExporterConfig>;
  flowprobe?: FlowprobeConfig;
  sflow?: SflowConfig;
}

export const ipfixKeys = { state: ['state', 'ipfix'] as const };
export const IPFIX_POLL_MS = 5_000;

export async function fetchIpfixState(signal?: AbortSignal) {
  return (await call(api.GET('/api/v1/state/ipfix', signal ? { signal } : {}))).data;
}
export type IpfixState = Awaited<ReturnType<typeof fetchIpfixState>>;

/** Live flow-export state (agent IpfixState). */
export function useIpfixState() {
  return useQuery({
    queryKey: ipfixKeys.state,
    queryFn: ({ signal }) => fetchIpfixState(signal),
    refetchInterval: IPFIX_POLL_MS,
    retry: false,
  });
}

/** `services.ipfix` of the candidate (the `services` node: a `/` inside one URL segment would be pointer-escaped). */
export function useCandidateIpfix() {
  return useQuery({
    queryKey: qk.candidate('services'),
    queryFn: async ({ signal }) => {
      const r = await call(
        api.GET('/api/v1/config/candidate/{path}', {
          params: { path: { path: 'services' } },
          signal,
        }),
      );
      return ((r.data ?? {}) as { ipfix?: IpfixConfig }).ipfix ?? {};
    },
  });
}

/** Candidate interface names for the interface picker (logical names, D-069). */
export function useInterfaceNames() {
  return useQuery({
    queryKey: qk.candidate('interfaces'),
    queryFn: async ({ signal }) => {
      const r = await call(
        api.GET('/api/v1/config/candidate/{path}', {
          params: { path: { path: 'interfaces' } },
          signal,
        }),
      );
      return r.data ?? {};
    },
    select: (d) => Object.keys(d as Record<string, unknown>).sort(),
  });
}

/** A merge patch of `services.ipfix` (generic pointer route; arrays are replaced, `null` removes a map entry). */
export function usePatchIpfix() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (patch: Record<string, unknown>) =>
      call(
        api.PATCH('/api/v1/config/{path}', {
          params: { path: { path: 'services' } },
          body: { ipfix: patch },
        }),
      ),
    onSettled: () =>
      Promise.all([invalidateConfig(qc), qc.invalidateQueries({ queryKey: ipfixKeys.state })]),
  });
}
