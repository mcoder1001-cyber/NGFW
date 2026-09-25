import type { paths } from '@ngfw/api-client';
import type { LispConfig } from '@ngfw/schema';
import type { VrxStatus } from '@ngfw/ui-kit';
import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import { domainSchemas } from '../../../schema/registry';

type Ok<O> = O extends { responses: { 200: { content: { 'application/json': infer T } } } } ? T : never;

/** `GET /api/v1/state/lisp` as generated from the OpenAPI document (never hand-written). */
export type LispState = Ok<NonNullable<paths['/api/v1/state/lisp']['get']>>;
export type { LispConfig };

/** Pointer of `tunnels.lisp` in the document (server problems are mapped relative to it). */
export const LISP_POINTER = '/tunnels/lisp';

/** Sub-tabs of the LISP tab and the `tunnels.lisp` keys each one edits (the schema's `x-vrx-ui.group`). */
export const SECTIONS = [
  { id: 'locators', keys: ['enabled', 'gpe', 'locatorSets'] },
  { id: 'eids', keys: ['localEids', 'eidTables'] },
  { id: 'mappings', keys: ['remoteMappings', 'adjacencies', 'gpeEntries'] },
  { id: 'resolvers', keys: ['mapResolvers', 'mapServers', 'pitr'] },
] as const;
export type SectionId = (typeof SECTIONS)[number]['id'];

/** `tunnels.lisp` schema — the one schema (00-CONTEXT rule 5), from the generated domain JSON Schema. */
export function lispSchema(): JsonSchema {
  const props = (domainSchemas.tunnels.properties ?? {}) as Record<string, JsonSchema>;
  const l = props['lisp'];
  if (!l || typeof l !== 'object') throw new Error('tunnels.lisp schema not found');
  return l;
}

/** The form of one sub-tab: `tunnels.lisp` restricted to the section's keys. */
export function sectionSchema(id: SectionId): JsonSchema {
  const l = lispSchema();
  const keys: readonly string[] = SECTIONS.find((s) => s.id === id)!.keys;
  const all = (l.properties ?? {}) as Record<string, JsonSchema>;
  const properties = Object.fromEntries(keys.filter((k) => k in all).map((k) => [k, all[k]!]));
  const required = Array.isArray(l.required) ? l.required.filter((r) => keys.includes(r)) : undefined;
  const out = { ...l, properties, ...(required ? { required } : {}) } as JsonSchema;
  delete out.title;
  delete out.description;
  return out;
}

/** The section's slice of the configured `tunnels.lisp`. */
export function sectionValue(id: SectionId, lisp: Partial<LispConfig> | undefined): Record<string, unknown> {
  const keys: readonly string[] = SECTIONS.find((s) => s.id === id)!.keys;
  const src = (lisp ?? {}) as Record<string, unknown>;
  return Object.fromEntries(keys.filter((k) => src[k] !== undefined).map((k) => [k, src[k]]));
}

/** Merge-patch body for `tunnels` that replaces the section's keys of `lisp` (unset keys become null = removed). */
export function sectionPatch(id: SectionId, value: Record<string, unknown>): Record<string, unknown> {
  const keys: readonly string[] = SECTIONS.find((s) => s.id === id)!.keys;
  return { lisp: Object.fromEntries(keys.map((k) => [k, value[k] ?? null])) };
}

/** One row of a sub-tab's status table: a configured object and what VPP shows for it. */
export interface StatusRow {
  kind: 'locatorSet' | 'localEid' | 'eidTable' | 'remoteMapping' | 'adjacency' | 'gpeEntry' | 'mapResolver' | 'mapServer' | 'pitr' | 'switch';
  id: string;
  detail: string;
  /** up = VPP has it; down = configured, not in VPP; degraded = cannot be read back (V13, only its VNI is visible). */
  status: VrxStatus;
  labelKey: 'status.applied' | 'status.missing' | 'status.writeOnly';
}

const row = (kind: StatusRow['kind'], id: string, detail: string, present: boolean | 'partial'): StatusRow => ({
  kind,
  id,
  detail,
  status: present === 'partial' ? 'degraded' : present ? 'up' : 'down',
  labelKey: present === 'partial' ? 'status.writeOnly' : present ? 'status.applied' : 'status.missing',
});

/** Status rows of a section: every configured object, matched against the live state (null = state unavailable). */
export function statusRows(id: SectionId, lisp: Partial<LispConfig> | undefined, st: LispState | null): StatusRow[] {
  const l = (lisp ?? {}) as Partial<LispConfig>;
  const key = (vni: number, eid: string) => `${vni}/${eid.toLowerCase()}`;
  const mappings = new Map((st?.mappings ?? []).map((m) => [`${m.local ? 'l' : 'r'}:${key(m.vni, m.eid)}`, m]));
  switch (id) {
    case 'locators': {
      const sets = new Map((st?.locatorSets ?? []).map((s) => [s.name, s]));
      return [
        row('switch', 'LISP', l.enabled ? 'on' : 'off', !!st && st.enabled === !!l.enabled),
        row('switch', 'LISP-GPE', l.gpe ? 'on' : 'off', !!st && (!l.gpe || st.gpeEnabled)),
        ...Object.entries(l.locatorSets ?? {}).map(([name, s]) =>
          row('locatorSet', name, s.locators.map((x) => x.interface).join(', '), sets.has(name) && sets.get(name)!.locators.length === s.locators.length),
        ),
      ];
    }
    case 'eids': {
      const tables = new Set((st?.eidTables ?? []).map((t) => `${t.vni}/${t.isL2 ? 'l2' : 'l3'}`));
      return [
        ...(l.localEids ?? []).map((e) => row('localEid', key(e.vni, e.eid), e.locatorSet, mappings.has(`l:${key(e.vni, e.eid)}`))),
        ...Object.entries(l.eidTables ?? {}).map(([vni, t]) =>
          row('eidTable', vni, t.vrf !== undefined ? `vrf ${t.vrf}` : `bd ${t.bridgeDomain}`, tables.has(`${vni}/${t.bridgeDomain !== undefined ? 'l2' : 'l3'}`)),
        ),
      ];
    }
    case 'mappings': {
      const adj = new Set((st?.adjacencies ?? []).map((a) => `${a.vni}/${a.reid}/${a.leid}`));
      const gpe = new Set(st?.gpeVnis ?? []);
      return [
        ...(l.remoteMappings ?? []).map((m) =>
          row('remoteMapping', key(m.vni, m.eid), m.rlocs.length > 0 ? m.rlocs.map((r) => r.address).join(', ') : m.action, mappings.has(`r:${key(m.vni, m.eid)}`)),
        ),
        ...(l.adjacencies ?? []).map((a) => row('adjacency', `${a.vni}/${a.reid}`, a.leid, adj.has(`${a.vni}/${a.reid}/${a.leid}`))),
        ...(l.gpeEntries ?? []).map((g) =>
          row('gpeEntry', `${g.vni}/${g.reid}`, g.leid, gpe.has(g.vni) ? 'partial' : false),
        ),
      ];
    }
    case 'resolvers': {
      const mr = new Set(st?.mapResolvers ?? []);
      const ms = new Set(st?.mapServers ?? []);
      return [
        ...(l.mapResolvers ?? []).map((a) => row('mapResolver', a, '', mr.has(a))),
        ...(l.mapServers ?? []).map((a) => row('mapServer', a, '', ms.has(a))),
        ...(l.pitr !== undefined ? [row('pitr', l.pitr, '', st?.pitr === l.pitr)] : []),
      ];
    }
  }
}
