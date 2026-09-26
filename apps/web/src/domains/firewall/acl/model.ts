import type { paths } from '@ngfw/api-client';
import type {
  AclAttachment,
  AclRule,
  AddressMatch,
  MacipAttachment,
  MacipList,
  ServiceMatch,
} from '@ngfw/schema';
import type { Formatters } from '@ngfw/ui-kit';
import type { ServerDataGridProps } from '@ngfw/ui-kit/data-grid';
import type { JsonSchema, ProblemDetails } from '@ngfw/ui-kit/schema-form';
import { ApiError } from '../../../api-problem';
import { domainSchemas } from '../../../schema/registry';
import { pickerKinds, summary } from '../object-model';

type Ok<O> = O extends { responses: { 200: { content: { 'application/json': infer T } } } }
  ? T
  : never;

/** F-acl API answers as generated from the OpenAPI document (never hand-written). */
export type AclListsState = Ok<NonNullable<paths['/api/v1/state/acl/lists']['get']>>;
export type AclListItem = AclListsState['lists'][number];
export type AclListLive = NonNullable<AclListItem['live']>;
export type MacipListItem = AclListsState['macip'][number];
export type RulesPage = Ok<NonNullable<paths['/api/v1/state/acl/lists/{name}/rules']['get']>>;
export type RuleItem = RulesPage['items'][number];
export type RuleLive = NonNullable<RuleItem['live']>;
export type RulesQuery = NonNullable<
  NonNullable<paths['/api/v1/state/acl/lists/{name}/rules']['get']>['parameters']['query']
>;
export type AttachmentsState = Ok<NonNullable<paths['/api/v1/state/acl/attachments']['get']>>;
export type LiveBinding = AttachmentsState['interfaces'][number];
export type BoundAcl = LiveBinding['input'][number];
export type ImportResult = Ok<NonNullable<paths['/api/v1/actions/acl/import']['post']>>;
export type ImportMode = NonNullable<
  NonNullable<paths['/api/v1/actions/acl/import']['post']>['parameters']['query']
>['mode'];
export type BulkBody = NonNullable<
  paths['/api/v1/actions/acl/lists/{name}/rules/bulk']['post']
>['requestBody']['content']['application/json'];
export type BulkResult = Ok<
  NonNullable<paths['/api/v1/actions/acl/lists/{name}/rules/bulk']['post']>
>;
export type RuleSource = 'candidate' | 'running';

export type { AclAttachment, AclRule, MacipAttachment, MacipList };

/** Tabs of the ACL page (`?tab=`), in display order. */
export const ACL_TABS = ['lists', 'rules', 'attachments', 'macip'] as const;
export type AclTab = (typeof ACL_TABS)[number];

export function isAclTab(v: string | null | undefined): v is AclTab {
  return (ACL_TABS as readonly string[]).includes(v ?? '');
}

/** D-132: nothing that walks VPP (AclState) is polled faster than every 30 s; a Refresh button does the rest. */
export const ACL_POLL_MS = 30_000;
/** Rule editor page sizes: the server caps a page at 1000 rules, so a 100 000-rule list is never loaded whole. */
export const RULE_PAGE_SIZES = [25, 50, 100, 250, 500, 1000];
export const DEFAULT_RULE_PAGE_SIZE = 100;
/** List names are the schema's `objectName` (the record keys). */
export const LIST_NAME_RE = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$/;
/** Sequence range of the schema (`ruleSequence`). */
export const MAX_SEQUENCE = 2_147_483_647;

// ---------------------------------------------------------------------------------------------------------------
// Rule rows
// ---------------------------------------------------------------------------------------------------------------

/** One grid row: a configuration rule at its document position (`/acl/lists/<list>/rules/<index>`). */
export interface RuleRow {
  /** The row id is the document index: sequences may collide in a candidate that is being edited. */
  id: number;
  index: number;
  sequence: number;
  rule: AclRule;
  pending: RuleItem['pending'];
  live: RuleItem['live'];
}

export function toRuleRow(item: RuleItem): RuleRow {
  return {
    id: item.index,
    index: item.index,
    sequence: item.sequence,
    rule: item.rule as unknown as AclRule,
    pending: item.pending,
    live: item.live,
  };
}

/** `?page` of the API is 1-based, the grid's is 0-based; the quick filter's words are sent as one `filter`. */
export function rulesQuery(
  req: { page: number; pageSize: number; quickFilter: readonly string[] },
  source: RuleSource,
  hitsOnly: boolean,
): RulesQuery {
  const filter = req.quickFilter.join(' ').trim().slice(0, 200);
  return {
    page: req.page + 1,
    pageSize: Math.min(req.pageSize, 1000),
    source,
    ...(filter ? { filter } : {}),
    ...(hitsOnly ? { hitsOnly: true } : {}),
  };
}

