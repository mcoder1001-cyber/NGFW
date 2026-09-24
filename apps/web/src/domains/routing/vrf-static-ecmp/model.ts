import type { StaticRouteConfig, VrfConfig } from '@ngfw/schema';
import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import { domainSchemas } from '../../../schema/registry';

/** i18n namespace of the feature (= task slug, wave-A-hotspots §0 rule 5). */
export const NS = 'vrf-static-ecmp';

export type VrfsConfig = Record<string, VrfConfig>;
export type { StaticRouteConfig, VrfConfig };

/** `vrfs.<name>` item schema — the one schema (00-CONTEXT rule 5), from the generated domain JSON Schema. */
export function vrfItemSchema(): JsonSchema {
  const s = domainSchemas.vrfs as { additionalProperties?: JsonSchema };
  if (!s.additionalProperties || typeof s.additionalProperties !== 'object') throw new Error('vrfs item schema not found');
  return s.additionalProperties;
}

/** `routing.static[]` item schema. */
export function staticRouteSchema(): JsonSchema {
  const props = (domainSchemas.routing.properties ?? {}) as Record<string, JsonSchema>;
  const items = (props['static'] as { items?: JsonSchema } | undefined)?.items;
  if (!items || typeof items !== 'object') throw new Error('routing.static item schema not found');
  return items;
}

type Translate = (key: string, opts?: Record<string, unknown>) => string;

/**
 * Titles and help in the UI language (`vrf-static-ecmp:field.<name>.title|help`, nested fields as `<parent>.<name>`); the
 * schema's English text stays the fallback. Walks objects and array items.
 */
export function localizeSchema(schema: JsonSchema, t: Translate, prefix = ''): JsonSchema {
  const out: Record<string, unknown> = { ...schema };
  const props = schema.properties as Record<string, JsonSchema> | undefined;
  if (props) {
    const next: Record<string, JsonSchema> = {};
    for (const [name, prop] of Object.entries(props)) {
      const key = prefix + name;
      const hints = (prop['x-vrx-ui'] ?? {}) as Record<string, unknown>;
      const help = t(`field.${key}.help`, { defaultValue: '' });
      const loc = localizeSchema(prop, t, `${key}.`);
      next[name] = {
        ...loc,
        title: t(`field.${key}.title`, { defaultValue: prop.title ?? name }),
        'x-vrx-ui': { ...hints, ...(help ? { help } : {}), ...(typeof hints.group === 'string' ? { group: t(`group.${hints.group}`, { defaultValue: hints.group }) } : {}) },
      } as JsonSchema;
    }
    out['properties'] = next;
  }
  const items = schema.items as JsonSchema | undefined;
  if (items && typeof items === 'object' && !Array.isArray(items)) out['items'] = localizeSchema(items, t, prefix);
  return out as JsonSchema;
}

/** VRF grid row. */
export interface VrfRow {
  id: string;
  name: string;
  tableId: number;
  description: string;
  sourceSelect: number;
}

export function vrfRows(vrfs: VrfsConfig | undefined): VrfRow[] {
  const rows = Object.entries(vrfs ?? {}).map(([name, v]) => ({
    id: name,
    name,
    tableId: v.id,
    description: v.description ?? '',
    sourceSelect: v.sourceSelect?.length ?? 0,
  }));
  if (!rows.some((r) => r.name === 'default')) rows.push({ id: 'default', name: 'default', tableId: 0, description: '', sourceSelect: 0 });
  return rows.sort((a, b) => a.tableId - b.tableId || a.name.localeCompare(b.name));
}

/** Static-route grid row; `index` is the position in `routing.static` (the pointer). */
export interface RouteRow {
  id: string;
  index: number;
  vrf: string;
  prefix: string;
  paths: string;
  ecmp: boolean;
  distance: number;
  viaFrr: boolean;
  description: string;
}

/** "10.2.1.2 ×3 · 10.2.1.3 ×1", "via lan", "10.2.2.2 in default", or "" for a blackhole route. */
export function pathsText(r: Pick<StaticRouteConfig, 'nextHops'>): string {
  const ecmp = r.nextHops.length > 1;
  return r.nextHops
    .map((h) => {
      const where = [h.address, h.interface ? `via ${h.interface}` : '', h.vrf ? `in ${h.vrf}` : ''].filter(Boolean).join(' ');
      return ecmp ? `${where} ×${h.weight}` : where;
    })
    .join(' · ');
}

export function routeRows(routes: readonly StaticRouteConfig[] | undefined): RouteRow[] {
  return (routes ?? []).map((r, index) => ({
    id: String(index),
    index,
    vrf: r.vrf,
    prefix: r.prefix,
    paths: r.blackhole ? '' : pathsText(r),
    ecmp: r.nextHops.length > 1,
    distance: r.distance,
    viaFrr: r.viaFrr === true,
    description: r.description ?? '',
  }));
}

/** Canonical prefix text for matching against the FIB ("10.2.0.0/24" as VPP prints it; IPv6 lower-case). */
export function prefixKey(p: string): string {
  return p.trim().toLowerCase();
}

/** Status of a configured static route against the live FIB (the API-source entries of its VRF). */
export type RouteStatus = 'installed' | 'missing' | 'frr' | 'unknown';

export function routeStatus(row: RouteRow, installed: ReadonlyMap<string, ReadonlySet<string>> | undefined): RouteStatus {
  if (row.viaFrr) return 'frr';
  const set = installed?.get(row.vrf);
  if (!set) return 'unknown';
  return set.has(prefixKey(row.prefix)) ? 'installed' : 'missing';
}
