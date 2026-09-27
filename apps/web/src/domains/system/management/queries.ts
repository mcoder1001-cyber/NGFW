import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';

/** `management.tls` as the configuration holds it (packages/schema domains/management.ts). */
export interface TlsConfig {
  certificateRef?: string;
  privateKeyRef?: string;
  minVersion?: '1.2' | '1.3';
}

const PATH = 'management';
export const MGMT_TLS_POLL_MS = 5_000;
export const managementKeys = {
  running: ['config', 'running', PATH] as const,
  tlsState: ['state', 'management', 'tls'] as const,
};

type Doc = { tls?: TlsConfig } & Record<string, unknown>;

export function useCandidateTls() {
  return useQuery({
    queryKey: qk.candidate(PATH),
    queryFn: async ({ signal }) =>
      ((
        await call(
          api.GET('/api/v1/config/candidate/{path}', { params: { path: { path: PATH } }, signal }),
        )
      ).data ?? {}) as Doc,
    select: (d: Doc) => d.tls ?? {},
  });
}

export function useRunningTls() {
  return useQuery({
    queryKey: managementKeys.running,
    queryFn: async ({ signal }) =>
      ((await call(api.GET('/api/v1/config/{path}', { params: { path: { path: PATH } }, signal })))
        .data ?? {}) as Doc,
    select: (d: Doc) => d.tls ?? {},
    refetchInterval: MGMT_TLS_POLL_MS,
  });
}

/** The certificate the API has loaded (GET /api/v1/state/management/tls). Never carries the key. */
export function useTlsState() {
  return useQuery({
    queryKey: managementKeys.tlsState,
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/management/tls', { signal }))).data,
    refetchInterval: MGMT_TLS_POLL_MS,
  });
}

/**
 * Replaces `management.tls` in the candidate. Members the form dropped are sent as `null` (RFC 7396), so clearing
 * both references really removes them.
 */
export function usePatchTls() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (tls: TlsConfig) =>
      call(
        api.PATCH('/api/v1/config/{path}', {
          params: { path: { path: PATH } },
          body: {
            tls: {
              certificateRef: tls.certificateRef ?? null,
              privateKeyRef: tls.privateKeyRef ?? null,
              minVersion: tls.minVersion ?? '1.2',
            },
          },
        }),
      ),
    onSettled: () =>
      Promise.all([
        invalidateConfig(qc),
        qc.invalidateQueries({ queryKey: managementKeys.tlsState }),
      ]),
  });
}