/** The grid's selection model and row ids (MUI X types through ui-kit: the web app does not depend on MUI X itself). */
export type SelectionModel = NonNullable<ServerDataGridProps<RuleRow>['rowSelectionModel']>;
export type GridRowId = SelectionModel['ids'] extends Set<infer I> ? I : never;
export const emptySelection = (): SelectionModel => ({ type: 'include', ids: new Set() });

/** Grid ids of the selection. An `exclude` model ("all but these") is resolved against the rows on the page. */
export function selectedIds(model: SelectionModel, pageIds: readonly GridRowId[]): GridRowId[] {
  if (model.type === 'include') return [...model.ids];
  return pageIds.filter((id) => !model.ids.has(id));
}

/** Suggested sequence of a new rule: the next multiple of `step` after the last rule. */
export function nextSequence(last: number | undefined, step = 10): number {
  if (last === undefined || last < 1) return step;
  return Math.min(MAX_SEQUENCE, (Math.floor(last / step) + 1) * step);
}

// ---------------------------------------------------------------------------------------------------------------
// Drag and drop
// ---------------------------------------------------------------------------------------------------------------

export type DropPlan =
  | { kind: 'none' }
  /** A free sequence exists between the target and the rule before it: PATCH the dragged rule's sequence. */
  | { kind: 'patch'; index: number; sequence: number }
  /** No room: move the dragged rule to the target's sequence (the target and the rules after it shift up). */
  | { kind: 'move'; sequences: number[]; to: number };

/**
 * Where a rule dropped onto another row goes: directly before the target. `predecessor` is the sequence of the rule
 * shown right before the target (0 when the target is the first rule of the list, `null` when unknown — first row of
 * a later page); `contiguous` is false when a filter hides rules, so the rows on the page are not neighbours.
 */
export function planDrop(
  dragged: { index: number; sequence: number },
  target: { sequence: number },
  predecessor: number | null,
  contiguous: boolean,
): DropPlan {
  if (dragged.sequence === target.sequence) return { kind: 'none' };
  if (contiguous && predecessor !== null) {
    if (predecessor === dragged.sequence) return { kind: 'none' };
    if (target.sequence - predecessor >= 2) {
      return {
        kind: 'patch',
        index: dragged.index,
        sequence: predecessor + Math.floor((target.sequence - predecessor) / 2),
      };
    }
  }
  return { kind: 'move', sequences: [dragged.sequence], to: target.sequence };
}

// ---------------------------------------------------------------------------------------------------------------
// Formatting
// ---------------------------------------------------------------------------------------------------------------

const BYTE_UNITS = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB'] as const;
export type ByteUnit = (typeof BYTE_UNITS)[number];

/** Bytes scaled to a binary unit (1024). */
export function scaleBytes(bytes: number): { value: number; unit: ByteUnit } {
  let value = bytes;
  let i = 0;
  while (Math.abs(value) >= 1024 && i < BYTE_UNITS.length - 1) {
    value /= 1024;
    i += 1;
  }
  return { value, unit: BYTE_UNITS[i]! };
}

/** A plain translate function (`useAclT()` wraps react-i18next's `t` into this shape). */
export type Translate = (key: string, opts?: Record<string, unknown>) => string;

/** Human-readable bytes in the UI language (digits by the formatter: Persian when the user asked for them). */
export function formatBytes(bytes: number, fmt: Pick<Formatters, 'number'>, t: Translate): string {
  const { value, unit } = scaleBytes(bytes);
  const digits = unit === 'B' ? 0 : value < 10 ? 2 : value < 100 ? 1 : 0;
  return t('unit.bytes', {
    value: fmt.number(value, { maximumFractionDigits: digits }),
    unit: t(`unit.${unit}`),
  });
}

/**
 * The counters columns of a rule: `null` means "—" (counters unavailable on this agent, or the rule is not in VPP:
 * not committed, disabled, inactive schedule); otherwise its packets and bytes.
 */
export function ruleCounters(
  live: RuleItem['live'],
  countersAvailable: boolean,
): { packets: number; bytes: number } | null {
  if (!countersAvailable || !live) return null;
  return { packets: live.packets, bytes: live.bytes };
}

/** Source/destination of a rule as text: `null` = any (the caller translates it). */
export function addressText(m: AddressMatch | undefined): string | null {
  if (!m || m.kind === 'any') return null;
  return m.kind === 'prefix' ? m.prefix : m.name;
}

/** Service of a rule as text (language-neutral: protocol/ports); `null` = any. */
export function serviceText(m: ServiceMatch | undefined): string | null {
  if (!m || m.kind === 'any') return null;
  if (m.kind === 'object') return m.name;
  if (m.spec.protocol === 'any') return null;
  return summary('services', { ...m.spec, tags: [] } as Parameters<typeof summary>[1]);
}

