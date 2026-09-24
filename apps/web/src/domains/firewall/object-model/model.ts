import type { paths } from '@ngfw/api-client';
import type {
  AddressGroup,
  AddressObject,
  ObjectsConfig,
  Schedule,
  ServiceGroup,
  ServiceObject,
  Tag,
  Zone,
} from '@ngfw/schema';
import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import { domainSchemas } from '../../../schema/registry';

type Ok<O> = O extends { responses: { 200: { content: { 'application/json': infer T } } } } ? T : never;

/** `GET /api/v1/state/objects/fqdn` and `…/usage` as generated from the OpenAPI document (never hand-written). */
export type FqdnState = Ok<NonNullable<paths['/api/v1/state/objects/fqdn']['get']>>;
export type FqdnItem = FqdnState['items'][number];
export type Usage = Ok<NonNullable<paths['/api/v1/state/objects/usage']['get']>>;
export type UsageRef = Usage['usedBy'][number];

export type { ObjectsConfig };

/** The seven records of `objects`, in tab order. */
export const OBJECT_KINDS = ['addresses', 'addressGroups', 'services', 'serviceGroups', 'schedules', 'zones', 'tags'] as const;
export type ObjectKind = (typeof OBJECT_KINDS)[number];

export interface EntryOf {
  addresses: AddressObject;
  addressGroups: AddressGroup;
  services: ServiceObject;
  serviceGroups: ServiceGroup;
  schedules: Schedule;
  zones: Zone;
  tags: Tag;
}
export type AnyEntry = EntryOf[ObjectKind];

/** Object names: the schema's `objectName` (the record keys). */
export const OBJECT_NAME_RE = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$/;

export function isObjectKind(v: string | null | undefined): v is ObjectKind {
  return (OBJECT_KINDS as readonly string[]).includes(v ?? '');
}

/** `objects.<kind>.<name>` item schema — the one schema (00-CONTEXT rule 5), from the generated domain JSON Schema. */
export function itemSchema(kind: ObjectKind): JsonSchema {
  const rec = (domainSchemas.objects.properties ?? {})[kind] as { additionalProperties?: JsonSchema } | undefined;
  if (!rec?.additionalProperties || typeof rec.additionalProperties !== 'object') throw new Error(`objects.${kind} item schema not found`);
  return rec.additionalProperties;
}

/** Entries of one kind, sorted by name. */
export function entriesOf<K extends ObjectKind>(objects: Partial<ObjectsConfig> | undefined, kind: K): [string, EntryOf[K]][] {
  const rec = (objects?.[kind] ?? {}) as Record<string, EntryOf[K]>;
  return Object.entries(rec).sort(([a], [b]) => a.localeCompare(b));
}

/** Which kinds an object reference may name (D-062: addresses and groups share one namespace, services likewise). */
export const PICKER_KINDS = {
  address: ['addresses', 'addressGroups'],
  service: ['services', 'serviceGroups'],
  schedule: ['schedules'],
  zone: ['zones'],
  tag: ['tags'],
} as const satisfies Record<string, readonly ObjectKind[]>;

const KIND_IN_TEXT = /objects\.(addresses|addressGroups|services|serviceGroups|schedules|zones|tags)\b/g;

/**
 * The kinds an `object-picker` field offers: an explicit `x-vrx-ui.objectKinds` hint, else the kinds its help text names
 * (`objects.addresses or objects.addressGroups`, as the schema writes them), else what its property name says
 * (`zone`, `schedule`, `tags`). A field none of these classify gets **no** kinds (review F4: `acl.attachments[].list`
 * names an ACL list, not an object) and the picker renders the plain field instead of guessing.
 */
export function pickerKinds(hints: { objectKinds?: unknown; help?: unknown; [hint: string]: unknown }, propPath: string): readonly ObjectKind[] {
  if (Array.isArray(hints.objectKinds)) {
    const ks = hints.objectKinds.filter((k): k is ObjectKind => typeof k === 'string' && isObjectKind(k));
    if (ks.length > 0) return ks;
  }
  if (typeof hints.help === 'string') {
    const found = [...hints.help.matchAll(KIND_IN_TEXT)].map((m) => m[1] as ObjectKind);
    if (found.length > 0) return [...new Set(found)];
  }
  const last = propPath.split('.').pop() ?? '';
  if (last === 'zone' || last === 'zones') return PICKER_KINDS.zone;
  if (last === 'schedule') return PICKER_KINDS.schedule;
  if (last === 'tags') return PICKER_KINDS.tag;
  return [];
}

/** A one-line summary of an entry for tables and picker labels (language-neutral: addresses, ports, days). */
export function summary(kind: ObjectKind, e: AnyEntry): string {
  switch (kind) {
    case 'addresses': {
      const a = e as AddressObject;
      if (a.type === 'host') return a.address;
      if (a.type === 'network') return a.prefix;
      if (a.type === 'range') return `${a.start}–${a.end}`;
      return a.fqdn;
    }
    case 'addressGroups':
    case 'serviceGroups':
      return (e as AddressGroup).members.join(', ');
    case 'services': {
      const s = e as ServiceObject;
      if ('destinationPorts' in s) {
        const dst = s.destinationPorts.length > 0 ? s.destinationPorts.join(',') : '*';
        const src = s.sourcePorts.length > 0 ? ` ← ${s.sourcePorts.join(',')}` : '';
        return `${s.protocol}/${dst}${src}`;
      }
      if (s.protocol === 'icmp' || s.protocol === 'icmp6') {
        return [s.protocol, s.type !== undefined ? `type ${s.type}` : '', s.code !== undefined ? `code ${s.code}` : ''].filter(Boolean).join(' ');
      }
      return s.protocol === 'other' ? `ip proto ${s.number}` : 'any';
    }
    case 'schedules': {
      const s = e as Schedule;
      return s.type === 'recurring' ? `${s.days.join(',')} ${s.start}–${s.end}` : `${s.start} → ${s.end}`;
    }
    case 'zones':
      return (e as Zone).interfaces.join(', ');
    case 'tags':
      return (e as Tag).color ?? '';
  }
}

