import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';

/** `services.snmp` as the UI edits it (refs only: community strings and passphrases are secrets, D-051). */
export interface SnmpCommunity {
  secretRef: string;
  access?: 'ro' | 'rw';
  sources?: string[];
  view?: string;
}
export interface SnmpV3User {
  securityLevel?: 'noAuthNoPriv' | 'authNoPriv' | 'authPriv';
  authProtocol?: 'sha' | 'sha256' | 'sha512' | 'md5';
  authRef?: string;
  privProtocol?: 'aes' | 'aes256' | 'des';
  privRef?: string;
  access?: 'ro' | 'rw';
  view?: string;
}
export interface SnmpTrapReceiver {
  address: string;
  port?: number;
  version?: 'v2c' | 'v3';
  community?: string;
  user?: string;
  inform?: boolean;
}
export interface SnmpConfig {
  enabled?: boolean;
  sysName?: string;
  sysLocation?: string;
  sysContact?: string;
  engineId?: string;
  communities?: Record<string, SnmpCommunity>;
  v3Users?: Record<string, SnmpV3User>;
  trapReceivers?: SnmpTrapReceiver[];
  subagent?: { enabled?: boolean };
}

const PATH = 'services/snmp';
export const snmpKeys = { state: ['state', 'snmp'] as const };

export function useCandidateSnmp() {
  return useQuery({
    queryKey: qk.candidate(PATH),
    queryFn: async ({ signal }) =>
      ((
        await call(
          api.GET('/api/v1/config/candidate/{path}', { params: { path: { path: PATH } }, signal }),
        )
      ).data ?? {}) as SnmpConfig,
  });
}

export function useSnmpState() {
  return useQuery({
    queryKey: snmpKeys.state,
    queryFn: async ({ signal }) => (await call(api.GET('/api/v1/state/snmp', { signal }))).data,
    refetchInterval: 5_000,
  });
}

/** A merge patch of the candidate's services.snmp (the generic pointer route). */
export function usePatchSnmp() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (patch: Record<string, unknown>) =>
      call(api.PATCH('/api/v1/config/{path}', { params: { path: { path: PATH } }, body: patch })),
    onSettled: () =>
      Promise.all([invalidateConfig(qc), qc.invalidateQueries({ queryKey: snmpKeys.state })]),
  });
}

/** Writes a secret (admin only) and returns its reference; the value never comes back. */
export async function putSecret(name: string, value: string): Promise<string> {
  const r = await call(
    api.POST('/api/v1/secrets', {
      params: { query: { replace: 'true' } },
      body: { kind: 'password', name, value },
    }),
  );
  return r.data.ref;
}