/** Where an attachment applies, as text (`host-w3l0`, `zone lan`). */
export function targetText(
  target: { kind: string; interface?: string; zone?: string },
  t: Translate,
): string {
  return target.kind === 'zone'
    ? t('target.zone', { zone: target.zone ?? '' })
    : (target.interface ?? '');
}

/** Pending marks of attachment rows: an entry of the candidate that is not in the running configuration. */
export function samePlain(a: unknown, b: unknown): boolean {
  return JSON.stringify(canonical(a)) === JSON.stringify(canonical(b));
}

function canonical(v: unknown): unknown {
  if (Array.isArray(v)) return v.map(canonical);
  if (v && typeof v === 'object') {
    return Object.fromEntries(
      Object.entries(v as Record<string, unknown>)
        .filter(([, x]) => x !== undefined)
        .sort(([x], [y]) => x.localeCompare(y))
        .map(([k, x]) => [k, canonical(x)]),
    );
  }
  return v;
}

// ---------------------------------------------------------------------------------------------------------------
// Schemas (the one schema, 00-CONTEXT rule 5: generated JSON Schema of the `acl` domain)
// ---------------------------------------------------------------------------------------------------------------

function aclProp(name: string): JsonSchema {
  const p = (domainSchemas.acl.properties ?? {})[name];
  if (!p) throw new Error(`acl.${name} schema not found`);
  return p;
}

function recordItem(name: string): JsonSchema {
  const rec = aclProp(name) as { additionalProperties?: JsonSchema | boolean };
  if (!rec.additionalProperties || typeof rec.additionalProperties !== 'object')
    throw new Error(`acl.${name} item schema not found`);
  return rec.additionalProperties;
}

/** `acl.lists.<name>.rules[]` item: the rule editor's form. */
export function ruleSchema(): JsonSchema {
  const rules = (recordItem('lists').properties ?? {})['rules'];
  if (!rules?.items) throw new Error('acl.lists rules schema not found');
  return rules.items;
}

/**
 * ui-kit's SchemaForm fills an ABSENT optional object member with an object of empty fields (P08-questions Q2), so an
 * optional object whose members are required (`service.spec.tcpFlags`) would make every such rule fail validation
 * ("does not match any allowed form"). Until the ui-kit handles absent optional objects, such members are edited as
 * JSON text, where empty means absent.
 */
export function optionalObjectsAsJson(schema: JsonSchema): JsonSchema {
  const out: JsonSchema = { ...schema };
  if (schema.properties) {
    const required = new Set(Array.isArray(schema.required) ? schema.required : []);
    out.properties = Object.fromEntries(
      Object.entries(schema.properties).map(([k, p]) => {
        const plainObject =
          p.type === 'object' && p.properties !== undefined && !p.oneOf && !p.anyOf;
        const needsMembers = Array.isArray(p.required) && p.required.length > 0;
        if (!required.has(k) && plainObject && needsMembers && p.default === undefined) {
          return [
            k,
            {
              ...p,
              'x-vrx-ui': { ...((p['x-vrx-ui'] ?? {}) as Record<string, unknown>), widget: 'json' },
            },
          ];
        }
        return [k, optionalObjectsAsJson(p)];
      }),
    );
  }
  for (const key of ['oneOf', 'anyOf'] as const) {
    const vs = schema[key];
    if (vs) out[key] = vs.map(optionalObjectsAsJson);
  }
  if (schema.items && typeof schema.items === 'object')
    out.items = optionalObjectsAsJson(schema.items);
  return out;
}

/** The rule editor's form: the rule schema with optional objects as JSON text (see `optionalObjectsAsJson`). */
export function ruleFormSchema(): JsonSchema {
  return optionalObjectsAsJson(ruleSchema());
}

/** `acl.lists.<name>` without its rules (they are edited in the rule editor, never as one array). */
export function listMetaSchema(): JsonSchema {
  return withoutProps(recordItem('lists'), ['rules']);
}

/** `acl.macip.<name>` (MACIP lists are small: edited whole, rules included). */
export function macipListSchema(): JsonSchema {
  return recordItem('macip');
}

/** `acl.attachments[]` item, the list offered as a choice of the existing lists (not an object picker). */
export function attachmentSchema(lists: readonly string[]): JsonSchema {
  return withListChoice(aclProp('attachments').items ?? {}, lists);
}

/** `acl.macipAttachments[]` item, the list offered as a choice of the existing MACIP lists. */
export function macipAttachmentSchema(lists: readonly string[]): JsonSchema {
  return withListChoice(aclProp('macipAttachments').items ?? {}, lists);
}

function withoutProps(schema: JsonSchema, drop: readonly string[]): JsonSchema {
  const props = { ...(schema.properties ?? {}) };
  for (const d of drop) delete props[d];
  const required = Array.isArray(schema.required)
    ? schema.required.filter((r) => !drop.includes(r))
    : undefined;
  return { ...schema, properties: props, ...(required ? { required } : {}) };
}

