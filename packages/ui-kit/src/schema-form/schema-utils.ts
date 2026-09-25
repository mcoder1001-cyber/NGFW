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

/**
 * Per-root memo of derived schema objects (review P07a L1): `resolveRef`/`mergeAllOf` return the *same* object for the
 * same input, so `compile()`'s identity-keyed cache hits instead of re-running `z.fromJSONSchema` on every keystroke.
 */
function memo(): (root: JsonSchema, schema: JsonSchema, make: () => JsonSchema) => JsonSchema {
  const byRoot = new WeakMap<JsonSchema, WeakMap<JsonSchema, JsonSchema>>();
  return (root, schema, make) => {
    let m = byRoot.get(root);
    if (!m) byRoot.set(root, (m = new WeakMap()));
    let out = m.get(schema);
    if (!out) m.set(schema, (out = make()));
    return out;
  };
}
const refMemo = memo();
const allOfMemo = memo();

/** Follow local `$ref`s (`#/$defs/...`); keywords next to the `$ref` (title, x-vrx-ui) override the target's. Memoised. */
export function resolveRef(schema: JsonSchema, root: JsonSchema): JsonSchema {
  if (typeof schema.$ref !== 'string') return schema;
  return refMemo(root, schema, () => resolveRefUncached(schema, root));
}

