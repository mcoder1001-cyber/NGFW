import {
  isRecordSchema,
  mergeAllOf,
  recordValueSchema,
  resolveRef,
  sortedProperties,
  type JsonSchema,
} from '@ngfw/ui-kit/schema-form';

/** `/config/<domain>[/<segment>...]` split into the top-level schema domain and the pointer segments below it. */
export interface ConfigPath {
  domainKey: string;
  /** Segments under the domain, already URL-decoded (never pointer-escaped: these are plain object/array keys). */
  segments: string[];
}

const ARRAY_INDEX = /^(?:0|[1-9][0-9]*)$/;

/**
 * Parse the `/config/*` route splat. `validDomains` is `ROOT_KEYS` (never a hand-written list, 00-CONTEXT rule 5);
 * an empty splat or an unknown first segment is not a configuration path.
 */
export function parseConfigPath(splat: string | undefined, validDomains: readonly string[]): ConfigPath | null {
  const raw = (splat ?? '').split('/').filter((s) => s !== '');
  const domainKey = raw[0];
  if (domainKey === undefined || !validDomains.includes(domainKey)) return null;
  return { domainKey, segments: raw.slice(1) };
}

/** Build the route splat for a config path (the inverse of {@link parseConfigPath}). */
export function configPathTo(domainKey: string, segments: readonly string[] = []): string {
  return ['config', domainKey, ...segments].join('/');
}

function resolved(schema: JsonSchema, root: JsonSchema): JsonSchema {
  return mergeAllOf(resolveRef(schema, root), root);
}

export interface ResolvedNode {
  /** Fully resolved ($ref followed, `allOf` merged) schema of the node at the path. */
  schema: JsonSchema;
  /** An `additionalProperties` map keyed by arbitrary names (VPP interface names, VRF names, …). */
  record: boolean;
}

/**
 * Walk `domainSchema` (the generated JSON Schema of one root key, `apps/web/src/schema/registry.ts`) by `segments` —
 * a record's segment is any key (its schema is always `additionalProperties`), a fixed object's segment must name a
 * property, an array's segment must be an index. `null` when the schema has nothing at that path (the JSON-pointer
 * subtree does not exist in the contract, distinct from "not created in the candidate yet").
 */
export function resolveNode(domainSchema: JsonSchema, segments: readonly string[]): ResolvedNode | null {
  let schema = resolved(domainSchema, domainSchema);
  for (const seg of segments) {
    if (isRecordSchema(schema)) {
      schema = resolved(recordValueSchema(schema), domainSchema);
      continue;
    }
    if (Array.isArray(schema.type) ? schema.type.includes('array') : schema.type === 'array') {
      if (!ARRAY_INDEX.test(seg)) return null;
      const item = schema.prefixItems?.[Number(seg)] ?? schema.items;
      if (!item) return null;
      schema = resolved(item, domainSchema);
      continue;
    }
    const prop = schema.properties?.[seg];
    if (prop === undefined) return null;
    schema = resolved(prop, domainSchema);
  }
  return { schema, record: isRecordSchema(schema) };
}

/**
 * Keys of `schema`'s fixed properties that are themselves navigable containers (a nested object or record) — these
 * become their own tree node instead of a field of this node's form (the existing convention: `interfaces.<name>`
 * excludes `subinterfaces` from its form for the same reason, `apps/web/src/domains/interfaces/model.ts`).
 */
export function childPropertyKeys(schema: JsonSchema, root: JsonSchema): string[] {
  if (isRecordSchema(schema)) return [];
  return sortedProperties(schema, root)
    .filter((e) => {
      const s = resolved(e.schema, root);
      const t = Array.isArray(s.type) ? s.type.find((x) => x !== 'null') : s.type;
      return t === 'object' || (t === undefined && (s.properties !== undefined || s.additionalProperties !== undefined));
    })
    .map((e) => e.key);
}

/** `schema` without the given properties (and them from `required`) — what the node's own `<SchemaForm>` renders. */
export function withoutChildProperties(schema: JsonSchema, childKeys: readonly string[]): JsonSchema {
  if (childKeys.length === 0) return schema;
  const properties = { ...(schema.properties ?? {}) };
  for (const key of childKeys) delete properties[key];
  const required = schema.required?.filter((r) => !childKeys.includes(r));
  return { ...schema, properties, ...(required ? { required } : {}) };
}

/**
 * `value` without the given members — the same projection as {@link withoutChildProperties}, applied to a plain
 * object value instead of a schema. Used to build the diff base for a node's own `<SchemaForm>` (`AdvancedEditorPage`
 * `save`): the form never carries the child-container members (they are their own tree node), so diffing the *whole*
 * node against the form's value would see them as removed and merge-patch them to `null`.
 */
export function withoutChildValues(value: unknown, childKeys: readonly string[]): unknown {
  if (childKeys.length === 0 || !isPlainObject(value)) return value;
  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(value)) if (!childKeys.includes(k)) out[k] = v;
  return out;
}

/** The value at `segments` of a plain JS value (arrays and objects only); `undefined` when any segment is missing. */
export function valueAt(value: unknown, segments: readonly string[]): unknown {
  let cur = value;
  for (const seg of segments) {
    if (Array.isArray(cur)) {
      if (!ARRAY_INDEX.test(seg)) return undefined;
      cur = cur[Number(seg)];
    } else if (cur !== null && typeof cur === 'object') {
      cur = (cur as Record<string, unknown>)[seg];
    } else {
      return undefined;
    }
  }
  return cur;
}

/** Nest `body` under `segments` (`['a','b'], v` → `{ a: { b: v } }`); `[]` returns `body` itself. */
export function wrapAtPath(segments: readonly string[], body: unknown): unknown {
  return segments.reduceRight<unknown>((acc, seg) => ({ [seg]: acc }), body);
}

function isPlainObject(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

/**
 * RFC 7386 merge patch that turns `from` into `to`: removed members become `null`, objects recurse, everything else
 * (arrays, scalars) is replaced. The same pure computation as `domains/interfaces/model.ts`'s `createMergePatch`
 * (not shared from `@ngfw/ui-kit` or `@ngfw/schema` yet — a kit gap noted in UI-domain-editor-questions.md).
 */
export function createMergePatch(from: unknown, to: unknown): unknown {
  if (!isPlainObject(from) || !isPlainObject(to)) return to === undefined ? null : to;
  const patch: Record<string, unknown> = {};
  for (const k of Object.keys(from)) if (!(k in to) || to[k] === undefined) patch[k] = null;
  for (const [k, v] of Object.entries(to)) {
    if (v === undefined) continue;
    const prev = from[k];
    if (isPlainObject(prev) && isPlainObject(v)) {
      const inner = createMergePatch(prev, v) as Record<string, unknown>;
      if (Object.keys(inner).length > 0) patch[k] = inner;
    } else if (JSON.stringify(prev) !== JSON.stringify(v)) {
      patch[k] = v;
    }
  }
  return patch;
}
