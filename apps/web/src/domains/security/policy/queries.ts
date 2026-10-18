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

/** What the tab saves: the form's value plus the value it was loaded from (to tell "cleared" from "never there"). */
export interface SaveAutoBlock {
  autoBlock: AutoBlockConfig;
  loaded: AutoBlockConfig | undefined;
}

/**
 * Replaces `security.autoBlock` in the candidate as a merge patch (RFC 7396): the arrays (rules, allow-list) are sent
 * whole. `description` is the only optional key of the node (every other key has a schema default and is always in
 * the form's value); when the loaded value had one and the form dropped it, it is sent as `null` so it is really
 * removed — when it was never there the key is left out, so a concurrent edit by another session is not wiped
 * (R2). `invalidateConfig` refreshes the pending-change bar and every candidate query, including this tab's own.
 */
export function usePatchAutoBlock() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ autoBlock, loaded }: SaveAutoBlock) => {
      const { description, ...rest } = autoBlock;
      const cleared = description === undefined && loaded?.description !== undefined;
      const body = { autoBlock: { ...rest, ...(description !== undefined ? { description } : cleared ? { description: null } : {}) } };
      return call(api.PATCH('/api/v1/config/{path}', { params: { path: { path: PATH } }, body }));
    },
    onSettled: () => invalidateConfig(qc),
  });
}
