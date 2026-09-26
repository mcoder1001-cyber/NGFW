import type { paths } from '@ngfw/api-client';
import type {
  AclConfig,
  AddressMatch,
  HostAclSettings,
  HostAttachment,
  HostList,
  HostRule,
  ServiceMatch,
} from '@ngfw/schema';
import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import { domainSchemas } from '../../../schema/registry';

type Ok<O> = O extends { responses: { 200: { content: { 'application/json': infer T } } } }
  ? T
  : never;

/** `GET /api/v1/state/host-acl` as generated from the OpenAPI document (never hand-written). */
export type HostAclState = Ok<NonNullable<paths['/api/v1/state/host-acl']['get']>>;
export type HostAclChain = HostAclState['chains'][number];
export type HostAclKernelRule = HostAclChain['rules'][number];
export type HostAclRuleCounters = HostAclState['rules'][number];

export type { AclConfig, HostAclSettings, HostAttachment, HostList, HostRule };
/** The part of `acl` this screen edits (F-acl owns lists/macip/attachments). */
export type HostAclConfig = Pick<Partial<AclConfig>, 'host' | 'hostAttachments' | 'hostSettings'>;

export const TABS = ['lists', 'attachments', 'settings', 'rendered'] as const;
export type HostAclTab = (typeof TABS)[number];
export function isTab(v: string | null | undefined): v is HostAclTab {
  return (TABS as readonly string[]).includes(v ?? '');
}

/** Table columns (i18n keys `col.<c>`, `rendered.col.<c>`, `rendered.setCol.<c>`). */
export const RULE_COLUMNS = [
  'sequence',
  'action',
  'ipVersion',
  'source',
  'destination',
  'service',
  'interface',
  'log',
  'enabled',
  'description',
  'packets',
  'bytes',
] as const;
export const ATTACHMENT_COLUMNS = ['list', 'chain', 'priority', 'enabled', 'description'] as const;
export const RENDERED_COLUMNS = ['kind', 'rule', 'verdict', 'packets', 'bytes'] as const;
export const SET_COLUMNS = ['name', 'object', 'type', 'elements'] as const;

/** Host list names: the schema's `objectName` (the record keys). */
export const LIST_NAME_RE = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$/;

/** The defaults of `acl.hostSettings` (the schema's; absent = all of them). */
export const DEFAULT_SETTINGS: HostAclSettings = {
  defaultInput: 'accept',
  allowIcmp: true,
  antiLockout: { enabled: true, sources: [], interfaces: [], ports: [22, 443] },
};

export function settingsOf(acl: HostAclConfig | undefined): HostAclSettings {
  const s = acl?.hostSettings;
  if (!s) return DEFAULT_SETTINGS;
  return {
    ...DEFAULT_SETTINGS,
    ...s,
    antiLockout: { ...DEFAULT_SETTINGS.antiLockout, ...s.antiLockout },
  };
}

// ---------------------------------------------------------------------------------------------------------------------
// Schemas (the one schema, 00-CONTEXT rule 5: sub-schemas of the generated `acl` JSON Schema)
// ---------------------------------------------------------------------------------------------------------------------

function prop(schema: JsonSchema | undefined, name: string): JsonSchema {
  const p = schema?.properties?.[name];
  if (!p || typeof p !== 'object') throw new Error(`acl schema: property ${name} not found`);
  return p;
}

function aclSchema(): JsonSchema {
  return domainSchemas.acl;
}

/** `acl.host.<name>` without `rules` (rules are edited one by one). */
export function listSchema(): JsonSchema {
  const host = prop(aclSchema(), 'host').additionalProperties as JsonSchema;
  const properties = { ...(host.properties ?? {}) };
  delete properties['rules'];
  return { ...host, properties, required: (host.required ?? []).filter((r) => r !== 'rules') };
}

/** One `acl.host.<name>.rules[]` item. */
export function ruleSchema(): JsonSchema {
  const host = prop(aclSchema(), 'host').additionalProperties as JsonSchema;
  return prop(host, 'rules').items as JsonSchema;
}

/** One `acl.hostAttachments[]` item. */
export function attachmentSchema(): JsonSchema {
  return prop(aclSchema(), 'hostAttachments').items as JsonSchema;
}

