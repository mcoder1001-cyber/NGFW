import { z } from 'zod';
import { MAX_PAGE_SIZE } from '../nat44-ed-sessions/dto.js';

/**
 * Request/response DTOs of F-nat44-ei-64-66-nptv6's state routes. The NAT44-EI session page has the NAT44-ED shape
 * (`SessionsQuery` / `SessionsOut` of F-nat44-ed-sessions, reused as they are); NAT64 sessions, the EI kill and the
 * NPTv6 bindings are defined here. One Zod definition per shape serves validation and OpenAPI (00-CONTEXT rule 5).
 */

const ipv4 = z.ipv4();
const vrfName = z
  .string()
  .max(63)
  .regex(/^[A-Za-z0-9][A-Za-z0-9_.-]*$/, 'a VRF name ("default", a configured VRF or a table id)');

export const Nat64SessionsQuery = z.object({
  page: z.coerce.number().int().min(1).max(4_000_000).default(1),
  pageSize: z.coerce.number().int().min(1).max(MAX_PAGE_SIZE).default(100),
  protocol: z
    .string()
    .regex(/^(tcp|udp|icmp|[0-9]{1,3})$/i, 'tcp, udp, icmp or an IP protocol number')
    .optional(),
});
export type Nat64SessionsQuery = z.output<typeof Nat64SessionsQuery>;

export const Nat64SessionOut = z.object({
  client: z.string().describe('IPv6 client (inside)'),
  clientPort: z.number().int(),
  poolAddress: z.string().describe('IPv4 pool address the client is translated to'),
  poolPort: z.number().int(),
  remote: z.string().describe('IPv4 remote host'),
  remotePort: z.number().int(),
  remoteIpv6: z.string().describe('the remote as the client addresses it: NAT64 prefix + IPv4'),
  protocol: z.string(),
  vrf: z.string(),
  tableId: z.number().int(),
});
export type Nat64SessionOut = z.infer<typeof Nat64SessionOut>;

export const Nat64SessionsOut = z.object({
  page: z.number().int(),
  pageSize: z.number().int(),
  total: z.number().int().describe('sessions of this agent (a lower bound when truncated)'),
  totalClients: z.number().int().describe('distinct IPv6 clients'),
  truncated: z.boolean().describe('the agent stopped counting at its scan cap'),
  retrievedAt: z.string().optional(),
  items: z.array(Nat64SessionOut),
});

export const EiKillBody = z.object({
  protocol: z.enum(['tcp', 'udp', 'icmp']),
  insideAddress: ipv4,
  insidePort: z.number().int().min(0).max(65535),
  externalAddress: ipv4
    .optional()
    .describe(
      'optional: NAT44-EI finds the session by its inside endpoint; echoed in the audit only',
    ),
  externalPort: z.number().int().min(0).max(65535).optional(),
  vrf: vrfName.optional().describe('inside VRF; default "default"'),
});
export type EiKillBody = z.output<typeof EiKillBody>;

export const EiKillOut = z.object({
  deleted: z.literal(true),
  summary: z.string(),
  stats: z.record(z.string(), z.string()),
});

export const Nptv6BindingOut = z.object({
  interface: z.string(),
  internal: z.string(),
  external: z.string(),
  description: z.string().nullable(),
});

export const Nptv6Out = z.object({
  writeOnly: z
    .literal(true)
    .describe(
      'VPP 26.06 has no npt66 dump: the bindings are those of the running configuration; the agent re-applies them on every resync and cannot read them back',
    ),
  bindings: z.array(Nptv6BindingOut),
});
