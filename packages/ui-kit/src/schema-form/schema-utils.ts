import type { JsonSchema, JsonSchemaType, UiDependsOn, UiHints } from './types.js';

export function hintsOf(schema: JsonSchema): UiHints {
  const h = schema['x-vrx-ui'];
  return h && typeof h === 'object' ? h : {};
}

/** RFC 6901: `/a/b~1c/0` → ['a', 'b/c', '0']. A leading `#` (URI fragment form) is tolerated. */
export function parsePointer(pointer: string): string[] {
  let p = pointer.startsWith('#') ? pointer.slice(1) : pointer;
  if (p === '') return [];
  if (!p.startsWith('/')) p = `/${p}`;
  return p
    .slice(1)
    .split('/')
    .map((s) => s.replace(/~1/g, '/').replace(/~0/g, '~'));
}

export function getByPointer(doc: unknown, pointer: string): unknown {
  let cur: unknown = doc;
  for (const seg of parsePointer(pointer)) {
    if (cur === null || typeof cur !== 'object') return undefined;
    cur = Array.isArray(cur) ? cur[Number(seg)] : (cur as Record<string, unknown>)[seg];
  }
  return cur;
}

/** Follow local `$ref`s (`#/$defs/...`); keywords next to the `$ref` (title, x-vrx-ui) override the target's. */
export function resolveRef(schema: JsonSchema, root: JsonSchema): JsonSchema {
  let cur = schema;
  const seen = new Set<string>();
  while (typeof cur.$ref === 'string' && cur.$ref.startsWith('#') && !seen.has(cur.$ref)) {
    seen.add(cur.$ref);
    const target = getByPointer(root, cur.$ref);
    if (!target || typeof target !== 'object') break;
    const siblings: JsonSchema = { ...cur };
    delete siblings.$ref;
    cur = { ...(target as JsonSchema), ...siblings };
  }
  return cur;
}

function jsTypeOf(v: unknown): JsonSchemaType | undefined {
  if (typeof v === 'string') return 'string';
  if (typeof v === 'number') return Number.isInteger(v) ? 'integer' : 'number';
  if (typeof v === 'boolean') return 'boolean';
  if (Array.isArray(v)) return 'array';
  if (v !== null && typeof v === 'object') return 'object';
  return undefined;
}

/** Effective (non-null) type, inferred from structural keywords when `type` is absent. */
export function typeOf(schema: JsonSchema): JsonSchemaType | undefined {
  const t = schema.type;
  if (Array.isArray(t)) return t.find((x) => x !== 'null');
  if (t) return t;
  if (schema.properties || schema.additionalProperties !== undefined) return 'object';
  if (schema.items || schema.prefixItems) return 'array';
  if (schema.enum) {
    const first = schema.enum.find((v) => v !== null);
    return first === undefined ? undefined : jsTypeOf(first);
  }
  if (schema.const !== undefined) return jsTypeOf(schema.const);
  return undefined;
}

export function isNullable(schema: JsonSchema): boolean {
  return Array.isArray(schema.type) && schema.type.includes('null');
}

export function variantsOf(schema: JsonSchema): JsonSchema[] | undefined {
  return schema.oneOf ?? schema.anyOf;
}

/** An object keyed by arbitrary names (VPP interface names, VRF names …): no fixed properties + a value schema. */
export function isRecordSchema(schema: JsonSchema): boolean {
  if (typeOf(schema) !== 'object') return false;
  if (schema.properties && Object.keys(schema.properties).length > 0) return false;
  const ap = schema.additionalProperties;
  return ap === true || (typeof ap === 'object' && ap !== null);
}

export function recordValueSchema(schema: JsonSchema): JsonSchema {
  const ap = schema.additionalProperties;
  return typeof ap === 'object' && ap !== null ? ap : {};
}

/** `{}` / `true`: accepts anything — rendered as a JSON editor. */
export function isAnySchema(schema: JsonSchema): boolean {
  return (
    typeOf(schema) === undefined &&
    !variantsOf(schema) &&
    !schema.allOf &&
    !schema.enum &&
    schema.const === undefined
  );
}

/** Shallow `allOf` merge (properties/required unioned, scalar keywords from the last member). */
export function mergeAllOf(schema: JsonSchema, root: JsonSchema): JsonSchema {
  if (!schema.allOf) return schema;
  const { allOf, ...base } = schema;
  let out: JsonSchema = { ...base };
  for (const raw of allOf) {
    const m = resolveRef(raw, root);
    out = {
      ...out,
      ...m,
      properties: { ...(out.properties ?? {}), ...(m.properties ?? {}) },
      required: [...new Set([...(out.required ?? []), ...(m.required ?? [])])],
    };
  }
  return out;
}