/** `acl.hostSettings`. */
export function settingsSchema(): JsonSchema {
  return prop(aclSchema(), 'hostSettings');
}

type Translate = (key: string, opts?: Record<string, unknown>) => string;

/**
 * Titles and help in the UI language, the schema's English text as fallback, recursively through nested objects, unions
 * and array items: `host-acl-nftables:field.<parent>.<prop>.title|help` (e.g. `field.antiLockout.enabled`) before the
 * shared `field.<prop>.title|help`. An empty translated help removes the schema's (English) help. `object-picker` fields
 * keep the kinds their original help names as `x-vrx-ui.objectKinds` (the translated help may not name them).
 */
export function localizeSchema(schema: JsonSchema, t: Translate, scope = ''): JsonSchema {
  const out: JsonSchema = { ...schema };
  if (schema.properties) {
    const props: Record<string, JsonSchema> = {};
    for (const [name, p] of Object.entries(schema.properties)) {
      const hints = { ...((p['x-vrx-ui'] ?? {}) as Record<string, unknown>) };
      const text = (what: 'title' | 'help', fallback: string): string =>
        t(`field.${scope}${name}.${what}`, {
          defaultValue: t(`field.${name}.${what}`, { defaultValue: fallback }),
        });
      if (hints.widget === 'object-picker' && typeof hints.help === 'string') {
        const kinds = [
          ...hints.help.matchAll(/objects\.(addresses|addressGroups|services|serviceGroups)\b/g),
        ].map((m) => m[1]);
        if (kinds.length > 0) hints.objectKinds = [...new Set(kinds)];
      }
      const help = text('help', typeof hints.help === 'string' ? hints.help : '');
      if (help) hints.help = help;
      else delete hints.help;
      props[name] = localizeSchema(
        {
          ...p,
          ...(p.const === undefined ? { title: text('title', p.title ?? name) } : {}),
          'x-vrx-ui': hints,
        } as JsonSchema,
        t,
        p.type === 'object' ? `${scope}${name}.` : scope,
      );
    }
    out.properties = props;
  }
  for (const key of ['oneOf', 'anyOf'] as const) {
    const vs = schema[key];
    if (!vs) continue;
    out[key] = vs.map((v) => {
      const lv = localizeSchema(v, t, scope);
      const kind = v.properties?.['kind']?.const;
      return kind !== undefined
        ? { ...lv, title: t(`match.${String(kind)}`, { defaultValue: String(kind) }) }
        : lv;
    });
  }
  if (schema.items && typeof schema.items === 'object')
    out.items = localizeSchema(schema.items, t, scope);
  return out;
}

// ---------------------------------------------------------------------------------------------------------------------
// Edits (RFC 7386 merge patches of the candidate's `acl`)
// ---------------------------------------------------------------------------------------------------------------------

/** Lists sorted by name. */
export function listsOf(acl: HostAclConfig | undefined): [string, HostList][] {
  return Object.entries(acl?.host ?? {}).sort(([a], [b]) => a.localeCompare(b));
}

/** Rules with their index in the document (pointers), sorted by sequence. */
export function rulesOf(list: HostList | undefined): { rule: HostRule; index: number }[] {
  return (list?.rules ?? [])
    .map((rule, index) => ({ rule, index }))
    .sort((a, b) => a.rule.sequence - b.rule.sequence);
}

/** The rules array with `rule` put at `index` (or appended), kept in sequence order. */
export function putRule(
  rules: readonly HostRule[],
  index: number | undefined,
  rule: HostRule,
): HostRule[] {
  const next = [...rules];
  if (index === undefined || index < 0 || index >= next.length) next.push(rule);
  else next[index] = rule;
  return next.sort((a, b) => a.sequence - b.sequence);
}

export function removeAt<T>(items: readonly T[], index: number): T[] {
  return items.filter((_x, i) => i !== index);
}

/** The next free sequence (last + 10, first 10). */
export function nextSequence(rules: readonly HostRule[]): number {
  return rules.reduce((m, r) => Math.max(m, r.sequence), 0) + 10;
}

/** A merge patch that deletes one list and its attachments. */
export function deleteListPatch(
  acl: HostAclConfig | undefined,
  name: string,
): Record<string, unknown> {
  const attachments = acl?.hostAttachments ?? [];
  const kept = attachments.filter((a) => a.list !== name);
  return {
    host: { [name]: null },
    ...(kept.length !== attachments.length ? { hostAttachments: kept } : {}),
  };
}

