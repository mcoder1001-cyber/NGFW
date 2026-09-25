import type { paths } from '@ngfw/api-client';
import {
  canonicalIp,
  canonicalPrefix,
  parseIpv6,
  SRV6_MAX_SIDS,
  type Srv6Config,
  type Srv6LocalSidConfig,
  type Srv6PolicyConfig,
  type Srv6SidListConfig,
  type Srv6SteeringConfig,
} from '@ngfw/schema';
import type { VrxStatus } from '@ngfw/ui-kit';
import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import { domainSchemas } from '../../../schema/registry';

type Ok<O> = O extends { responses: { 200: { content: { 'application/json': infer T } } } }
  ? T
  : never;

/** `GET /api/v1/state/srv6` as generated from the OpenAPI document (never hand-written). */
export type Srv6State = Ok<NonNullable<paths['/api/v1/state/srv6']['get']>>;
export type LocalSidState = Srv6State['localSids'][number];
export type PolicyState = Srv6State['policies'][number];
export type SteeringState = Srv6State['steering'][number];

export type {
  Srv6Config,
  Srv6LocalSidConfig,
  Srv6PolicyConfig,
  Srv6SidListConfig,
  Srv6SteeringConfig,
};

/** `routing.srv6` of the candidate, every collection present (absent = SRv6 not configured). */
export interface Srv6View {
  encapSource?: string | undefined;
  encapHopLimit?: number | undefined;
  localSids: Record<string, Srv6LocalSidConfig>;
  policies: Record<string, Srv6PolicyConfig>;
  steering: Srv6SteeringConfig[];
}

/** The candidate's `routing` node → its `srv6` with empty collections filled in. */
export function srv6Of(routing: unknown): Srv6View {
  const r = (routing ?? {}) as { srv6?: Partial<Srv6Config> };
  const s = r.srv6 ?? {};
  return {
    ...(s.encapSource !== undefined ? { encapSource: s.encapSource } : {}),
    ...(s.encapHopLimit !== undefined ? { encapHopLimit: s.encapHopLimit } : {}),
    localSids: s.localSids ?? {},
    policies: s.policies ?? {},
    steering: s.steering ?? [],
  };
}

// ---- the one schema (00-CONTEXT rule 5) ------------------------------------------------------------

function prop(s: JsonSchema | undefined, name: string): JsonSchema {
  const p = ((s?.properties ?? {}) as Record<string, JsonSchema>)[name];
  if (p === undefined) throw new Error(`schema property ${name} not found`);
  return p;
}

function recordItem(s: JsonSchema): JsonSchema {
  const a = (s as { additionalProperties?: JsonSchema }).additionalProperties;
  if (!a || typeof a !== 'object') throw new Error('record item schema not found');
  return a;
}

function without(s: JsonSchema, drop: readonly string[]): JsonSchema {
  const props = { ...((s.properties ?? {}) as Record<string, JsonSchema>) };
  for (const d of drop) delete props[d];
  const required = Array.isArray(s.required)
    ? s.required.filter((r) => !drop.includes(r))
    : undefined;
  return { ...s, properties: props, ...(required ? { required } : {}) } as JsonSchema;
}

/** `routing.srv6` object schema. */
export function srv6Schema(): JsonSchema {
  return prop(domainSchemas.routing, 'srv6');
}

/** `routing.srv6.localSids.<sid>` item schema. */
export function localSidFormSchema(): JsonSchema {
  return recordItem(prop(srv6Schema(), 'localSids'));
}

/** `routing.srv6.policies.<bsid>` item schema without `sidLists` (the SID-list editor edits those). */
export function policyFormSchema(): JsonSchema {
  return without(recordItem(prop(srv6Schema(), 'policies')), ['sidLists']);
}

/** The two VPP-wide settings of `routing.srv6` (encapSource, encapHopLimit). */
export function globalsFormSchema(): JsonSchema {
  return without(srv6Schema(), ['localSids', 'policies', 'steering']);
}

// ---- canonical form (rule routing.srv6-canonical) ------------------------------------------------------