/** The discriminator of an entry (address `type`, service `protocol`, schedule `type`); '' for kinds without one. */
export function variantOf(kind: ObjectKind, e: AnyEntry): string {
  if (kind === 'addresses' || kind === 'schedules') return (e as AddressObject | Schedule).type;
  if (kind === 'services') return (e as ServiceObject).protocol;
  return '';
}

/** Tags of an entry (tags themselves have none). */
export function tagsOf(e: AnyEntry): string[] {
  return 'tags' in e && Array.isArray(e.tags) ? e.tags : [];
}

type Translate = (key: string, opts?: Record<string, unknown>) => string;

/**
 * Titles, help and variant names in the UI language (`object-model:field.<prop>.title|help`, `variant.<value>`), the
 * schema's English text as fallback, recursively through unions and nested objects. `object-picker` fields keep the
 * kinds their original help names as `x-vrx-ui.objectKinds` (the translated help may not name them).
 */
export function localizeSchema(schema: JsonSchema, t: Translate, scope = ''): JsonSchema {
  const hints = (schema['x-vrx-ui'] ?? {}) as Record<string, unknown>;
  const out: JsonSchema = { ...schema };
  if (schema.properties) {
    const props: Record<string, JsonSchema> = {};
    for (const [name, prop] of Object.entries(schema.properties)) {
      const ph = (prop['x-vrx-ui'] ?? {}) as Record<string, unknown>;
      // `field.<scope>.<prop>` (e.g. field.addresses.start = "First address") before the shared `field.<prop>`
      const text = (what: 'title' | 'help', fallback: string) =>
        t(`field.${scope}.${name}.${what}`, { defaultValue: t(`field.${name}.${what}`, { defaultValue: fallback }) });
      const help = text('help', '');
      const extra: Record<string, unknown> = {};
      if (ph.widget === 'object-picker' || ph.widget === 'tag-picker') {
        const kinds = pickerKinds(ph, name);
        if (kinds.length > 0) extra.objectKinds = kinds;
      }
      props[name] = localizeSchema(
        {
          ...prop,
          ...(prop.const === undefined ? { title: text('title', prop.title ?? name) } : {}),
          'x-vrx-ui': { ...ph, ...extra, ...(help ? { help } : {}) },
        } as JsonSchema,
        t,
        scope,
      );
    }
    out.properties = props;
  }
  for (const key of ['oneOf', 'anyOf'] as const) {
    const vs = schema[key];
    if (!vs) continue;
    out[key] = vs.map((v) => {
      const lv = localizeSchema(v, t, scope);
      const disc = variantValues(v);
      return disc ? { ...lv, title: t(`variant.${disc.join('|')}`, { defaultValue: disc.join(' / ') }) } : lv;
    });
  }
  if (schema.items && typeof schema.items === 'object') out.items = localizeSchema(schema.items, t, scope);
  if (Object.keys(hints).length > 0) out['x-vrx-ui'] = hints;
  return out;
}

/** The discriminator values of a union variant: its `const` property, or a small `enum` one (`protocol: tcp|tcp-udp`). */
function variantValues(v: JsonSchema): string[] | undefined {
  for (const [name, p] of Object.entries(v.properties ?? {})) {
    if (p.const !== undefined) return [String(p.const)];
    if ((name === 'protocol' || name === 'type' || name === 'kind') && Array.isArray(p.enum)) return p.enum.map(String);
  }
  return undefined;
}

/**
 * RFC 7386 merge patch that turns `from` into `to` (removed members → null, objects recurse, arrays and scalars are
 * replaced). An object edit is sent as `PATCH /api/v1/config/objects {<kind>: {<name>: patch}}`.
 */
export function mergePatch(from: unknown, to: unknown): unknown {
  if (!isObject(from) || !isObject(to)) return to === undefined ? null : to;
  const patch: Record<string, unknown> = {};
  for (const k of Object.keys(from)) if (!(k in to) || to[k] === undefined) patch[k] = null;
  for (const [k, v] of Object.entries(to)) {
    if (v === undefined) continue;
    const prev = from[k];
    if (isObject(prev) && isObject(v)) {
      const inner = mergePatch(prev, v) as Record<string, unknown>;
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

/** Two entries are the same configuration (pending-change marks). */
export function sameEntry(a: unknown, b: unknown): boolean {
  if (a === undefined || b === undefined) return a === b;
  const p = mergePatch(a, b);
  return isObject(p) && Object.keys(p).length === 0;
}

/** Readable text colour on a tag colour (`#rrggbb`), by relative luminance. */
export function contrastText(hex: string | undefined): string | undefined {
  if (!hex || !/^#[0-9a-fA-F]{6}$/.test(hex)) return undefined;
  const [r, g, b] = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255);
  const lin = (c: number) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4);
  const l = 0.2126 * lin(r!) + 0.7152 * lin(g!) + 0.0722 * lin(b!);
  return l > 0.4 ? '#000000' : '#ffffff';
}
