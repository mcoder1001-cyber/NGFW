import type { paths } from '@ngfw/api-client';
import type { DhcpRelay, DhcpServer, DhcpSubnet, RootConfig } from '@ngfw/schema';
import type { JsonSchema, ProblemDetails } from '@ngfw/ui-kit/schema-form';
import { ApiError } from '../../../api-problem';
import { domainSchemas } from '../../../schema/registry';

type Ok<O> = O extends { responses: { 200: { content: { 'application/json': infer T } } } }
  ? T
  : never;

/** `GET /api/v1/state/dhcp/leases` as generated from the OpenAPI document (never hand-written). */
export type LeasesState = Ok<NonNullable<paths['/api/v1/state/dhcp/leases']['get']>>;
export type Lease = LeasesState['items'][number];
export type ServerStatus = LeasesState['servers'][number];
export type SubnetUsage = ServerStatus['subnets'][number];
/** `GET /api/v1/state/dhcp/relays`. */
export type RelaysState = Ok<NonNullable<paths['/api/v1/state/dhcp/relays']['get']>>;
export type RelayItem = RelaysState['items'][number];
export type RelayStateName = RelayItem['state'];
export type LeasesQuery = NonNullable<
  NonNullable<paths['/api/v1/state/dhcp/leases']['get']>['parameters']['query']
>;

/**
 * A candidate node as the API stores it: merge patches keep what was sent, so every member may be absent (a record's
 * entries are present when their key is).
 */
type Loose<T> = T extends readonly (infer U)[]
  ? Loose<U>[]
  : T extends object
    ? string extends keyof T
      ? { [K in keyof T]: Loose<T[K]> }
      : { [K in keyof T]?: Loose<T[K]> }
    : T;

/**
 * The DHCP part of the candidate, typed from the configuration schema (`@ngfw/schema`, never hand-written); every
 * member is optional here because the schema fills defaults only at commit.
 */
export type ServerCfg = Loose<DhcpServer>;
export type SubnetCfg = Loose<DhcpSubnet>;
export type PoolCfg = NonNullable<SubnetCfg['pools']>[number];
export type ReservationCfg = NonNullable<NonNullable<SubnetCfg['reservations']>[string]>;
export type RelayCfg = Loose<DhcpRelay>;
export type DhcpCfg = Loose<RootConfig['services']['dhcp']>;
export type ServicesCfg = Loose<RootConfig['services']>;

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

function prop(schema: JsonSchema | undefined, name: string): JsonSchema {
  const p = (schema?.properties as Record<string, JsonSchema> | undefined)?.[name];
  if (!p || typeof p !== 'object') throw new Error(`schema property ${name} not found`);
  return p;
}

function recordValue(schema: JsonSchema): JsonSchema {
  const v = (schema as { additionalProperties?: unknown }).additionalProperties;
  if (!isObject(v)) throw new Error('record value schema not found');
  return v as JsonSchema;
}

/** `schema` without the named properties (edited in their own tab). */
export function without(schema: JsonSchema, ...names: string[]): JsonSchema {
  const props = { ...((schema.properties ?? {}) as Record<string, JsonSchema>) };
  for (const n of names) delete props[n];
  const required = Array.isArray(schema.required)
    ? schema.required.filter((r) => !names.includes(r))
    : undefined;
  return { ...schema, properties: props, ...(required ? { required } : {}) } as JsonSchema;
}

/** The one schema (00-CONTEXT rule 5): item schemas of `services.dhcp` from the generated domain JSON Schema. */
export function serverItemSchema(): JsonSchema {
  return recordValue(prop(prop(domainSchemas.services, 'dhcp'), 'servers'));
}
export function subnetItemSchema(): JsonSchema {
  return recordValue(prop(serverItemSchema(), 'subnets'));
}
export function reservationItemSchema(): JsonSchema {
  return recordValue(prop(subnetItemSchema(), 'reservations'));
}
export function relayItemSchema(): JsonSchema {
  return recordValue(prop(prop(domainSchemas.services, 'dhcp'), 'relays'));
}

