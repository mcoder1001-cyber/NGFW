import { configUrl } from '@ngfw/api-client';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../api';
import { ApiError, call } from '../../api-problem';
import { invalidateConfig } from '../../config/queries';

/** Query keys for a config pointer's candidate value — distinct from `config/queries.ts`'s `qk.candidate`, which is
 * keyed by a bare domain name for that file's single-segment call sites. */
const POINTER_ROOT = ['config', 'candidateAt'] as const;
const qk = {
  pointer: (pointer: string) => [...POINTER_ROOT, pointer] as const,
};

// The generic route's own path template, as the generated schema declares it. `configUrl` (D-UDE-1: the generated
// client's path serializer percent-encodes a `/` between JSON-pointer segments, which the server's wildcard route
// does not expect) builds the real, already-resolved URL and is passed as this literal argument instead of the
// `{path}` placeholder — nothing remains for the serializer to substitute, so the `path` param below is a required
// but inert placeholder (openapi-fetch only touches params for the `{...}` it finds in the *template string*, and
// this argument no longer has one).
type CandidateAtPath = '/api/v1/config/candidate/{path}';
type ConfigAtPath = '/api/v1/config/{path}';
const NO_PATH_PARAM = { path: { path: '' } } as const;

/**
 * The candidate value at any JSON pointer, through the generic pointer route at its real depth (D-UDE-1). A
 * pointer the candidate does not have yet (e.g. an interface name not created) answers 404, treated here as `null`
 * rather than an error — the same "not there yet" a whole-domain read used to give for free (`null`, not
 * `undefined`: a query function may never resolve `undefined`, TanStack Query reserves it for "no data yet"; every
 * node this hook ever reads is an object/record, so `null` cannot collide with a real value). No `refetchInterval`
 * (D-132: no timer under 30 s on anything that reaches the API/agent) — the page has a Refresh button instead.
 */
export function useConfigNode(pointer: string, enabled = true) {
  return useQuery({
    queryKey: qk.pointer(pointer),
    queryFn: async ({ signal }): Promise<unknown> => {
      try {
        return (await call(api.GET(configUrl('/api/v1/config/candidate', pointer) as CandidateAtPath, { params: NO_PATH_PARAM, signal }))).data;
      } catch (e) {
        if (e instanceof ApiError && e.status === 404) return null;
        throw e;
      }
    },
    enabled,
  });
}

/** A merge patch of the candidate node at any JSON pointer — the generic P06 pointer route, at its real depth. */
export function usePatchConfigNode() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ pointer, body }: { pointer: string; body: unknown }) =>
      call(api.PATCH(configUrl('/api/v1/config', pointer) as ConfigAtPath, { params: NO_PATH_PARAM, body })),
    onSettled: () => Promise.all([invalidateConfig(qc), qc.invalidateQueries({ queryKey: POINTER_ROOT })]),
  });
}
