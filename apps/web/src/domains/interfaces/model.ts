import type { paths } from '@ngfw/api-client';
import type { InterfaceConfig, SubinterfaceConfig } from '@ngfw/schema';
import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import type { VrxStatus } from '@ngfw/ui-kit';
import { domainSchemas } from '../../schema/registry';

type Ok<O> = O extends { responses: { 200: { content: { 'application/json': infer T } } } } ? T : never;

/** `GET /api/v1/state/interfaces` as generated from the OpenAPI document (never hand-written). */
export type InterfacesState = Ok<NonNullable<paths['/api/v1/state/interfaces']['get']>>;
export type InterfaceItem = InterfacesState['items'][number];
export type LiveState = NonNullable<InterfaceItem['state']>;
export type Counters = NonNullable<InterfaceItem['counters']>;

/** The configured interfaces map as the candidate holds it (`interfaces.<name>`). */
export type InterfacesConfig = Record<string, InterfaceConfig>;
export type { InterfaceConfig, SubinterfaceConfig };

/** `interfaces.<name>` item schema — the one schema (00-CONTEXT rule 5), from the generated domain JSON Schema. */
export function interfaceItemSchema(): JsonSchema {
  const s = domainSchemas.interfaces as { additionalProperties?: JsonSchema };
  if (!s.additionalProperties || typeof s.additionalProperties !== 'object') throw new Error('interfaces item schema not found');
  return s.additionalProperties;
}

/** The interface form edits everything but `subinterfaces` (they have their own table in the drawer). */
export function interfaceFormSchema(): JsonSchema {
  const item = interfaceItemSchema();
  const props = { ...((item.properties ?? {}) as Record<string, JsonSchema>) };
  delete props['subinterfaces'];
  delete props['bond']; // F-bonding: edited on the Bonds page; its required `mode` would block every save here (phantom default)
  const required = Array.isArray(item.required) ? item.required.filter((r) => r !== 'subinterfaces') : undefined;
  return { ...item, properties: props, ...(required ? { required } : {}) } as JsonSchema;
}

/** `interfaces.<name>.subinterfaces.<id>` item schema. */
export function subinterfaceSchema(): JsonSchema {
  const props = (interfaceItemSchema().properties ?? {}) as Record<string, JsonSchema>;
  const sub = props['subinterfaces'] as { additionalProperties?: JsonSchema } | undefined;
  if (!sub?.additionalProperties || typeof sub.additionalProperties !== 'object') throw new Error('subinterface schema not found');
  return sub.additionalProperties;
}

type Translate = (key: string, opts?: Record<string, unknown>) => string;

/** Titles and help in the UI language (`interfaces:field.<name>.title|help`); the schema's English text stays the fallback. */
export function localizeSchema(schema: JsonSchema, t: Translate): JsonSchema {
  const props = (schema.properties ?? {}) as Record<string, JsonSchema>;
  const out: Record<string, JsonSchema> = {};
  for (const [name, prop] of Object.entries(props)) {
    const hints = (prop['x-vrx-ui'] ?? {}) as Record<string, unknown>;
    const help = t(`field.${name}.help`, { defaultValue: '' });
    out[name] = {
      ...prop,
      title: t(`field.${name}.title`, { defaultValue: prop.title ?? name }),
      // group names are fieldset titles: translate them too (the same key for every member keeps the grouping)
      'x-vrx-ui': { ...hints, ...(help ? { help } : {}), ...(typeof hints.group === 'string' ? { group: t(`group.${hints.group}`, { defaultValue: hints.group }) } : {}) },
    } as JsonSchema;
  }
  return { ...schema, properties: out } as JsonSchema;
}

/**
 * RFC 7386 merge patch that turns `from` into `to`: removed members become `null`, objects recurse, everything else
 * (arrays, scalars) is replaced. Edits go through `PATCH /api/v1/config/interfaces` so that interface names with `/`
 * (`GigabitEthernet0/8/0`) never have to be squeezed into one URL path segment.
 */
export function createMergePatch(from: unknown, to: unknown): unknown {
  if (!isObject(from) || !isObject(to)) return to === undefined ? null : to;
  const patch: Record<string, unknown> = {};
  for (const k of Object.keys(from)) if (!(k in to) || to[k] === undefined) patch[k] = null;
  for (const [k, v] of Object.entries(to)) {
    if (v === undefined) continue;
    const prev = from[k];
    if (isObject(prev) && isObject(v)) {
      const inner = createMergePatch(prev, v) as Record<string, unknown>;
      if (Object.keys(inner).length > 0) patch[k] = inner;
    } else if (JSON.stringify(prev) !== JSON.stringify(v)) {
      patch[k] = v;
    }
  }
  return patch;
}

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

/** Semantic status of the admin state. */
export function adminStatus(s: LiveState | null | undefined): VrxStatus | undefined {
  if (!s) return undefined;
  return s.adminUp ? 'up' : 'adminDown';
}

/** Semantic status of the link: an admin-down interface is `adminDown`, not a failure. */
export function linkStatus(s: LiveState | null | undefined): VrxStatus | undefined {
  if (!s) return undefined;
  if (!s.adminUp) return 'adminDown';
  return s.linkUp ? 'up' : 'down';
}

/** Addresses to show: live when VPP has the interface, else the configured (running) ones. */
export function addressesOf(it: InterfaceItem): string[] {
  if (it.state) return [...it.state.ipv4, ...it.state.ipv6];
  const c = (it.running ?? {}) as { ipv4?: string[]; ipv6?: string[] };
  return [...(c.ipv4 ?? []), ...(c.ipv6 ?? [])];
}

/**
 * @deprecated WEB-1: presence toggle replaced it; returns input unchanged
 *
 * SchemaForm keeps an optional object absent until its presence switch is on, so there is no phantom to drop, and
 * dropping "default-only" objects would delete one the user just enabled (WEB-1 review H1). Kept only so that branches
 * importing it keep compiling (WEB-1 verify C1); remove the calls, then this, in a later TD.
 */
export function dropPhantomOptionals(_schema: JsonSchema, _before: unknown, after: unknown): unknown {
  return after;
}