/** Form schemas: servers without subnets, subnets without reservations (each has its own tab). */
export const serverFormSchema = (): JsonSchema => without(serverItemSchema(), 'subnets');
export const subnetFormSchema = (): JsonSchema => without(subnetItemSchema(), 'reservations');

type Translate = (key: string, opts?: Record<string, unknown>) => string;

/**
 * Titles and help in the UI language, recursively: `kea-dhcp-relay:label.<base>_<path>` / `help.<base>_<path>` (path
 * segments joined with `_`; record values and array items keep their parent's path, their own title stays). The
 * schema's English text is the fallback. `base` names the form: server, subnet, reservation, relay.
 */
export function localize(schema: JsonSchema, t: Translate, base: string): JsonSchema {
  return localizeNode(schema, t, base, true);
}

function localizeNode(schema: JsonSchema, t: Translate, path: string, root: boolean): JsonSchema {
  const out: Record<string, unknown> = { ...schema };
  if (!root) {
    const hints = isObject(schema['x-vrx-ui']) ? schema['x-vrx-ui'] : {};
    const help = t(`help.${path}`, {
      defaultValue: typeof hints['help'] === 'string' ? hints['help'] : '',
    });
    out['title'] = t(`label.${path}`, { defaultValue: schema.title ?? path });
    out['x-vrx-ui'] = { ...hints, ...(help ? { help } : {}) };
  }
  if (isObject(schema.properties)) {
    out['properties'] = Object.fromEntries(
      Object.entries(schema.properties as Record<string, JsonSchema>).map(([k, v]) => [
        k,
        localizeNode(v, t, `${path}_${k}`, false),
      ]),
    );
  }
  const ap = (schema as { additionalProperties?: unknown }).additionalProperties;
  if (isObject(ap)) out['additionalProperties'] = localizeInner(ap as JsonSchema, t, path);
  const items = (schema as { items?: unknown }).items;
  if (isObject(items)) out['items'] = localizeInner(items as JsonSchema, t, path);
  return out as JsonSchema;
}

/** Record values / array items: their properties are localized, their own title stays the schema's. */
function localizeInner(schema: JsonSchema, t: Translate, path: string): JsonSchema {
  const loc = localizeNode(schema, t, path, true);
  return loc;
}

/** Server pointers under `prefix` (e.g. `/services/dhcp/servers/lan`) → pointers relative to the edited form. */
export function problemFor(error: unknown, prefix: string): ProblemDetails | null {
  if (!(error instanceof ApiError)) return null;
  const p = error.toFormProblem();
  return {
    ...p,
    errors: (p.errors ?? []).map((e) => ({
      ...e,
      pointer: e.pointer.startsWith(prefix) ? e.pointer.slice(prefix.length) : e.pointer,
    })),
  };
}

export const esc = (s: string) => s.replace(/~/g, '~0').replace(/\//g, '~1');

/** A merge patch of `services` that sets `value` (or `null` = remove) at `path` below it. */
export function nestPatch(path: readonly string[], value: unknown): Record<string, unknown> {
  let out: unknown = value;
  for (let i = path.length - 1; i >= 0; i--) out = { [path[i]!]: out };
  return out as Record<string, unknown>;
}

/** Percentage (0–100, one decimal) of assigned over total addresses; decimal strings (DHCPv6 totals exceed 2^53). */
export function usagePercent(assigned: string, total: string): number {
  try {
    const a = BigInt(assigned || '0');
    const t = BigInt(total || '0');
    if (t <= 0n) return 0;
    const permille = (a * 1000n) / t;
    return Math.min(100, Number(permille) / 10);
  } catch {
    return 0;
  }
}

export interface ServerRow {
  id: string;
  name: string;
  family: 'ipv4' | 'ipv6';
  vrf: string;
  interfaces: string[];
  enabled: boolean;
  subnets: number;
  cfg: ServerCfg;
}

export function serverRows(dhcp: DhcpCfg | undefined): ServerRow[] {
  return Object.entries(dhcp?.servers ?? {})
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([name, s]) => ({
      id: name,
      name,
      family: s.family ?? 'ipv4',
      vrf: s.vrf ?? 'default',
      interfaces: s.interfaces ?? [],
      enabled: s.enabled !== false,
      subnets: Object.keys(s.subnets ?? {}).length,
      cfg: s,
    }));
}