function withListChoice(item: JsonSchema, lists: readonly string[]): JsonSchema {
  const props = { ...(item.properties ?? {}) };
  const list = props['list'];
  if (list) {
    const hints = { ...((list['x-vrx-ui'] ?? {}) as Record<string, unknown>) };
    delete hints.widget;
    props['list'] = { ...list, enum: [...lists], 'x-vrx-ui': { ...hints, widget: 'select' } };
  }
  return { ...item, properties: props };
}

/**
 * Titles, help, enum labels and union-variant titles in the UI language (`acl:field.<prop>.title|help`,
 * `acl:enum.<prop>.<value>`, `acl:variant.<value>`), the schema's English text as fallback, recursively. Object-picker
 * fields keep the kinds their original help names (`x-vrx-ui.objectKinds`): the translated help may not name them.
 */
export function localizeSchema(schema: JsonSchema, t: Translate): JsonSchema {
  const out: JsonSchema = { ...schema };
  if (schema.properties) {
    const props: Record<string, JsonSchema> = {};
    for (const [name, prop] of Object.entries(schema.properties)) {
      const ph = (prop['x-vrx-ui'] ?? {}) as Record<string, unknown>;
      const help = t(`field.${name}.help`, {
        defaultValue: typeof ph.help === 'string' ? ph.help : '',
      });
      const extra: Record<string, unknown> = {};
      if (ph.widget === 'object-picker' || ph.widget === 'tag-picker') {
        const kinds = pickerKinds(ph, name);
        if (kinds.length > 0) extra.objectKinds = kinds;
      }
      if (Array.isArray(prop.enum)) {
        extra.enumLabels = Object.fromEntries(
          prop.enum.map((v) => [
            String(v),
            t(`enum.${name}.${String(v)}`, { defaultValue: String(v) }),
          ]),
        );
      }
      props[name] = localizeSchema(
        {
          ...prop,
          ...(prop.const === undefined
            ? { title: t(`field.${name}.title`, { defaultValue: prop.title ?? name }) }
            : {}),
          'x-vrx-ui': { ...ph, ...extra, ...(help ? { help } : {}) },
        } as JsonSchema,
        t,
      );
    }
    out.properties = props;
  }
  for (const key of ['oneOf', 'anyOf'] as const) {
    const vs = schema[key];
    if (!vs) continue;
    out[key] = vs.map((v) => {
      const lv = localizeSchema(v, t);
      const disc = variantValues(v);
      return disc
        ? { ...lv, title: t(`variant.${disc.join('|')}`, { defaultValue: disc.join(' / ') }) }
        : lv;
    });
  }
  if (schema.items && typeof schema.items === 'object') out.items = localizeSchema(schema.items, t);
  return out;
}

function variantValues(v: JsonSchema): string[] | undefined {
  for (const [name, p] of Object.entries(v.properties ?? {})) {
    if (p.const !== undefined) return [String(p.const)];
    if ((name === 'protocol' || name === 'kind') && Array.isArray(p.enum))
      return p.enum.map(String);
  }
  return undefined;
}

/**
 * RFC 7386 merge patch that turns `from` into `to` (removed members → null, objects recurse, arrays and scalars are
 * replaced). Used for the list description/tags edit: only what changed is sent.
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

/** Server pointers under `prefix` (`/acl/lists/web/rules/3/…`) → pointers relative to the edited node, for `<SchemaForm problem>`. */
export function problemFor(error: unknown, prefix: string): ProblemDetails | null {
  if (!(error instanceof ApiError)) return null;
  const p = error.toFormProblem();
  const rel = (ptr: string) =>
    ptr === prefix ? '' : ptr.startsWith(`${prefix}/`) ? ptr.slice(prefix.length) : ptr;
  return {
    ...p,
    ...(typeof p.pointer === 'string' ? { pointer: rel(p.pointer) } : {}),
    errors: (p.errors ?? []).map((e) => ({ ...e, pointer: rel(e.pointer) })),
  };
}

/** Escape one JSON-pointer segment (RFC 6901). */
export const esc = (s: string) => s.replace(/~/g, '~0').replace(/\//g, '~1');

/** Interface names an attachment may target: the configured interfaces and sub-interfaces plus what VPP reports. */
export function interfaceChoices(
  config: Record<string, { subinterfaces?: Record<string, unknown> }> | undefined,
  live: readonly string[] = [],
): string[] {
  const names = new Set<string>(live);
  for (const [name, cfg] of Object.entries(config ?? {})) {
    names.add(name);
    for (const id of Object.keys(cfg.subinterfaces ?? {})) names.add(`${name}.${id}`);
  }
  return [...names].sort((a, b) => a.localeCompare(b, undefined, { numeric: true }));
}
