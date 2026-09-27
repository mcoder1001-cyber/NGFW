import { z } from 'zod';

/**
 * Request/response DTOs of the DET44 / CNAT state and actions (F-det44-map-dslite-cnat). One Zod definition per shape
 * serves validation (ZodPipe → 400 problem+json with pointers) and OpenAPI.
 */

const ipv4 = z.ipv4();
const port = z.number().int().min(0).max(65535);

/** Largest page (the agent allows ≤ 1000; DET44 sessions are per user, CNAT is one global table). */
export const MAX_PAGE_SIZE = 1000;

const paging = {
  page: z.coerce.number().int().min(1).max(4_000_000).default(1),
  pageSize: z.coerce.number().int().min(1).max(MAX_PAGE_SIZE).default(100),
};

export const Det44SessionsQuery = z.object({
  user: ipv4.describe('inside (subscriber) IPv4 address — VPP keeps DET44 sessions per user'),
  ...paging,
});
export type Det44SessionsQuery = z.output<typeof Det44SessionsQuery>;

export const Det44SessionOut = z.object({
  insidePort: z.number().int(),
  outsidePort: z.number().int(),
  externalAddress: z.string(),
  externalPort: z.number().int(),
  state: z.string(),
  expire: z.number().int().describe('VPP expiry time (seconds, VPP-relative)'),
});

export const Det44SessionsOut = z.object({
  user: z.string(),
  outsideAddress: z.string().describe("the user's deterministic outside address"),
  portLo: z.number().int(),
  portHi: z.number().int(),
  page: z.number().int(),
  pageSize: z.number().int(),
  total: z.number().int(),
  retrievedAt: z.string().optional(),
  items: z.array(Det44SessionOut),
});

export const Det44LookupBody = z.union([
  z.strictObject({
    inside: ipv4.describe('forward: inside address → outside address + port block'),
  }),
  z.strictObject({
    outside: ipv4.describe('reverse: outside address + port → inside address'),
    port: z.number().int().min(1024).max(65535),
  }),
]);
export type Det44LookupBody = z.output<typeof Det44LookupBody>;

export const Det44LookupOut = z.object({
  inside: z.string(),
  outside: z.string(),
  portLo: z.number().int().nullable().describe('forward only'),
  portHi: z.number().int().nullable().describe('forward only'),
});

export const Det44CloseBody = z.object({
  direction: z
    .enum(['in', 'out'])
    .describe('in: by the inside endpoint; out: by the outside endpoint'),
  address: ipv4,
  port,
  externalAddress: ipv4,
  externalPort: port,
});
export type Det44CloseBody = z.output<typeof Det44CloseBody>;

export const DoneOut = z.object({ summary: z.string() });

export const CnatSessionsQuery = z.object(paging);
export type CnatSessionsQuery = z.output<typeof CnatSessionsQuery>;

export const CnatSessionOut = z.object({
  dstAddress: z.string(),
  dstPort: z.number().int(),
  srcAddress: z.string(),
  srcPort: z.number().int(),
  protocol: z.string(),
  translationIndex: z.number().int(),
  flags: z.number().int(),
});

export const CnatSessionsOut = z.object({
  page: z.number().int(),
  pageSize: z.number().int(),
  total: z.number().int(),
  truncated: z
    .boolean()
    .describe("the agent's per-call cap stopped the dump: total is a lower bound"),
  retrievedAt: z.string().optional(),
  items: z.array(CnatSessionOut),
});
