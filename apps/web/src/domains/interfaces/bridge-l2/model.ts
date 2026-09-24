import type { paths } from '@ngfw/api-client';
import {
  BridgeDomainSchema,
  BridgeL2PortSchema,
  L2XconnectSchema,
  L3xcSchema,
  MacFilterSchema,
  type BridgeDomainConfig,
  type BridgeL2Config,
  type BridgeL2PortConfig,
  type InterfaceConfig,
  type L2XconnectConfig,
  type L3xcConfig,
  type MacFilterConfig,
} from '@ngfw/schema';
import type { VrxStatus } from '@ngfw/ui-kit';
import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import { z } from 'zod';

type Ok<O> = O extends { responses: { 200: { content: { 'application/json': infer T } } } }
  ? T
  : never;

/** `GET /api/v1/state/l2/bridge-domains` as generated from the OpenAPI document (never hand-written). */
export type BridgeDomainsState = Ok<NonNullable<paths['/api/v1/state/l2/bridge-domains']['get']>>;
export type BridgeDomainItem = BridgeDomainsState['items'][number];
export type LiveBridgeDomain = NonNullable<BridgeDomainItem['state']>;
export type MacsPage = Ok<NonNullable<paths['/api/v1/state/l2/bridge-domains/{id}/macs']['get']>>;
export type MacRow = MacsPage['items'][number];

export type {
  BridgeDomainConfig,
  BridgeL2Config,
  BridgeL2PortConfig,
  InterfaceConfig,
  L2XconnectConfig,
  L3xcConfig,
  MacFilterConfig,
};

const OPTIONS = { target: 'draft-2020-12', io: 'input' } as const;

/** Form schemas — the one Zod schema of `packages/schema` as JSON Schema (00-CONTEXT rule 5). */
export const formSchemas = {
  domain: () => z.toJSONSchema(BridgeDomainSchema, OPTIONS) as JsonSchema,
  /** The member form: `bridgeDomain` is the bridge domain being edited, not a form field. */
  port: () =>
    withoutProps(z.toJSONSchema(BridgeL2PortSchema, OPTIONS) as JsonSchema, ['bridgeDomain']),
  xconnect: () => z.toJSONSchema(L2XconnectSchema, OPTIONS) as JsonSchema,
  l3xc: () => z.toJSONSchema(L3xcSchema, OPTIONS) as JsonSchema,
  macFilter: () => z.toJSONSchema(MacFilterSchema, OPTIONS) as JsonSchema,
};

export function withoutProps(schema: JsonSchema, names: string[]): JsonSchema {
  const props = { ...((schema.properties ?? {}) as Record<string, JsonSchema>) };
  for (const n of names) delete props[n];
  const required = Array.isArray(schema.required)
    ? schema.required.filter((r) => !names.includes(r))
    : undefined;
  return { ...schema, properties: props, ...(required ? { required } : {}) } as JsonSchema;
}

/** A (sub-)interface of the candidate with its L2 leaf. */
export interface Port {
  /** Logical name: `<parent>` or `<parent>.<id>`. */
  name: string;
  parent: string;
  sub: string | null;
  l2: BridgeL2PortConfig | undefined;
}

/** Every configured (sub-)interface, sorted by name. */
export function portsOf(ifs: Record<string, InterfaceConfig> | undefined): Port[] {
  const out: Port[] = [];
  for (const [parent, itf] of Object.entries(ifs ?? {})) {
    out.push({ name: parent, parent, sub: null, l2: itf.l2 });
    for (const [id, sub] of Object.entries(itf.subinterfaces ?? {})) {
      out.push({ name: `${parent}.${id}`, parent, sub: id, l2: sub.l2 });
    }
  }
  return out.sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true }));
}

/** Members of bridge domain `bd` in the candidate. */
export function membersOf(ports: Port[], bd: string): Port[] {
  return ports.filter((p) => p.l2?.bridgeDomain === bd);
}

/** Merge patch (for PATCH /config/interfaces) that sets the L2 leaf of `port` to `l2` (null removes it). */
export function portPatch(
  port: Pick<Port, 'parent' | 'sub'>,
  l2: unknown,
): Record<string, unknown> {
  return port.sub === null
    ? { [port.parent]: { l2 } }
    : { [port.parent]: { subinterfaces: { [port.sub]: { l2 } } } };
}

