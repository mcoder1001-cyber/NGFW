import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';

export interface CaptureFile {
  id: string;
  state: string;
  interface: string;
  direction: string;
  bpf: string;
  startedAt: string | null;
  stoppedAt: string | null;
  size: string;
  packets: string;
  sha256: string;
  maxPackets: number;
  seconds: number;
  snaplen: number;
  reason: string;
}
export interface CapturesState {
  captures: CaptureFile[];
  maxFiles: number;
  maxBytes: string;
  trace: { available: boolean; reason: string };
  pg: { available: boolean; reason: string };
}
export interface CaptureRequest {
  interface: string;
  direction: 'rx' | 'tx' | 'both';
  drop: boolean;
  errorFilter?: string;
  bpf: string;
  maxPackets: number;
  seconds: number;
  snaplen: number;
}

const KEY = ['state', 'captures'] as const;

/** Polls every 2 s while a capture runs (live progress), otherwise every 15 s. */
export function useCaptures() {
  return useQuery({
    queryKey: KEY,
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/captures', { signal }))).data as CapturesState,
    refetchInterval: (q) =>
      (q.state.data as CapturesState | undefined)?.captures.some((c) => c.state === 'running')
        ? 2000
        : 15000,
  });
}

export function useStartCapture() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: CaptureRequest) =>
      (await call(api.POST('/api/v1/actions/capture', { body }))).data as { id: string },
    onSettled: () => qc.invalidateQueries({ queryKey: KEY }),
  });
}

export function useStopCapture() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) =>
      call(api.POST('/api/v1/actions/capture/{id}/stop', { params: { path: { id } } })),
    onSettled: () => qc.invalidateQueries({ queryKey: KEY }),
  });
}

export function useDeleteCapture() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) =>
      call(api.DELETE('/api/v1/state/captures/{id}', { params: { path: { id } } })),
    onSettled: () => qc.invalidateQueries({ queryKey: KEY }),
  });
}

/** Authenticated download → blob → object URL (admin only; the API audits it). */
export async function downloadCapture(id: string): Promise<void> {
  const r = await call(
    api.GET('/api/v1/state/captures/{id}/file', { params: { path: { id } }, parseAs: 'blob' }),
  );
  const url = URL.createObjectURL(r.data as unknown as Blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `${id}.pcap`;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 0);
}
