import type {
  MplsConfig,
  MplsLabelRouteConfig,
  MplsPathConfig,
  MplsSrPolicyConfig,
  MplsTunnelConfig,
} from '@ngfw/schema';
import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import { domainSchemas } from '../../../schema/registry';

/** i18n namespace of the feature (= task slug, wave-A-hotspots §0 rule 5). */
export const NS = 'mpls-srmpls';

export type {
  MplsConfig,
  MplsLabelRouteConfig,
  MplsPathConfig,
  MplsSrPolicyConfig,
  MplsTunnelConfig,
};

/** The generated JSON Schema of `routing.mpls` — the one schema (00-CONTEXT rule 5). */
export function mplsSchema(): JsonSchema {
  const props = (domainSchemas.routing.properties ?? {}) as Record<string, JsonSchema>;
  const s = props['mpls'];
  if (!s || typeof s !== 'object') throw new Error('routing.mpls schema not found');
  return s;
}

function prop(s: JsonSchema, name: string): JsonSchema {
  const p = (s.properties as Record<string, JsonSchema> | undefined)?.[name];
  if (!p || typeof p !== 'object') throw new Error(`schema property ${name} not found`);
  return p;
}

/** Item schema of an array member of `routing.mpls` (`labelRoutes`, `ipBindings`) or of `routing.mpls.sr` (`steering`). */
export function itemSchema(...path: string[]): JsonSchema {
  let s = mplsSchema();
  for (const p of path) s = prop(s, p);
  const items = s.items as JsonSchema | undefined;
  if (items && typeof items === 'object') return items;
  const values = s.additionalProperties as JsonSchema | undefined;
  if (values && typeof values === 'object') return values;
  throw new Error(`no item schema at ${path.join('.')}`);
}

/** An object schema made of some members of `routing.mpls` (e.g. `interfaces` + `tables`). */
export function pickSchema(...names: string[]): JsonSchema {
  const s = mplsSchema();
  const properties = Object.fromEntries(names.map((n) => [n, prop(s, n)]));
  return { type: 'object', properties, additionalProperties: false } as JsonSchema;
}

/** A record entry edited as one object: its key as a `name` member, then the value's members. */
export function keyedSchema(value: JsonSchema, key: JsonSchema): JsonSchema {
  const props = (value.properties ?? {}) as Record<string, JsonSchema>;
  const required = [...((value.required as string[] | undefined) ?? [])];
  return {
    ...value,
    properties: { name: key, ...props },
    required: ['name', ...required],
  } as JsonSchema;
}

type Translate = (key: string, opts?: Record<string, unknown>) => string;

/** Titles and help in the UI language (`field.<path>.title|help`); the schema's English text stays the fallback. */
export function localizeSchema(schema: JsonSchema, t: Translate, prefix = ''): JsonSchema {
  const out: Record<string, unknown> = { ...schema };
  const props = schema.properties as Record<string, JsonSchema> | undefined;
  if (props) {
    const next: Record<string, JsonSchema> = {};
    for (const [name, p] of Object.entries(props)) {
      const key = prefix + name;
      const hints = (p['x-vrx-ui'] ?? {}) as Record<string, unknown>;
      const help = t(`field.${key}.help`, { defaultValue: '' });
      next[name] = {
        ...localizeSchema(p, t, `${key}.`),
        title: t(`field.${key}.title`, { defaultValue: p.title ?? name }),
        'x-vrx-ui': { ...hints, ...(help ? { help } : {}) },
      } as JsonSchema;
    }
    out['properties'] = next;
  }
  const items = schema.items as JsonSchema | undefined;
  if (items && typeof items === 'object' && !Array.isArray(items))
    out['items'] = localizeSchema(items, t, prefix);
  const values = schema.additionalProperties as JsonSchema | undefined;
  if (values && typeof values === 'object')
    out['additionalProperties'] = localizeSchema(values, t, prefix);
  return out as JsonSchema;
}

type Json = null | boolean | number | string | Json[] | { [k: string]: Json };

function isObject(v: unknown): v is Record<string, Json> {
  return v !== null && typeof v === 'object' && !Array.isArray(v);
}

/**
 * The RFC 7386 merge patch that turns `from` into `to` (members removed → null; arrays replaced whole; `undefined`
 * when nothing changes) — records such as `tunnels` and `sr.policies` lose entries only through an explicit null.
 */
export function mergePatchFor(from: unknown, to: unknown): Json | undefined {
  if (!isObject(from) || !isObject(to))
    return JSON.stringify(from) === JSON.stringify(to) ? undefined : ((to ?? null) as Json);
  const out: Record<string, Json> = {};
  for (const k of Object.keys(from)) if (!(k in to)) out[k] = null;
  for (const [k, v] of Object.entries(to)) {
    const p = k in from ? mergePatchFor(from[k], v) : (v as Json);
    if (p !== undefined) out[k] = p;
  }
  return Object.keys(out).length === 0 ? undefined : out;
}

/** An MPLS label route or tunnel path as one line: `10.5.1.2 via loop5001 push [50017 50018] ×1`. */
export function pathText(p: MplsPathConfig, t: Translate): string {
  const parts: string[] = [];
  if (p.vrf !== undefined) parts.push(t('path.lookup', { vrf: p.vrf }));
  if (p.nextHop !== undefined) parts.push(p.nextHop);
  if (p.interface !== undefined) parts.push(t('path.via', { interface: p.interface }));
  if (p.outLabels.length > 0) parts.push(t('path.push', { labels: p.outLabels.join(' ') }));
  else if (p.vrf === undefined) parts.push(t('path.pop'));
  if (p.weight !== 1) parts.push(`×${p.weight}`);
  return parts.join(' ');
}

export interface LabelRouteRow {
  id: string;
  index: number;
  table: number;
  label: number;
  eos: string;
  payload: string;
  paths: string;
}

export function labelRouteRows(m: MplsConfig | undefined, t: Translate): LabelRouteRow[] {
  return (m?.labelRoutes ?? []).map((r, index) => ({
    id: `${r.table}/${r.label}/${r.eos ? 'eos' : 'neos'}/${index}`,
    index,
    table: r.table,
    label: r.label,
    eos: r.eos ? t('eos.yes') : t('eos.no'),
    payload: r.eos ? (r.payload ?? '') : '',
    paths: r.paths.map((p) => pathText(p, t)).join(' · '),
  }));
}

/** An SR-MPLS policy's segment lists as one line: `[50101 50102]×1 [50103]×2`. */
export function segmentListsText(p: MplsSrPolicyConfig): string {
  return p.segmentLists.map((l) => `[${l.labels.join(' ')}]×${l.weight}`).join(' ');
}

/** Default MPLS table 0 as the first choice of the FIB browser, then the configured tables. */
export function tableChoices(
  m: MplsConfig | undefined,
  live: readonly { tableId: number }[] = [],
): number[] {
  const ids = new Set<number>([
    0,
    ...Object.keys(m?.tables ?? {}).map(Number),
    ...live.map((x) => x.tableId),
  ]);
  return [...ids].sort((a, b) => a - b);
}
