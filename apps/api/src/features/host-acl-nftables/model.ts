import type { HostAclStateResponse } from '@ngfw/proto';
import { z } from 'zod';

/**
 * `GET /api/v1/state/host-acl` (F-host-acl-nftables): the host firewall table as the agent reports it (HostAclState RPC,
 * docs/contracts/proto.md §11) plus a per-configuration-rule aggregate of the kernel counters. uint64 counters are
 * decimal strings, as in `/state/interfaces` (D-039: `forceLong=string`, exact above 2^53).
 */

const counter = z.string().regex(/^\d+$/).describe('uint64 counter as a decimal string');

export const HostAclRuleOut = z.object({
  kind: z
    .enum(['established', 'loopback', 'icmp', 'anti-lockout', 'rule', 'unknown'])
    .describe(
      'what rendered the rule; unknown = not rendered by this agent (or its rendering is unknown)',
    ),
  list: z.string().describe('kind=rule: the host list (acl.host.<list>)'),
  sequence: z.number().int().describe('kind=rule: the rule sequence; 0 otherwise'),
  pointer: z.string().describe('kind=rule: JSON pointer of the rule in the applied document'),
  text: z
    .string()
    .describe('the rule as rendered (nftables syntax, without the counter); "" when unknown'),
  verdict: z.string().describe('accept | drop | reject'),
  comment: z
    .string()
    .describe('nftables comment, the rule identity vrx:<list>:<sequence>/<n>:<hash>'),
  packets: counter,
  bytes: counter,
});

export const HostAclChainOut = z.object({
  name: z.string(),
  hook: z.string().describe('input | output | forward'),
  priority: z.number().int(),
  policy: z.string().describe('accept | drop'),
  list: z.string().describe('the attached host list; "" when unknown'),
  rules: z.array(HostAclRuleOut),
});

export const HostAclSetOut = z.object({
  name: z.string().describe('a4_<object> / a6_<object>'),
  type: z.string().describe('ipv4_addr | ipv6_addr'),
  object: z.string().describe('address object or group it was expanded from; "" when unknown'),
  elements: z.array(z.string()).describe('canonical prefixes, sorted'),
});

export const HostAclRuleCountersOut = z.object({
  list: z.string(),
  sequence: z.number().int(),
  pointer: z.string(),
  packets: counter.describe('sum over the kernel rules this configuration rule rendered to'),
  bytes: counter,
  nftRules: z
    .number()
    .int()
    .describe('how many kernel rules (families × protocols) it rendered to'),
});

export const HostAclStateOut = z.object({
  retrievedAt: z.string().optional(),
  table: z
    .string()
    .describe('nftables table in family inet: vrx (product) or vrx_<owner> (test slots)'),
  mode: z
    .enum(['apply', 'netns', 'check'])
    .describe(
      'apply = root netns; netns = a test slot namespace; check = validated with nft -c only, never loaded',
    ),
  present: z.boolean().describe('the table exists in the kernel'),
  inSync: z
    .boolean()
    .describe('the kernel table equals the last applied rendering (false = drift)'),
  sets: z.array(HostAclSetOut),
  chains: z.array(HostAclChainOut),
  rules: z
    .array(HostAclRuleCountersOut)
    .describe('per configuration rule (list + sequence), sorted'),
});

export type HostAclStateJson = z.output<typeof HostAclStateOut>;
type Kind = z.output<typeof HostAclRuleOut>['kind'];
type Mode = HostAclStateJson['mode'];

const KINDS = new Set<string>([
  'established',
  'loopback',
  'icmp',
  'anti-lockout',
  'rule',
  'unknown',
]);
const MODES = new Set<string>(['apply', 'netns', 'check']);

/** A counter from the agent as a canonical decimal string ("" or garbage → "0"). */
function u64(v: string | number | undefined): string {
  const s = String(v ?? '0');
  return /^\d+$/.test(s) ? BigInt(s).toString() : '0';
}

/**
 * Per configuration rule (list + sequence) aggregate: one host rule renders to one kernel rule per address family and
 * protocol group, each with its own counter; the UI shows their sum next to the configured rule.
 */
export function aggregateRuleCounters(
  chains: HostAclStateJson['chains'],
): HostAclStateJson['rules'] {
  const acc = new Map<
    string,
    {
      list: string;
      sequence: number;
      pointer: string;
      packets: bigint;
      bytes: bigint;
      nftRules: number;
    }
  >();
  for (const chain of chains) {
    for (const r of chain.rules) {
      if (r.kind !== 'rule' || r.list === '') continue;
      const key = `${r.list}\u0000${r.sequence}`;
      const a = acc.get(key) ?? {
        list: r.list,
        sequence: r.sequence,
        pointer: r.pointer,
        packets: 0n,
        bytes: 0n,
        nftRules: 0,
      };
      a.packets += BigInt(r.packets);
      a.bytes += BigInt(r.bytes);
      a.nftRules += 1;
      if (a.pointer === '') a.pointer = r.pointer;
      acc.set(key, a);
    }
  }
  return [...acc.values()]
    .sort((x, y) => (x.list === y.list ? x.sequence - y.sequence : x.list < y.list ? -1 : 1))
    .map((a) => ({ ...a, packets: a.packets.toString(), bytes: a.bytes.toString() }));
}

/** HostAclStateResponse → the route's JSON (every field present; ts-proto drops nothing here, but be explicit). */
export function hostAclStateJson(r: HostAclStateResponse): HostAclStateJson {
  const chains = r.chains.map((c) => ({
    name: c.name,
    hook: c.hook,
    priority: c.priority,
    policy: c.policy,
    list: c.list,
    rules: c.rules.map((x) => ({
      kind: (KINDS.has(x.kind) ? x.kind : 'unknown') as Kind,
      list: x.list,
      sequence: x.sequence,
      pointer: x.pointer,
      text: x.text,
      verdict: x.verdict,
      comment: x.comment,
      packets: u64(x.packets),
      bytes: u64(x.bytes),
    })),
  }));
  return {
    retrievedAt: r.retrievedAt?.toISOString(),
    table: r.table,
    mode: (MODES.has(r.mode) ? r.mode : 'check') as Mode,
    present: r.present,
    inSync: r.inSync,
    sets: r.sets.map((s) => ({
      name: s.name,
      type: s.type,
      object: s.object,
      elements: [...s.elements],
    })),
    chains,
    rules: aggregateRuleCounters(chains),
  };
}