function resolveRefUncached(schema: JsonSchema, root: JsonSchema): JsonSchema {
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

/** Does the schema allow JSON `null` (`type` with null, `const`/`enum` null, or a variant that does)? */
export function acceptsNull(raw: JsonSchema, root: JsonSchema): boolean {
  const s = mergeAllOf(resolveRef(raw, root), root);
  if (s.type === 'null' || (Array.isArray(s.type) && s.type.includes('null'))) return true;
  if (s.const === null || (s.enum?.includes(null) ?? false)) return true;
  const variants = variantsOf(s);
  return variants ? variants.some((v) => acceptsNull(v, root)) : false;
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

/** Shallow `allOf` merge (properties/required unioned, scalar keywords from the last member). Memoised. */
export function mergeAllOf(schema: JsonSchema, root: JsonSchema): JsonSchema {
  if (!schema.allOf) return schema;
  return allOfMemo(root, schema, () => mergeAllOfUncached(schema, root));
}

function mergeAllOfUncached(schema: JsonSchema, root: JsonSchema): JsonSchema {
  const { allOf, ...base } = schema;
  let out: JsonSchema = { ...base };
  for (const raw of allOf ?? []) {
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

/**
 * Default label of each property: its schema title — or, when siblings share that title (a shared sub-schema without
 * its own title, e.g. ACL `source`/`destination` both "Address match"), the humanized key, so labels stay distinct.
 */
export function propertyTitles(entries: readonly PropertyEntry[]): Record<string, string> {
  const count = new Map<string, number>();
  for (const e of entries) count.set(titleOf(e.schema, e.key), (count.get(titleOf(e.schema, e.key)) ?? 0) + 1);
  return Object.fromEntries(
    entries.map((e) => {
      const t = titleOf(e.schema, e.key);
      return [e.key, (count.get(t) ?? 0) > 1 ? humanize(e.key) : t];
    }),
  );
}

function cloneJson<T>(v: T): T {
  return v === undefined ? v : (JSON.parse(JSON.stringify(v)) as T);
}

/** How defaults are built for absent members. */
export interface DefaultsOptions {
  /**
   * `true` (what `<SchemaForm>` uses): an optional object member without a schema `default` (`isOptionalObject`)
   * stays **absent** until the user switches it on — its presence carries meaning (`dhcpClient`, `routing.ospf`;
   * P08-questions Q2). `false`/omitted (the historical behaviour): it is filled with its properties' defaults.
   */
  presence?: boolean;
}

/** The options `<SchemaForm>` builds every fresh value with (initial value, added rows, variant switch, presence on). */
export const FORM_DEFAULTS: Readonly<DefaultsOptions> = { presence: true };

/**
 * A non-required, plain (non-record, non-union) object member with at least one property and no `default`:
 * absent and present mean different things, so the form renders a presence switch instead of inventing it.
 */
export function isOptionalObject(raw: JsonSchema, required: boolean, root: JsonSchema): boolean {
  if (required) return false;
  const schema = mergeAllOf(resolveRef(raw, root), root);
  return (
    typeOf(schema) === 'object' &&
    !isRecordSchema(schema) &&
    !variantsOf(schema) &&
    schema.default === undefined &&
    Object.keys(schema.properties ?? {}).length > 0
  );
}

/** Starting value for a fresh instance: `default`, `const`, or an object filled with its properties' defaults. */
export function defaultValueFor(raw: JsonSchema, root: JsonSchema, options: DefaultsOptions = {}): unknown {
  const schema = mergeAllOf(resolveRef(raw, root), root);
  if (schema.default !== undefined) return cloneJson(schema.default);
  if (schema.const !== undefined) return cloneJson(schema.const);
  const t = typeOf(schema);
  if (t === 'object') {
    if (isRecordSchema(schema)) return {};
    const out: Record<string, unknown> = {};
    for (const e of sortedProperties(schema, root)) {
      if (options.presence && isOptionalObject(e.schema, e.required, root)) continue;
      const v = defaultValueFor(e.schema, root, options);
      if (v !== undefined) out[e.key] = v;
    }
    return out;
  }
  if (t === 'array') return [];
  const variants = variantsOf(schema);
  if (variants?.[0]) return defaultValueFor(variants[0], root, options);
  return undefined;
}

/** `x-vrx-ui.itemKey` of an array schema (string members only), or `[]`. */
export function itemKeyOf(hints: UiHints): string[] {
  const k: unknown = hints.itemKey;
  return Array.isArray(k) ? k.filter((m): m is string => typeof m === 'string') : [];
}

/**
 * Widgets whose values are identifiers or technical tokens: monospace, and LTR inside an RTL page so bidi never
 * reorders them (review P07a L7, RTL-1). The pickers without a data source yet (vrf/object/host-interface/secret-ref)
 * render as text inputs but keep this treatment.
 */
export const IDENTIFIER_WIDGETS: ReadonlySet<string> = new Set([
  'cidr',
  'ip',
  'mac',
  'mono',
  'interface-picker',
  'host-interface-picker',
  'vrf-picker',
  'object-picker',
  'tag-picker',
  'secret-ref',
  'port-range',
  'ip-range',
  'time',
  'datetime',
  'timezone',
  'timezone-picker',
  'color',
]);

/** JSON Schema string formats that are identifiers (never prose). `idn-*` may hold RTL scripts and are left out. */
const LTR_FORMATS: ReadonlySet<string> = new Set([
  'ipv4',
  'ipv6',
  'cidrv4',
  'cidrv6',
  'mac',
  'hostname',
  'email',
  'uri',
  'url',
  'uuid',
  'date-time',
  'date',
  'time',
  'duration',
]);

/**
 * Can this JSON Schema `pattern` match only ASCII? A conservative scan: `.`, a negated class, `\D` `\S` `\W` `\p{…}`,
 * or a non-ASCII literal/escape makes it `false`. A value restricted to ASCII (object names, host names, user
 * names) is an identifier, and cannot hold Persian text anyway, so it renders LTR in an RTL page (RTL-1).
 */
export function isAsciiOnlyPattern(pattern: string | undefined): boolean {
  if (!pattern) return false;
  let inClass = false;
  for (let i = 0; i < pattern.length; i++) {
    const c = pattern[i]!;
    if (c.charCodeAt(0) > 0x7e) return false;
    if (c === '\\') {
      const n = pattern[i + 1];
      if (n === undefined || 'DSWpPk'.includes(n)) return false;
      if (n === 'u' || n === 'x') {
        const hex = pattern.slice(i + 2, i + (n === 'u' ? 6 : 4));
        if (!/^[0-9a-fA-F]{2,4}$/.test(hex) || parseInt(hex, 16) > 0x7e) return false;
        i += 1 + hex.length;
      } else {
        i += 1;
      }
      continue;
    }
    if (inClass) {
      if (c === ']') inClass = false;
      continue;
    }
    if (c === '[') {
      if (pattern[i + 1] === '^') return false;
      inClass = true;
    } else if (c === '.') {
      return false;
    }
  }
  return true;
}

/** Should this string value render LTR in an RTL page? Identifier widget, identifier format, or ASCII-only pattern. */
export function isLtrString(schema: JsonSchema, widget: string | undefined): boolean {
  if (widget !== undefined && IDENTIFIER_WIDGETS.has(widget)) return true;
  if (typeof schema.format === 'string' && LTR_FORMATS.has(schema.format)) return true;
  return isAsciiOnlyPattern(typeof schema.pattern === 'string' ? schema.pattern : undefined);
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
