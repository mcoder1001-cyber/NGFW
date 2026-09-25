import { useCallback } from 'react';
import { emptyMpls, useRoutingCandidate, useWriteMpls } from './api';
import type { MplsConfig } from './model';

/**
 * The candidate's `routing.mpls` plus a writer: `update(fn)` applies `fn` to a copy of the current section (or an empty
 * one with the schema's defaults) and writes the result back as one merge patch of `routing`.
 */
export function useMpls() {
  const routing = useRoutingCandidate();
  const write = useWriteMpls();
  const current = routing.data?.mpls;
  const update = useCallback(
    async (fn: (m: MplsConfig) => MplsConfig) => {
      const base = structuredClone(current ?? emptyMpls());
      await write.mutateAsync({ from: current, to: fn(base) });
    },
    [current, write],
  );
  return { routing, mpls: current, update, write };
}
