import { z } from 'zod';

/**
 * Request/response DTOs of the NAT44-ED session browser (F-nat44-ed-sessions). One Zod definition per shape serves
 * validation (ZodPipe → 400 problem+json with pointers) and OpenAPI (00-CONTEXT rule 5).
 */

const ipv4 = z.ipv4();
const port = z.coerce.number().int().min(0).max(65535);
const protocol = z
  .string()
  .regex(/^(tcp|udp|icmp|[0-9]{1,3})$/i, 'tcp, udp, icmp or an IP protocol number');
const vrfName = z
  .string()
  .max(63)
  .regex(/^[A-Za-z0-9][A-Za-z0-9_.-]*$/, 'a VRF name ("default", a configured VRF or a table id)');

/**
 * Largest page. The agent dumps at most 256 inside hosts per NatSessions call (each per-host dump walks VPP's whole
 * session table under the barrier, review H1), so a page of ≤ 256 sessions is never cut short by that cap.
 */
export const MAX_PAGE_SIZE = 256;

export const SessionsQuery = z.object({
  page: z.coerce.number().int().min(1).max(4_000_000).default(1),
  pageSize: z.coerce.number().int().min(1).max(MAX_PAGE_SIZE).default(100),
  inside: ipv4.optional().describe('inside (pre-translation) address'),
  outside: ipv4.optional().describe('outside (translated) address'),
  external: ipv4.optional().describe('external (remote) host address'),
  port: port.optional().describe('inside, outside or external port'),
  protocol: protocol.optional(),
  vrf: vrfName.optional().describe('inside VRF'),
});
export type SessionsQuery = z.output<typeof SessionsQuery>;

export const SessionOut = z.object({
  insideAddress: z.string(),
  insidePort: z.number().int(),
  outsideAddress: z.string(),
  outsidePort: z.number().int(),
  externalAddress: z.string(),
  externalPort: z.number().int(),
  externalNatAddress: z.string(),
  externalNatPort: z.number().int(),
  protocol: z.string(),
  vrf: z.string(),
  tableId: z.number().int(),
  static: z.boolean(),
  twiceNat: z.boolean(),
  timedOut: z.boolean(),
  idleSeconds: z.number(),
  bytes: z.number(),
  packets: z.number(),
});
export type SessionOut = z.infer<typeof SessionOut>;

export const SessionsOut = z.object({
  page: z.number().int(),
  pageSize: z.number().int(),
  total: z.number().int().describe('sessions matching the filter (a lower bound when truncated)'),
  totalUsers: z.number().int(),
  truncated: z
    .boolean()
    .describe(
      'the agent stopped at a per-call cap (256 inside hosts, 200 000 sessions): with an address/port/protocol filter total is a lower bound and later pages may be empty; narrow the filter (e.g. by inside address)',
    ),
  retrievedAt: z.string().optional(),
  items: z.array(SessionOut),
});

export const PoolUsageOut = z.object({
  name: z.string().nullable().describe('the running configuration pool this usage belongs to'),
  kind: z.enum(['range', 'interface']),
  range: z.string().nullable(),
  interface: z.string().nullable(),
  vrf: z.string().nullable(),
  twiceNat: z.boolean(),
  addresses: z.number().int(),
  sessions: z.number().int(),
  utilisation: z
    .number()
    .describe(
      'sessions / (addresses × 64 512 ports), capped at 1 (an estimate: ED reuses ports per destination)',
    ),
  applied: z.boolean().describe('the agent retrieved this pool from VPP'),
  configured: z.boolean().describe('the running configuration has this pool'),
});
export type PoolUsageOut = z.infer<typeof PoolUsageOut>;

export const SummaryOut = z.object({
  enabled: z.boolean(),
  sessionLimit: z.number().int().describe('per worker thread (VPP running config)'),
  totalUsers: z.number().int(),
  totalSessions: z.number().int(),
  staticSessions: z.number().int(),
  truncated: z
    .boolean()
    .describe(
      "the per-pool and per-protocol counts stopped at the agent's per-call caps (64 inside hosts, 200 000 sessions): lower bounds; the totals are complete",
    ),
  byProtocol: z.record(z.string(), z.number().int()),
  pools: z.array(PoolUsageOut),
  retrievedAt: z
    .string()
    .optional()
    .describe('when the agent computed the summary; it serves one computation for up to 30 s'),
});

export const KillBody = z.object({
  protocol: z.enum(['tcp', 'udp', 'icmp']),
  insideAddress: ipv4,
  insidePort: z.number().int().min(0).max(65535),
  externalAddress: ipv4,
  externalPort: z.number().int().min(0).max(65535),
  vrf: vrfName.optional().describe('inside VRF; default "default"'),
});
export type KillBody = z.output<typeof KillBody>;

export const KillOut = z.object({
  deleted: z.literal(true),
  summary: z.string(),
  stats: z.record(z.string(), z.string()),
});
