import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../api';
import { call } from '../../api-problem';
import { invalidateConfig, qk } from '../../config/queries';
import type { DataplaneConfig } from '@ngfw/schema';
import type { InterfacesConfig } from './model';

export const ifaceKeys = {
  state: ['state', 'interfaces'] as const,
};

/** Live table refresh; counters/rates come from the WS topic in between. */
export const STATE_POLL_MS = 3_000;

export async function fetchInterfacesState(signal?: AbortSignal) {
  return (await call(api.GET('/api/v1/state/interfaces', signal ? { signal } : {}))).data;
}

export function useInterfacesState() {
  return useQuery({
    queryKey: ifaceKeys.state,
    queryFn: ({ signal }) => fetchInterfacesState(signal),
    refetchInterval: STATE_POLL_MS,
  });
}

async function fetchCandidateInterfaces(signal?: AbortSignal): Promise<InterfacesConfig> {
  const r = await call(
    api.GET('/api/v1/config/candidate/{path}', {
      params: { path: { path: 'interfaces' } },
      ...(signal ? { signal } : {}),
    }),
  );
  return (r.data ?? {}) as InterfacesConfig;
}

/** `interfaces` of the candidate (equals running when nobody edits). */
export function useCandidateInterfaces() {
  return useQuery({
    queryKey: qk.candidate('interfaces'),
    queryFn: ({ signal }) => fetchCandidateInterfaces(signal),
  });
}

/** A merge patch of the candidate's `interfaces` node (the generic P06 pointer route, nothing interface-specific). */
export function usePatchInterfaces() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (patch: Record<string, unknown>) =>
      call(
        api.PATCH('/api/v1/config/{path}', {
          params: { path: { path: 'interfaces' } },
          body: patch,
        }),
      ),
    onSettled: () =>
      Promise.all([invalidateConfig(qc), qc.invalidateQueries({ queryKey: ifaceKeys.state })]),
  });
}

/** The candidate's interfaces right now (never write back a snapshot up to one poll old). */
export function useFreshCandidate() {
  const qc = useQueryClient();
  return () =>
    qc.fetchQuery({
      queryKey: qk.candidate('interfaces'),
      queryFn: ({ signal }) => fetchCandidateInterfaces(signal),
      staleTime: 0,
    });
}

type DataplaneCandidate = Partial<Pick<DataplaneConfig, 'pciWhitelist' | 'devices'>>;

async function fetchCandidateDataplane(signal?: AbortSignal): Promise<DataplaneCandidate> {
  const r = await call(
    api.GET('/api/v1/config/candidate/{path}', {
      params: { path: { path: 'dataplane' } },
      ...(signal ? { signal } : {}),
    }),
  );
  return (r.data ?? {}) as DataplaneCandidate;
}

/**
 * F-default-vpp-nics: release a physical NIC to the host (`owner: 'host'`) or reclaim it for the dataplane
 * (`owner: 'dataplane'`) in one root merge patch that also keeps `dataplane.pciWhitelist`/`devices` consistent
 * (the `dataplane.owner-consistent` rule). It applies with the next dataplane apply.
 */
export function useSetPhysicalOwner() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({
      name,
      pci,
      owner,
    }: {
      name: string;
      pci: string;
      owner: 'host' | 'dataplane';
    }) => {
      const dp = await qc.fetchQuery({
        queryKey: qk.candidate('dataplane'),
        queryFn: ({ signal }) => fetchCandidateDataplane(signal),
        staleTime: 0,
      });
      const current = dp.pciWhitelist ?? [];
      const low = pci.toLowerCase();
      const pciWhitelist =
        owner === 'host'
          ? current.filter((p) => p.toLowerCase() !== low)
          : [...current.filter((p) => p.toLowerCase() !== low), pci];
      // Existing device keys may use different PCI casing: remove every matching spelling before reclaiming.
      const devices: Record<string, null | { name: string }> = Object.fromEntries(
        Object.keys(dp.devices ?? {})
          .filter((key) => key.toLowerCase() === low)
          .map((key) => [key, null]),
      );
      if (owner === 'dataplane') devices[pci] = { name };
      const body = {
        interfaces: { [name]: { physical: { owner } } },
        dataplane: { pciWhitelist, devices },
      };
      // the root merge-patch route carries an unschematised body (Record<string, never> in the client)
      return call(api.PATCH('/api/v1/config', { body: body as unknown as Record<string, never> }));
    },
    onSettled: () =>
      Promise.all([invalidateConfig(qc), qc.invalidateQueries({ queryKey: ifaceKeys.state })]),
  });
}

/** F-pppoe-client: redial the PPPoE client on `name` now; the live table refreshes afterwards. */
export function usePppoeReconnect() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (name: string) =>
      (
        await call(
          api.POST('/api/v1/actions/interfaces/{name}/pppoe/reconnect', {
            params: { path: { name } },
          }),
        )
      ).data,
    onSettled: () => qc.invalidateQueries({ queryKey: ifaceKeys.state }),
  });
}