export interface SubnetRow {
  id: string;
  server: string;
  family: 'ipv4' | 'ipv6';
  name: string;
  prefix: string;
  pools: string[];
  reservations: number;
  usage: SubnetUsage | undefined;
  cfg: SubnetCfg;
}

export function subnetRows(
  dhcp: DhcpCfg | undefined,
  status: readonly ServerStatus[] | undefined,
): SubnetRow[] {
  const usage = new Map<string, SubnetUsage>();
  for (const s of status ?? []) for (const u of s.subnets) usage.set(`${u.server}/${u.subnet}`, u);
  const out: SubnetRow[] = [];
  for (const srv of serverRows(dhcp)) {
    for (const [name, sub] of Object.entries(srv.cfg.subnets ?? {}).sort(([a], [b]) =>
      a.localeCompare(b),
    )) {
      out.push({
        id: `${srv.name}/${name}`,
        server: srv.name,
        family: srv.family,
        name,
        prefix: sub.subnet ?? '',
        pools: (sub.pools ?? []).map((p) => `${p.start ?? ''}–${p.end ?? ''}`),
        reservations: Object.keys(sub.reservations ?? {}).length,
        usage: usage.get(`${srv.name}/${name}`),
        cfg: sub,
      });
    }
  }
  return out;
}

export interface ReservationRow {
  id: string;
  server: string;
  subnet: string;
  name: string;
  client: string;
  ip: string;
  hostname: string;
  cfg: ReservationCfg;
}

export function reservationRows(dhcp: DhcpCfg | undefined): ReservationRow[] {
  const out: ReservationRow[] = [];
  for (const sub of subnetRows(dhcp, undefined)) {
    for (const [name, r] of Object.entries(sub.cfg.reservations ?? {}).sort(([a], [b]) =>
      a.localeCompare(b),
    )) {
      out.push({
        id: `${sub.server}/${sub.name}/${name}`,
        server: sub.server,
        subnet: sub.name,
        name,
        client: r.mac ?? r.duid ?? '',
        ip: r.ip ?? '',
        hostname: r.hostname ?? '',
        cfg: r,
      });
    }
  }
  return out;
}

export interface RelayRow {
  id: string;
  name: string;
  vrf: string;
  serverVrf: string;
  servers: string[];
  source: string;
  interfaces: string[];
  state: RelayStateName | 'pending';
  cfg: RelayCfg | undefined;
}

/**
 * Relays: the candidate's (editable) plus what the agent retrieves (`/state/dhcp/relays`); a candidate relay the state
 * does not list yet is `pending` (not committed).
 */
export function relayRows(
  dhcp: DhcpCfg | undefined,
  state: readonly RelayItem[] | undefined,
): RelayRow[] {
  const cand = dhcp?.relays ?? {};
  const byName = new Map((state ?? []).map((i) => [i.name, i]));
  const names = [...new Set([...Object.keys(cand), ...byName.keys()])].sort((a, b) =>
    a.localeCompare(b),
  );
  return names.map((name) => {
    const cfg = cand[name];
    const st = byName.get(name);
    const src = (cfg ??
      (st?.config as RelayCfg | null) ??
      (st?.retrieved as RelayCfg | null) ??
      {}) as RelayCfg;
    const vrf = src.vrf ?? 'default';
    return {
      id: name,
      name,
      vrf,
      serverVrf: src.serverVrf ?? vrf,
      servers: src.servers ?? [],
      source: src.sourceAddress ?? '',
      interfaces: src.interfaces ?? [],
      state: st?.state ?? 'pending',
      cfg,
    };
  });
}