/** Canonical IPv6 text (lower case, shortest `::` form); the input itself when it is not an address. */
export function canon(a: string): string {
  return canonicalIp(a.trim()) ?? a.trim();
}

export function isIpv6(a: string): boolean {
  return parseIpv6(a.trim()) !== undefined;
}

/** A local SID with its addresses in canonical form. */
export function canonLocalSid(l: Srv6LocalSidConfig): Srv6LocalSidConfig {
  return l.nextHop === undefined ? l : { ...l, nextHop: canon(l.nextHop) };
}

/** A policy with its addresses in canonical form. */
export function canonPolicy(p: Srv6PolicyConfig): Srv6PolicyConfig {
  return {
    ...p,
    ...(p.encapSource !== undefined ? { encapSource: canon(p.encapSource) } : {}),
    sidLists: p.sidLists.map((l) => ({ ...l, sids: l.sids.map(canon) })),
  };
}

export function canonSteering(s: Srv6SteeringConfig): Srv6SteeringConfig {
  return s.type === 'l3'
    ? { ...s, prefix: canonicalPrefix(s.prefix.trim()) ?? s.prefix.trim(), bsid: canon(s.bsid) }
    : { ...s, bsid: canon(s.bsid) };
}

/** Plain code-unit order — the agent sorts with Go string comparison, never localeCompare. */
function cmp(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0;
}

/**
 * The order Retrieve returns steering in (so the running document compares equal): every L3 entry sorted by VRF name
 * then canonical prefix, then every L2 entry sorted by interface name.
 */
export function sortSteering(list: readonly Srv6SteeringConfig[]): Srv6SteeringConfig[] {
  return [...list].sort((a, b) => {
    if (a.type !== b.type) return a.type === 'l3' ? -1 : 1;
    if (a.type === 'l3' && b.type === 'l3') return cmp(a.vrf, b.vrf) || cmp(a.prefix, b.prefix);
    if (a.type === 'l2' && b.type === 'l2') return cmp(a.interface, b.interface);
    return 0;
  });
}

/** Identity of a steering entry (what `routing.srv6-steering-unique` compares). */
export function steeringKey(s: {
  type: string;
  vrf?: string | null;
  prefix?: string | null;
  interface?: string | null;
}): string {
  return s.type === 'l2'
    ? `l2|${s.interface ?? ''}`
    : `l3|${s.vrf ?? 'default'}|${canonicalPrefix(s.prefix ?? '') ?? s.prefix ?? ''}`;
}

// ---- segment lists -------------------------------------------------------------------------------------

export { SRV6_MAX_SIDS };

/** A segment list being edited (SIDs as typed). */
export interface SidListDraft {
  sids: string[];
  weight: number;
}

export type SidListProblem = 'empty' | 'tooMany' | 'badSid' | 'badWeight' | 'noList';

/** What is wrong with the edited segment lists (first finding per list), or [] when they can be saved. */
export function sidListProblems(
  lists: readonly SidListDraft[],
): { list: number; problem: SidListProblem }[] {
  if (lists.length === 0) return [{ list: -1, problem: 'noList' }];
  const out: { list: number; problem: SidListProblem }[] = [];
  lists.forEach((l, i) => {
    if (l.sids.length === 0) out.push({ list: i, problem: 'empty' });
    else if (l.sids.length > SRV6_MAX_SIDS) out.push({ list: i, problem: 'tooMany' });
    else if (l.sids.some((s) => !isIpv6(s))) out.push({ list: i, problem: 'badSid' });
    else if (!Number.isInteger(l.weight) || l.weight < 1 || l.weight > 65535)
      out.push({ list: i, problem: 'badWeight' });
  });
  return out;
}

/** Move element `i` of `a` by `delta` (−1 up, +1 down); out-of-range moves return a copy unchanged. */
export function move<T>(a: readonly T[], i: number, delta: number): T[] {
  const j = i + delta;
  const out = [...a];
  if (i < 0 || i >= a.length || j < 0 || j >= a.length) return out;
  [out[i], out[j]] = [out[j]!, out[i]!];
  return out;
}

