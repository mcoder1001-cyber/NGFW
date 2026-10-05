import { z } from 'zod';
import type { HaSyncStateResponse } from '@ngfw/proto';
export const syncState = z.object({
  owner: z.string(),
  kinds: z.array(
    z.object({
      kind: z.string(),
      supported: z.boolean(),
      configured: z.boolean(),
      active: z.boolean(),
      reason: z.string(),
    }),
  ),
  listener: z
    .object({
      address: z.string().optional(),
      port: z.number().optional(),
      pathMtu: z.number().optional(),
    })
    .nullable(),
  failover: z
    .object({
      address: z.string().optional(),
      port: z.number().optional(),
      sessionRefreshSec: z.number().optional(),
    })
    .nullable(),
  lastResync: z.string().nullable(),
  lastMissedCount: z.number().nullable(),
  resyncCount: z.string().regex(/^\d+$/),
  packetCountersAvailable: z.boolean(),
  actionsAllowed: z.boolean(),
  retrievedAt: z.string(),
  observationError: z.string(),
});
export const syncResult = z.object({ summary: z.string() });
export function toState(s: HaSyncStateResponse): z.output<typeof syncState> {
  return {
    owner: s.owner,
    kinds: s.kinds,
    listener: s.listener ?? null,
    failover: s.failover ?? null,
    lastResync: s.lastResync?.toISOString() ?? null,
    lastMissedCount: s.lastMissedCount ?? null,
    resyncCount: s.resyncCount,
    packetCountersAvailable: s.packetCountersAvailable,
    actionsAllowed: s.actionsAllowed,
    retrievedAt: s.retrievedAt?.toISOString() ?? '',
    observationError: s.observationError,
  };
}