// ---------------------------------------------------------------------------------------------------------------------
// Display
// ---------------------------------------------------------------------------------------------------------------------

/** One-line text of an address match (language-neutral: prefixes and object names). */
export function matchText(m: AddressMatch | undefined): string {
  if (!m || m.kind === 'any') return '*';
  return m.kind === 'prefix' ? m.prefix : m.name;
}

/** One-line text of a service match. */
export function serviceText(m: ServiceMatch | undefined): string {
  if (!m || m.kind === 'any') return '*';
  if (m.kind === 'object') return m.name;
  const s = m.spec;
  if ('destinationPorts' in s) {
    const dst = s.destinationPorts.length > 0 ? s.destinationPorts.join(',') : '*';
    const src = s.sourcePorts.length > 0 ? ` ← ${s.sourcePorts.join(',')}` : '';
    return `${s.protocol}/${dst}${src}`;
  }
  if (s.protocol === 'icmp' || s.protocol === 'icmp6') {
    return [
      s.protocol,
      s.type !== undefined ? `type ${s.type}` : '',
      s.code !== undefined ? `code ${s.code}` : '',
    ]
      .filter(Boolean)
      .join(' ');
  }
  return s.protocol === 'other' ? `ip proto ${s.number}` : 'any';
}

/** Kernel counters per configuration rule: `<list>\0<sequence>` → aggregate. */
export function countersByRule(state: HostAclState | undefined): Map<string, HostAclRuleCounters> {
  return new Map((state?.rules ?? []).map((r) => [ruleKey(r.list, r.sequence), r]));
}

export function ruleKey(list: string, sequence: number): string {
  return `${list}\u0000${sequence}`;
}

/** A uint64 decimal string with thousands separators (exact; no Number rounding above 2^53). */
export function groupDigits(value: string): string {
  if (!/^\d+$/.test(value)) return value;
  return value.replace(/\B(?=(\d{3})+(?!\d))/g, ',');
}

export type BannerSeverity = 'info' | 'warning' | 'error';

export interface Banner {
  severity: BannerSeverity;
  /** i18n key in the host-acl-nftables namespace. */
  key: string;
  params: Record<string, string>;
}

/**
 * The anti-lockout banner: info while the anti-lockout rule is on, a warning while it is off, and an error when the live
 * table is out of sync with what was applied or missing while enabled attachments exist (the check mode never loads it).
 */
export function banners(
  settings: HostAclSettings,
  state: HostAclState | undefined,
  running: HostAclConfig | undefined,
): Banner[] {
  const lock = settings.antiLockout;
  const params = {
    ports: lock.ports.join(', '),
    sources: lock.sources.join(', '),
    interfaces: lock.interfaces.join(', '),
  };
  const out: Banner[] = [];
  if (lock.enabled) {
    const who = `${lock.sources.length > 0 ? 'Sources' : 'AnySource'}${lock.interfaces.length > 0 ? 'Ifaces' : 'AnyIface'}`;
    out.push({ severity: 'info', key: `banner.enabled${who}`, params });
  } else {
    out.push({ severity: 'warning', key: 'banner.disabled', params });
  }
  if (state) {
    const attached = (running?.hostAttachments ?? []).some((a) => a.enabled !== false);
    if (!state.inSync)
      out.push({ severity: 'error', key: 'banner.drift', params: { table: state.table } });
    else if (state.mode !== 'check' && attached && !state.present)
      out.push({ severity: 'error', key: 'banner.missing', params: { table: state.table } });
    if (state.mode === 'check')
      out.push({ severity: 'info', key: 'banner.checkMode', params: { table: state.table } });
  }
  return out;
}

/** Two values are the same configuration (pending marks; key order of objects does not matter). */
export function same(a: unknown, b: unknown): boolean {
  return canonical(a) === canonical(b);
}

function canonical(v: unknown): string {
  return JSON.stringify(v, (_k, x: unknown) =>
    x && typeof x === 'object' && !Array.isArray(x)
      ? Object.fromEntries(
          Object.entries(x as Record<string, unknown>).sort(([p], [q]) => p.localeCompare(q)),
        )
      : x,
  );
}
