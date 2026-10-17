import type { AutoBlock } from '@ngfw/schema';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';

/** `security.autoBlock` as the configuration holds it (packages/schema domains/security.ts). */
export type AutoBlockConfig = AutoBlock;

/** The domain-level candidate route this tab reads and patches, like the Management tabs do for theirs. */
export const PATH = 'security';

type Doc = { autoBlock?: AutoBlockConfig } & Record<string, unknown>;

/** The candidate `security` document; the tab reads `autoBlock` from it (absent = schema defaults). */
export function useCandidateSecurity() {
  return useQuery({
    queryKey: qk.candidate(PATH),
    queryFn: async ({ signal }) =>
      ((await call(api.GET('/api/v1/config/candidate/{path}', { params: { path: { path: PATH } }, signal }))).data ?? {}) as Doc,
  });
}

/**
 * Replaces `security.autoBlock` in the candidate as a merge patch (RFC 7396): the arrays (rules, allow-list) are sent
 * whole, and a description the form dropped is sent as `null` so it is really removed. `invalidateConfig` refreshes
 * the pending-change bar and every candidate query, including this tab's own.
 */
export function usePatchAutoBlock() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (autoBlock: AutoBlockConfig) =>
      call(
        api.PATCH('/api/v1/config/{path}', {
          params: { path: { path: PATH } },
          body: { autoBlock: { ...autoBlock, description: autoBlock.description ?? null } },
        }),
      ),
    onSettled: () => invalidateConfig(qc),
  });
}
