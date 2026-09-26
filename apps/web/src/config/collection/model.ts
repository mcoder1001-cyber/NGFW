import { jsonPointer, mergePatch, type RootKey } from '@ngfw/schema';
import { compile, resolveRef, type JsonSchema, type ProblemDetails } from '@ngfw/ui-kit/schema-form';
import { ApiError } from '../../api-problem';
import { domainSchemas } from '../../schema/registry';

/**
 * Config screen kit — the generic half of P08's interfaces screen and System › Users (WEB-2).
 *
 * A *collection* is one node of the candidate document that holds many items:
 *   - `map`:  an object keyed by name (`interfaces.<name>`, `vrfs.<name>`, `objects.addresses.<name>`); the key rules come
 *             from the JSON Schema's `propertyNames`;
 *   - `list`: an array whose items carry their own key (`management.users[]` keyed by `username`, `routing.static[]` by
 *             `vrf`+`prefix`); the key members come from the array's `x-vrx-ui.itemKey` hint.
 * Everything here is derived from `packages/schema` (00-CONTEXT rule 5) — no per-domain code.
 */
export interface CollectionSpec {
  /** Root key of the configuration document; edits are merge patches of this node (`PATCH /api/v1/config/<domain>`). */
  domain: RootKey;
  /** Members from the domain root down to the collection node; `[]`/absent = the domain itself is the collection. */
  path?: readonly string[] | undefined;
  /** i18n namespace with `field.<member>.title|help|enum.<value>` and `group.<name>` for the item form. */
  ns: string;
  /** Item members edited somewhere else (P08: `subinterfaces` has its own table); left out of the form, never written. */
  omit?: readonly string[] | undefined;
}

export type CollectionShape = { kind: 'map'; keySchema: JsonSchema | undefined } | { kind: 'list'; itemKey: readonly string[] };

export interface CollectionModel {
  spec: CollectionSpec;
  path: readonly string[];
  shape: CollectionShape;
  /** Schema of one item, as generated. */
  itemSchema: JsonSchema;
  /** The item schema without the `omit` members — what the drawer form edits. */
  formSchema: JsonSchema;
}

export type Json = Record<string, unknown>;