// ---- rows: configuration joined with the live state --------------------------------------------------

/** installed = configured and in VPP · missing = configured, not in VPP · unmanaged = in VPP, not configured. */
export type RowStatus = 'installed' | 'missing' | 'unmanaged';

export function rowChip(s: RowStatus): VrxStatus {
  return s === 'installed' ? 'up' : s === 'missing' ? 'down' : 'degraded';
}

export interface LocalSidRow {
  sid: string;
  config: Srv6LocalSidConfig | undefined;
  state: LocalSidState | undefined;
  status: RowStatus;
}

export function localSidRows(cfg: Srv6View, st: Srv6State | undefined): LocalSidRow[] {
  const live = new Map((st?.localSids ?? []).map((l) => [canon(l.sid), l]));
  const rows: LocalSidRow[] = Object.entries(cfg.localSids).map(([sid, config]) => {
    const state = live.get(canon(sid));
    return { sid, config, state, status: state ? 'installed' : 'missing' };
  });
  const named = new Set(rows.map((r) => canon(r.sid)));
  for (const [sid, state] of live)
    if (!named.has(sid))
      rows.push({ sid: state.sid, config: undefined, state, status: 'unmanaged' });
  return rows.sort((a, b) => cmp(canon(a.sid), canon(b.sid)));
}

export interface PolicyRow {
  bsid: string;
  config: Srv6PolicyConfig | undefined;
  state: PolicyState | undefined;
  status: RowStatus;
}

export function policyRows(cfg: Srv6View, st: Srv6State | undefined): PolicyRow[] {
  const live = new Map((st?.policies ?? []).map((p) => [canon(p.bsid), p]));
  const rows: PolicyRow[] = Object.entries(cfg.policies).map(([bsid, config]) => {
    const state = live.get(canon(bsid));
    return { bsid, config, state, status: state ? 'installed' : 'missing' };
  });
  const named = new Set(rows.map((r) => canon(r.bsid)));
  for (const [bsid, state] of live)
    if (!named.has(bsid))
      rows.push({ bsid: state.bsid, config: undefined, state, status: 'unmanaged' });
  return rows.sort((a, b) => cmp(canon(a.bsid), canon(b.bsid)));
}

export interface SteeringRow {
  key: string;
  config: Srv6SteeringConfig | undefined;
  /** Index in the candidate's steering list (for delete). */
  index: number;
  state: SteeringState | undefined;
  status: RowStatus;
}

export function steeringRows(cfg: Srv6View, st: Srv6State | undefined): SteeringRow[] {
  const live = new Map((st?.steering ?? []).map((s) => [steeringKey(s), s]));
  const rows: SteeringRow[] = cfg.steering.map((config, index) => {
    const key = steeringKey(config);
    const state = live.get(key);
    return { key, config, index, state, status: state ? 'installed' : 'missing' };
  });
  const named = new Set(rows.map((r) => r.key));
  for (const [key, state] of live)
    if (!named.has(key))
      rows.push({ key, config: undefined, index: -1, state, status: 'unmanaged' });
  return rows;
}

/** The outer source an encapsulating policy uses: its own, else the global one (D-074). */
export function effectiveEncapSource(p: Srv6PolicyConfig, cfg: Srv6View): string | undefined {
  if (!p.encap) return undefined;
  return p.encapSource ?? cfg.encapSource;
}

/** Binding SIDs that steering entries point at (a policy with steering cannot be deleted alone). */
export function steeredBsids(cfg: Srv6View): Set<string> {
  return new Set(cfg.steering.map((s) => canon(s.bsid)));
}

/** Traffic type of a configured steering entry (what the data plane reports as trafficType). */
export function trafficOf(s: Srv6SteeringConfig | undefined): 'ipv4' | 'ipv6' | 'l2' {
  if (s === undefined || s.type === 'l2') return 'l2';
  return s.prefix.includes(':') ? 'ipv6' : 'ipv4';
}
