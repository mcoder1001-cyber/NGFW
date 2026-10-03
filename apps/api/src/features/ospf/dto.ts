import { z } from 'zod';

export const OSPF_ROW_LIMIT = 100;
export const OspfNeighborOut = z.strictObject({
  vrf: z.string().min(1).max(96),
  routerId: z.string().max(96),
  address: z.string().max(96).nullable(),
  interface: z.string().max(96).nullable(),
  state: z.string().max(32).describe('Observed FRR adjacency state, or Unknown'),
  priority: z.int().min(0).max(255).nullable(),
});
export const OspfStateOut = z.strictObject({
  frrRunning: z.boolean(),
  retrievedAt: z.iso.datetime().nullable(),
  unavailable: z
    .enum(['frr-unavailable', 'reader-unavailable', 'reader-invalid', 'reader-limit-exceeded'])
    .nullable(),
  warning: z.literal('routing-observation-partial').nullable(),
  truncated: z.boolean(),
  neighbors: z.array(OspfNeighborOut).max(OSPF_ROW_LIMIT),
});
export type OspfState = z.infer<typeof OspfStateOut>;
export type OspfNeighbor = z.infer<typeof OspfNeighborOut>;