export function isObject(v: unknown): v is Json {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

/** A nullable wrapper (`anyOf: [X, {type: 'null'}]`) → X; anything else unchanged. */
function unwrapNullable(schema: JsonSchema, root: JsonSchema): JsonSchema {
  const s = resolveRef(schema, root);
  const variants = s.anyOf ?? s.oneOf;
  if (!variants) return s;
  const real = variants.filter((v) => resolveRef(v, root).type !== 'null');
  return real.length === 1 && real.length < variants.length ? resolveRef(real[0]!, root) : s;
}

/**
 * The schema of the node at `path` below `root`. A segment is a member name, or a key of a map it passes through, so a
 * nested collection is addressed like the document (`['host-w1l0', 'subinterfaces']` in `interfaces`). Throws when the
 * schema has no such member.
 */
export function schemaAt(root: JsonSchema, path: readonly string[]): JsonSchema {
  let cur = unwrapNullable(root, root);
  for (const seg of path) {
    const ap = cur.additionalProperties;
    const next = cur.properties?.[seg] ?? (typeof ap === 'object' && ap !== null ? ap : undefined);
    if (!next) throw new Error(`schema has no member '${seg}' (path ${jsonPointer(...path)})`);
    cur = unwrapNullable(next, root);
  }
  return cur;
}

/** Shape and item schema of a collection node, from the schema alone. */
export function shapeOf(node: JsonSchema, root: JsonSchema): { shape: CollectionShape; itemSchema: JsonSchema } {
  if (node.type === 'array' || node.items) {
    const itemKey = node['x-vrx-ui']?.itemKey;
    if (!Array.isArray(itemKey) || itemKey.length === 0 || !itemKey.every((k) => typeof k === 'string')) {
      throw new Error('a list collection needs x-vrx-ui.itemKey on its array schema');
    }
    if (!node.items) throw new Error('array schema without items');
    return { shape: { kind: 'list', itemKey: itemKey as string[] }, itemSchema: resolveRef(node.items, root) };
  }
  const ap = node.additionalProperties;
  if (typeof ap !== 'object' || ap === null) throw new Error('a map collection needs an object schema with additionalProperties');
  return { shape: { kind: 'map', keySchema: node.propertyNames }, itemSchema: resolveRef(ap, root) };
}

/** The item schema without some members (P08's `interfaceFormSchema` generalised). Unions are left as they are. */
export function withoutMembers(schema: JsonSchema, omit: readonly string[]): JsonSchema {
  if (omit.length === 0 || !schema.properties) return schema;
  const props = { ...schema.properties };
  for (const k of omit) delete props[k];
  const required = Array.isArray(schema.required) ? schema.required.filter((r) => !omit.includes(r)) : undefined;
  return { ...schema, properties: props, ...(required ? { required } : {}) } as JsonSchema;
}

/** Build the model of a collection from the generated domain JSON Schema (`schema/registry`). */
export function collectionModel(spec: CollectionSpec, root: JsonSchema = domainSchemas[spec.domain]): CollectionModel {
  const path = spec.path ?? [];
  const { shape, itemSchema } = shapeOf(schemaAt(root, path), root);
  return { spec, path, shape, itemSchema, formSchema: withoutMembers(itemSchema, spec.omit ?? []) };
}

/* ------------------------------------------------------------------------------------------------------------ keys */

/** Stable id of a list item: the key member for a single itemKey, a JSON tuple for a compound one. */
export function listKeyOf(item: unknown, itemKey: readonly string[]): string {
  const values = itemKey.map((k) => (isObject(item) ? item[k] : undefined));
  return values.length === 1 ? String(values[0] ?? '') : JSON.stringify(values.map((v) => v ?? null));
}

/** Human label of a key id (compound keys are shown as their members separated by spaces). */
export function keyLabel(model: CollectionModel, id: string): string {
  if (model.shape.kind === 'map' || model.shape.itemKey.length === 1) return id;
  try {
    const v = JSON.parse(id) as unknown;
    return Array.isArray(v) ? v.map((x) => String(x ?? '')).join(' ') : id;
  } catch {
    return id;
  }
}

/** `undefined` when `key` is a valid map key per the schema's `propertyNames`, otherwise a reason (`empty`/`invalid`). */
export function mapKeyIssue(model: CollectionModel, key: string): 'empty' | 'invalid' | undefined {
  if (key === '') return 'empty';
  if (model.shape.kind !== 'map' || !model.shape.keySchema) return undefined;
  return compile(model.shape.keySchema, model.shape.keySchema).safeParse(key).success ? undefined : 'invalid';
}

/* ---------------------------------------------------------------------------------------------------------- values */

/** The node at `path` inside a JSON value (`undefined` when absent). */
export function valueAt(doc: unknown, path: readonly string[]): unknown {
  let cur = doc;
  for (const seg of path) cur = isObject(cur) ? cur[seg] : undefined;
  return cur;
}

/** `{a: {b: value}}` for `path = ['a', 'b']` — a merge patch that touches only that node. */
export function nest(path: readonly string[], value: unknown): unknown {
  return path.reduceRight<unknown>((acc, seg) => ({ [seg]: acc }), value);
}

/** The item without the `omit` members (they are never part of the form value). */
export function formValueOf(model: CollectionModel, item: unknown): Json | undefined {
  if (!isObject(item)) return undefined;
  const omit = model.spec.omit ?? [];
  return omit.length === 0 ? item : Object.fromEntries(Object.entries(item).filter(([k]) => !omit.includes(k)));
}

/**
 * RFC 7386 merge patch that turns `from` into `to`: removed members become `null`, objects recurse, everything else
 * (arrays, scalars) is replaced — `mergePatch(from, createMergePatch(from, to))` equals `to` (property-tested).
 */
export function createMergePatch(from: unknown, to: unknown): unknown {
  if (!isObject(from) || !isObject(to)) return to === undefined ? null : to;
  const patch: Json = {};
  for (const k of Object.keys(from)) if (!(k in to) || to[k] === undefined) patch[k] = null;
  for (const [k, v] of Object.entries(to)) {
    if (v === undefined) continue;
    const prev = from[k];
    if (isObject(prev) && isObject(v)) {
      const inner = createMergePatch(prev, v) as Json;
      if (Object.keys(inner).length > 0) patch[k] = inner;
    } else if (!deepEqual(prev, v)) {
      patch[k] = v;
    }
  }
  return patch;
}

/** Structural equality of JSON values; member order does not matter, array order does. */
export function deepEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true;
  if (Array.isArray(a)) return Array.isArray(b) && a.length === b.length && a.every((x, i) => deepEqual(x, b[i]));
  if (isObject(a) && isObject(b)) {
    const ka = Object.keys(a).filter((k) => a[k] !== undefined);
    const kb = Object.keys(b).filter((k) => b[k] !== undefined);
    return ka.length === kb.length && ka.every((k) => deepEqual(a[k], b[k]));
  }
  return false;
}

/**
 * The next array for a list collection: the edited item is located by its ORIGINAL key in the candidate as it is
 * NOW (`current`), and only the members the user changed against the value the form opened with are applied to it
 * (P08 review N4, for arrays: merge patches replace arrays whole, so the item is rebuilt, not the array blindly
 * written back). A new item, or one that vanished meanwhile, is appended.
 */
export function nextList(
  current: readonly unknown[],
  itemKey: readonly string[],
  originalKey: string | null,
  opened: unknown,
  value: unknown,
): { list: unknown[]; index: number } {
  const list = [...current];
  const at = originalKey === null ? -1 : list.findIndex((i) => listKeyOf(i, itemKey) === originalKey);
  if (at < 0) {
    list.push(value);
    return { list, index: list.length - 1 };
  }
  list[at] = opened === undefined ? value : mergePatch(list[at], createMergePatch(opened, value));
  return { list, index: at };
}