export interface PropertyEntry {
  key: string;
  schema: JsonSchema;
  required: boolean;
  hints: UiHints;
}

/** Properties in render order: `x-vrx-ui.order` ascending, then declaration order. */
export function sortedProperties(schema: JsonSchema, root: JsonSchema): PropertyEntry[] {
  const required = new Set(schema.required ?? []);
  return Object.entries(schema.properties ?? {})
    .map(([key, raw], index) => {
      const resolved = resolveRef(raw, root);
      return { key, schema: resolved, required: required.has(key), hints: hintsOf(resolved), index };
    })
    .sort(
      (a, b) =>
        (a.hints.order ?? Number.MAX_SAFE_INTEGER) - (b.hints.order ?? Number.MAX_SAFE_INTEGER) ||
        a.index - b.index,
    )
    .map(({ index: _index, ...entry }) => entry);
}

export interface PropertyGroup {
  /** `undefined` = ungrouped fields (rendered first). */
  name: string | undefined;
  entries: PropertyEntry[];
}

export function groupProperties(entries: PropertyEntry[]): PropertyGroup[] {
  const groups = new Map<string | undefined, PropertyEntry[]>();
  for (const e of entries) {
    const list = groups.get(e.hints.group);
    if (list) list.push(e);
    else groups.set(e.hints.group, [e]);
  }
  const out: PropertyGroup[] = [];
  const ungrouped = groups.get(undefined);
  if (ungrouped) out.push({ name: undefined, entries: ungrouped });
  for (const [name, list] of groups) if (name !== undefined) out.push({ name, entries: list });
  return out;
}

export function humanize(key: string): string {
  return key
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .replace(/[_-]+/g, ' ')
    .replace(/^./, (c) => c.toUpperCase());
}

export function titleOf(schema: JsonSchema, key?: string): string {
  if (schema.title) return schema.title;
  return key === undefined ? '' : humanize(key);
}

function cloneJson<T>(v: T): T {
  return v === undefined ? v : (JSON.parse(JSON.stringify(v)) as T);
}

/** Starting value for a fresh instance: `default`, `const`, or an object filled with its properties' defaults. */
export function defaultValueFor(raw: JsonSchema, root: JsonSchema): unknown {
  const schema = mergeAllOf(resolveRef(raw, root), root);
  if (schema.default !== undefined) return cloneJson(schema.default);
  if (schema.const !== undefined) return cloneJson(schema.const);
  const t = typeOf(schema);
  if (t === 'object') {
    if (isRecordSchema(schema)) return {};
    const out: Record<string, unknown> = {};
    for (const e of sortedProperties(schema, root)) {
      const v = defaultValueFor(e.schema, root);
      if (v !== undefined) out[e.key] = v;
    }
    return out;
  }
  if (t === 'array') return [];
  const variants = variantsOf(schema);
  if (variants?.[0]) return defaultValueFor(variants[0], root);
  return undefined;
}

export function dependsOnOf(hints: UiHints): UiDependsOn | undefined {
  const d = hints.dependsOn;
  if (!d) return undefined;
  return typeof d === 'string' ? { field: d } : d;
}

export function deepEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true;
  if (a === null || b === null || typeof a !== 'object' || typeof b !== 'object') return false;
  return JSON.stringify(a) === JSON.stringify(b);
}

export function dependencyMet(dep: UiDependsOn, actual: unknown): boolean {
  if (dep.values) return dep.values.some((v) => deepEqual(v, actual));
  if (dep.value !== undefined) return deepEqual(dep.value, actual);
  return actual !== undefined && actual !== null && actual !== false && actual !== '';
}

/** Join RHF field paths (`''` is the root). */
export function joinPath(parent: string, child: string | number): string {
  return parent === '' ? String(child) : `${parent}.${child}`;
}

/** Nested lookup by dotted RHF path (used for the errors object). */
export function getIn(obj: unknown, path: string): unknown {
  let cur: unknown = obj;
  for (const seg of path.split('.')) {
    if (cur === null || cur === undefined || typeof cur !== 'object') return undefined;
    cur = (cur as Record<string, unknown>)[seg];
  }
  return cur;
}