/** Merge patch (for PATCH /config/routing) that sets `routing.l2.<table>.<key>` (null removes it). */
export function l2Patch(
  table: keyof BridgeL2Config,
  key: string,
  value: unknown,
): Record<string, unknown> {
  return { l2: { [table]: { [key]: value } } };
}

/** `HH:MM–HH:MM mon,tue` summary of a MAC-filter device's ranges. */
export function rangesText(f: MacFilterConfig, always: string): string {
  if (!f.ranges || f.ranges.length === 0) return always;
  return f.ranges.map((r) => `${r.days.join(',')} ${r.start}–${r.end}`).join('; ');
}

/** Short tag-rewrite text: `pop-1`, `translate-1-1 300`, `push-2 100/200 (802.1ad)`. */
export function tagRewriteText(l2: BridgeL2PortConfig | undefined): string {
  const tr = l2?.tagRewrite;
  if (!tr) return '';
  const tags = [tr.tag1, tr.tag2].filter((x) => x !== undefined).join('/');
  return [tr.op, tags, tr.dot1ad ? '(802.1ad)' : ''].filter(Boolean).join(' ');
}

export const NAME_RE = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,31}$/;

/** The routing.l2 tables the page edits. */
export const L2_TABLES = {
  bridgeDomains: 'bridgeDomains',
  xconnects: 'xconnects',
  l3xc: 'l3xc',
  macFilters: 'macFilters',
} as const;

/** Chip status of "present in VPP". */
export function presence(live: boolean): VrxStatus {
  return live ? 'up' : 'down';
}

export type PortRole = 'normal' | 'bvi' | 'uu-fwd';

/** Role of a bridge member (i18n key `role.<role>`). */
export function roleOf(l2: BridgeL2PortConfig | undefined): PortRole {
  return l2?.bvi ? 'bvi' : l2?.uuFwd ? 'uu-fwd' : 'normal';
}

/** Kind of an L2 FIB entry (i18n key `type.<kind>`). */
export function macKind(m: { bvi: boolean; filter: boolean; static: boolean }): string {
  return m.bvi ? 'bvi' : m.filter ? 'filter' : m.static ? 'static' : 'learned';
}

/** Table cells aligned to the end of the row (logical, RTL-safe). */
export const END_CELL = { textAlign: 'end' } as const;

/**
 * P08's generated interface drawer (`interfaces/model.ts` → `interfaceItemSchema`) gets `l2` as an opaque JSON field
 * in the `bridge-l2` group: SchemaForm materialises an absent optional nested object with its defaults, and the
 * nested `tagRewrite` (required `op`) would then block every save of the drawer (P08-questions Q2). Opaque, the leaf is
 * absent when absent and round-trips unchanged when set; the full Zod schema still validates it on the server, and the
 * Bridging page edits it with its own schema form.
 */
export function drawerSafeL2(item: JsonSchema): JsonSchema {
  const opaque = (s: JsonSchema): JsonSchema => {
    const props = (s.properties ?? {}) as Record<string, JsonSchema>;
    const l2 = props['l2'];
    if (!l2) return s;
    const hints = (l2['x-vrx-ui'] ?? {}) as Record<string, unknown>;
    const out: JsonSchema = { 'x-vrx-ui': { ...hints, widget: 'json' } } as JsonSchema;
    if (l2.title !== undefined) out.title = l2.title;
    if (l2.description !== undefined) out.description = l2.description;
    return { ...s, properties: { ...props, l2: out } } as JsonSchema;
  };
  const top = opaque(item);
  const props = (top.properties ?? {}) as Record<string, JsonSchema>;
  const subs = props['subinterfaces'] as { additionalProperties?: JsonSchema } | undefined;
  if (!subs?.additionalProperties || typeof subs.additionalProperties !== 'object') return top;
  return {
    ...top,
    properties: {
      ...props,
      subinterfaces: { ...subs, additionalProperties: opaque(subs.additionalProperties) },
    },
  } as JsonSchema;
}
