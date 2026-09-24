import type { paths } from '@ngfw/api-client';
import type { NatConfig } from '@ngfw/schema';
import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import { domainSchemas } from '../../../schema/registry';

type Ok<O> = O extends { responses: { 200: { content: { 'application/json': infer T } } } }
  ? T
  : never;
type Body<O> = O extends { requestBody: { content: { 'application/json': infer T } } } ? T : never;

/** `GET /api/v1/state/nat/sessions` as generated from the OpenAPI document (never hand-written). */
export type SessionsPage = Ok<NonNullable<paths['/api/v1/state/nat/sessions']['get']>>;
export type Session = SessionsPage['items'][number];
/** `GET /api/v1/state/nat/summary`. */
export type NatSummary = Ok<NonNullable<paths['/api/v1/state/nat/summary']['get']>>;
export type PoolUsage = NatSummary['pools'][number];
/** `POST /api/v1/actions/nat/sessions/kill` body. */
export type KillBody = Body<NonNullable<paths['/api/v1/actions/nat/sessions/kill']['post']>>;

/** VPP's default VRF: what a missing `vrf` means. */
export const DEFAULT_VRF = 'default';

/** Pointer prefix of the NAT domain in server problems. */
export const NAT_POINTER = '/nat';

/** The `nat` node of the candidate (NAT44 at the top level, the translators as siblings). */
export type { NatConfig };

/** The `nat` domain JSON Schema — the one schema (00-CONTEXT rule 5), generated from the Zod source. */
export function natSchema(): JsonSchema {
  return domainSchemas.nat;
}

/** Keys of `nat` edited on the Outbound tab (NAT44 settings; lists have their own tabs). */
export const OUTBOUND_KEYS = [
  'enabled',
  'mode',
  'inside',
  'outside',
  'outputFeature',
  'insideVrf',
  'outsideVrf',
  'forwarding',
  'sessionLimit',
  'timeouts',
] as const;

/** Keys of `nat` that are lists of items edited one item at a time. */
export type NatListKey = 'pools' | 'staticMappings' | 'identityMappings' | 'loadBalancedMappings';

/** `nat` restricted to `keys` (same property schemas, same `required`/`additionalProperties` intent). */
export function pickSchema(keys: readonly string[]): JsonSchema {
  const s = natSchema();
  const props = (s.properties ?? {}) as Record<string, JsonSchema>;
  const picked = Object.fromEntries(keys.filter((k) => k in props).map((k) => [k, props[k]!]));
  return {
    type: 'object',
    properties: picked,
    additionalProperties: false,
    ...(s.$defs ? { $defs: s.$defs } : {}),
  } as JsonSchema;
}

/** Item schema of one `nat.<list>` array. */
export function itemSchema(key: NatListKey): JsonSchema {
  const props = (natSchema().properties ?? {}) as Record<string, JsonSchema>;
  const items = (props[key] as { items?: JsonSchema } | undefined)?.items;
  if (!items || typeof items !== 'object') throw new Error(`nat.${key} item schema not found`);
  return items;
}

/** The value of the outbound keys in the candidate (absent keys stay absent: no defaults are written back). */
export function pickValue(
  nat: Record<string, unknown> | undefined,
  keys: readonly string[],
): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const k of keys) if (nat && nat[k] !== undefined) out[k] = nat[k];
  return out;
}

/** Identity of a configured pool as the agent reports usage: range (+VRF) or interface, and the twice-NAT class. */
export function poolKey(p: {
  range?: string | null;
  interface?: string | null;
  vrf?: string | null;
  twiceNat?: boolean | null;
}): string {
  if (p.interface) return `if|${p.interface}|${p.twiceNat === true}`;
  return `range|${p.range ?? ''}|${p.vrf || 'default'}|${p.twiceNat === true}`;
}

/** The usage row the summary reports for a configured pool: by name first, then by identity. */
export function usageFor(
  pool: Record<string, unknown>,
  usage: readonly PoolUsage[],
): PoolUsage | undefined {
  const name = typeof pool['name'] === 'string' ? pool['name'] : undefined;
  const byName = usage.find((u) => u.name !== null && u.name === name);
  if (byName) return byName;
  const key = poolKey({
    range: typeof pool['range'] === 'string' ? pool['range'] : null,
    interface: typeof pool['interface'] === 'string' ? pool['interface'] : null,
    vrf: typeof pool['vrf'] === 'string' ? pool['vrf'] : null,
    twiceNat: pool['twiceNat'] === true,
  });
  return usage.find((u) => poolKey(u) === key);
}

/** The kill request for one session row (the ED 5-tuple + inside VRF). */
export function killBodyOf(s: Session): KillBody {
  const protocol =
    s.protocol === 'tcp' || s.protocol === 'udp' || s.protocol === 'icmp' ? s.protocol : 'tcp';
  return {
    protocol,
    insideAddress: s.insideAddress,
    insidePort: s.insidePort,
    externalAddress: s.externalAddress,
    externalPort: s.externalPort,
    vrf: s.vrf,
  };
}

/** Stable row id of a session (5-tuple + table). */
export function sessionId(s: Session): string {
  return `${s.tableId}|${s.protocol}|${s.insideAddress}:${s.insidePort}|${s.externalAddress}:${s.externalPort}`;
}

/** Session filters of the grid (the query parameters of `GET /state/nat/sessions`). */
export interface SessionFilter {
  inside: string;
  outside: string;
  external: string;
  port: string;
  protocol: string;
  vrf: string;
}

export const EMPTY_FILTER: SessionFilter = {
  inside: '',
  outside: '',
  external: '',
  port: '',
  protocol: '',
  vrf: '',
};

/** Only the filled-in filters, as query parameters. */
export function filterQuery(f: SessionFilter): Record<string, string | number> {
  const out: Record<string, string | number> = {};
  for (const [k, v] of Object.entries(f)) {
    const val = v.trim();
    if (val === '') continue;
    out[k] = k === 'port' && /^[0-9]+$/.test(val) ? Number(val) : val;
  }
  return out;
}
