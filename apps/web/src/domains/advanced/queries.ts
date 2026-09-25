import type { RootKey } from '@ngfw/schema';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../api';
import { call } from '../../api-problem';
import { invalidateConfig, qk } from '../../config/queries';

/**
 * The candidate value of a whole schema domain, through the generic pointer route scoped to the single top-level
 * segment (`GET /api/v1/config/candidate/{path}`, `path` = the domain key). A deeper JSON pointer cannot be one URL
 * path segment (`openapi-fetch` percent-encodes a `/` inside a single param value, so a multi-segment pointer would
 * arrive as one opaque segment and fail the API's `ROOT_KEYS` check, `apps/api/src/config/path.ts`); the advanced
 * editor reads/writes the whole domain and walks/patches the JSON pointer subtree client-side (`schemaPath.ts`) —
 * exactly what `domains/interfaces/queries.ts` already does for `interfaces` (see its `createMergePatch` comment).
 * No `refetchInterval` (D-132: no timer under 30 s on anything that reaches the API/agent) — the page has a Refresh
 * button instead.
 */
export function useDomainCandidate(domainKey: RootKey, enabled = true) {
  return useQuery({
    queryKey: qk.candidate(domainKey),
    queryFn: async ({ signal }) => (await call(api.GET('/api/v1/config/candidate/{path}', { params: { path: { path: domainKey } }, signal }))).data,
    enabled,
  });
}

/** A merge patch of the candidate's whole domain node — the generic P06 pointer route, scoped to one safe segment. */
export function usePatchDomain(domainKey: RootKey) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (patch: unknown) => call(api.PATCH('/api/v1/config/{path}', { params: { path: { path: domainKey } }, body: patch })),
    onSettled: () => Promise.all([invalidateConfig(qc), qc.invalidateQueries({ queryKey: qk.candidate(domainKey) })]),
  });
}