/* ------------------------------------------------------------------------------------------------------------ rows */

/** Candidate vs running of one item. */
export type RowState = 'new' | 'changed' | 'removed' | undefined;

export interface CollectionRow<T = Json> {
  /** Key id (grid row id): the map key, or the list item's key members. */
  id: string;
  /** The key as shown. */
  label: string;
  /** The item in the candidate (`undefined` when the candidate removes it). */
  value: T | undefined;
  /** The item in the running configuration (`undefined` when it is new). */
  running: T | undefined;
  /** Position in the candidate array (list shape); -1 for maps and removed items. */
  index: number;
  state: RowState;
}

function entriesOf(model: CollectionModel, node: unknown): [string, unknown, number][] {
  if (model.shape.kind === 'map') return isObject(node) ? Object.entries(node).map(([k, v]) => [k, v, -1]) : [];
  const { itemKey } = model.shape;
  return Array.isArray(node) ? node.map((v, i) => [listKeyOf(v, itemKey), v, i]) : [];
}

/** Rows of a collection: candidate items in candidate order with their pending state, then items the candidate removes. */
export function collectionRows<T = Json>(model: CollectionModel, candidate: unknown, running: unknown): CollectionRow<T>[] {
  const run = new Map(entriesOf(model, running).map(([k, v]) => [k, v]));
  const rows: CollectionRow<T>[] = [];
  const seen = new Set<string>();
  for (const [id, value, index] of entriesOf(model, candidate)) {
    seen.add(id);
    const r = run.get(id);
    rows.push({
      id,
      label: keyLabel(model, id),
      value: value as T,
      running: r as T | undefined,
      index,
      state: !run.has(id) ? 'new' : deepEqual(r, value) ? undefined : 'changed',
    });
  }
  for (const [id, r] of run) {
    if (!seen.has(id)) rows.push({ id, label: keyLabel(model, id), value: undefined, running: r as T, index: -1, state: 'removed' });
  }
  return rows;
}

/* -------------------------------------------------------------------------------------------------------- pointers */

/** RFC 6901 pointer of one item in the whole document (map key, or candidate index for lists). */
export function itemPointer(model: CollectionModel, key: string | number): string {
  return jsonPointer(model.spec.domain, ...model.path, key);
}

/**
 * A server problem with its `errors[].pointer`s made relative to the edited item (`<SchemaForm problem>` maps them onto
 * fields). Pointers elsewhere stay absolute (the form lists them on top). `null` for anything but an API problem.
 */
export function problemAt(error: unknown, prefix: string): ProblemDetails | null {
  if (!(error instanceof ApiError)) return null;
  const p = error.toFormProblem();
  const rel = (ptr: string) => (ptr === prefix ? '' : ptr.startsWith(`${prefix}/`) ? ptr.slice(prefix.length) : ptr);
  return {
    ...p,
    ...(typeof p.pointer === 'string' ? { pointer: rel(p.pointer) } : {}),
    errors: (p.errors ?? []).map((e) => ({ ...e, pointer: rel(e.pointer) })),
  };
}

/* ------------------------------------------------------------------------------------------------------------ i18n */

type Translate = (key: string, opts?: Record<string, unknown>) => string;

/**
 * Titles, help, group names and enum labels of the item form in the UI language, from the domain namespace
 * (`field.<member>.title|help|enum.<value>`, `group.<name>`); the schema's English text stays the fallback. One function
 * for P08's `localizeSchema` and Users' `localizeUserSchema` (WEB-1 moves per-path translation into SchemaForm).
 */
export function localizeSchema(schema: JsonSchema, t: Translate): JsonSchema {
  const props = schema.properties ?? {};
  const out: Record<string, JsonSchema> = {};
  for (const [name, prop] of Object.entries(props)) {
    const hints = prop['x-vrx-ui'] ?? {};
    const help = t(`field.${name}.help`, { defaultValue: '' });
    const enumLabels = Array.isArray(prop.enum)
      ? Object.fromEntries(prop.enum.filter((v) => v !== null).map((v) => [String(v), t(`field.${name}.enum.${String(v)}`, { defaultValue: String(v) })]))
      : undefined;
    out[name] = {
      ...prop,
      title: t(`field.${name}.title`, { defaultValue: prop.title ?? name }),
      'x-vrx-ui': {
        ...hints,
        ...(help ? { help } : {}),
        ...(typeof hints.group === 'string' ? { group: t(`group.${hints.group}`, { defaultValue: hints.group }) } : {}),
        ...(enumLabels ? { enumLabels } : {}),
      },
    } as JsonSchema;
  }
  return { ...schema, properties: out } as JsonSchema;
}
