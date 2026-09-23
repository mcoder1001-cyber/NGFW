import { z } from 'zod';
import {
  defaultValueFor,
  isRecordSchema,
  mergeAllOf,
  parsePointer,
  recordValueSchema,
  resolveRef,
  typeOf,
  variantsOf,
} from './schema-utils.js';
import type { JsonPath, JsonSchema } from './types.js';

/**
 * The form keeps records (`additionalProperties` objects keyed by user-chosen names such as
 * `TenGigabitEthernet0/0/0.100`) as ordered entry arrays, because react-hook-form paths are dot-separated
 * and cannot address keys that contain `.`. `toFormValue` / `fromFormValue` convert both ways, and
 * `formPathFor` maps a JSON path (Zod issue path or RFC 9457 pointer) onto the current form layout.
 */
export interface RecordEntry {
  key: string;
  value: unknown;
}

const compiled = new WeakMap<JsonSchema, z.ZodType>();

function withDefs(schema: JsonSchema, root: JsonSchema): JsonSchema {
  if (schema === root || !root.$defs || schema.$defs) return schema;
  return { ...schema, $defs: root.$defs };
}

/**
 * `z.fromJSONSchema` enforces only standard formats (ipv4, ipv6, email, uri, hostname, …). Zod's own
 * `toJSONSchema` emits a `pattern` next to non-standard formats such as `cidrv4`, so generated schemas
 * validate anyway; a hand-written `format` without `pattern` gets Zod's regex for that format here.
 * `pattern → format` is remembered so a pattern failure can still produce the format-specific message.
 */
const formatByPattern = new Map<string, string>();

export function formatForPattern(pattern: string | undefined): string | undefined {
  if (!pattern) return undefined;
  const m = /^\/(.*)\/[a-z]*$/s.exec(pattern);
  return formatByPattern.get(m ? m[1]! : pattern);
}

const KNOWN_FORMAT_REGEXES: Record<string, RegExp | undefined> = {
  cidrv4: z.regexes.cidrv4,
  cidrv6: z.regexes.cidrv6,
  ipv4: z.regexes.ipv4,
  ipv6: z.regexes.ipv6,
  mac: z.regexes.mac(),
  hostname: z.regexes.hostname,
};

function augmentFormats(node: unknown): unknown {
  if (Array.isArray(node)) return node.map(augmentFormats);
  if (node === null || typeof node !== 'object') return node;
  const s = node as JsonSchema;
  let out: JsonSchema = s;
  if (typeof s.format === 'string') {
    if (typeof s.pattern === 'string') formatByPattern.set(s.pattern, s.format);
    else {
      const re = KNOWN_FORMAT_REGEXES[s.format];
      if (re) {
        formatByPattern.set(re.source, s.format);
        out = { ...s, pattern: re.source };
      }
    }
  }
  let changed = out !== s;
  const copy: JsonSchema = { ...out };
  for (const [k, v] of Object.entries(out)) {
    if (v !== null && typeof v === 'object') {
      const nv = augmentFormats(v);
      if (nv !== v) {
        copy[k] = nv;
        changed = true;
      }
    }
  }
  return changed ? copy : out;
}

/** JSON Schema → Zod (Zod 4 `fromJSONSchema`); memoised per schema object. Falls back to `unknown`. */
export function compile(schema: JsonSchema, root: JsonSchema): z.ZodType {
  let zs = compiled.get(schema);
  if (!zs) {
    try {
      const prepared = augmentFormats(withDefs(schema, root)) as z.core.JSONSchema.JSONSchema;
      zs = z.fromJSONSchema(prepared);
    } catch {
      zs = z.unknown();
    }
    compiled.set(schema, zs);
  }
  return zs;
}

/** Index of the first variant that accepts `value` (JSON shape), or -1. */
export function matchVariant(variants: JsonSchema[], value: unknown, root: JsonSchema): number {
  if (value === undefined) return -1;
  for (let i = 0; i < variants.length; i++) {
    const v = resolveRef(variants[i]!, root);
    if (compile(v, root).safeParse(value).success) return i;
  }
  return -1;
}

/** Index of the first variant whose JSON conversion of the *form* value is accepted, or -1. */
export function matchVariantForm(variants: JsonSchema[], formValue: unknown, root: JsonSchema): number {
  if (formValue === undefined) return -1;
  for (let i = 0; i < variants.length; i++) {
    const v = resolveRef(variants[i]!, root);
    if (compile(v, root).safeParse(fromFormValue(v, formValue, root)).success) return i;
  }
  return -1;
}

function isPlainObject(v: unknown): v is Record<string, unknown> {
  return v !== null && typeof v === 'object' && !Array.isArray(v);
}

export function toFormValue(raw: JsonSchema, value: unknown, root: JsonSchema): unknown {
  const schema = mergeAllOf(resolveRef(raw, root), root);
  const variants = variantsOf(schema);
  if (variants && typeOf(schema) === undefined) {
    const i = matchVariant(variants, value, root);
    return i < 0 ? value : toFormValue(variants[i]!, value, root);
  }
  const t = typeOf(schema);
  if (t === 'object') {
    if (isRecordSchema(schema)) {
      if (value === undefined || value === null) return [];
      if (!isPlainObject(value)) return value;
      const vs = recordValueSchema(schema);
      return Object.entries(value).map(([key, v]): RecordEntry => ({ key, value: toFormValue(vs, v, root) }));
    }
    if (!isPlainObject(value)) return value;
    const out: Record<string, unknown> = { ...value };
    for (const [k, ps] of Object.entries(schema.properties ?? {})) {
      if (k in out) out[k] = toFormValue(ps, out[k], root);
    }
    return out;
  }
  if (t === 'array' && Array.isArray(value) && schema.items) {
    const items = schema.items;
    return value.map((v) => toFormValue(items, v, root));
  }
  return value;
}

export function fromFormValue(raw: JsonSchema, formValue: unknown, root: JsonSchema): unknown {
  const schema = mergeAllOf(resolveRef(raw, root), root);
  const variants = variantsOf(schema);
  if (variants && typeOf(schema) === undefined) {
    for (const rawVariant of variants) {
      const v = resolveRef(rawVariant, root);
      const candidate = fromFormValue(v, formValue, root);
      if (compile(v, root).safeParse(candidate).success) return candidate;
    }
    // Nothing validates yet (user still typing): convert with the structurally closest variant.
    const structural = variants.find((v) => {
      const rv = resolveRef(v, root);
      return typeOf(rv) === 'object' && (isRecordSchema(rv) ? Array.isArray(formValue) : isPlainObject(formValue));
    });
    return structural ? fromFormValue(structural, formValue, root) : formValue;
  }
  const t = typeOf(schema);
  if (t === 'object') {
    if (isRecordSchema(schema)) {
      if (!Array.isArray(formValue)) return formValue;
      const vs = recordValueSchema(schema);
      const out: Record<string, unknown> = {};
      for (const entry of formValue as RecordEntry[]) {
        if (!isPlainObject(entry)) continue;
        out[String(entry.key ?? '')] = fromFormValue(vs, entry.value, root);
      }
      return out;
    }
    if (!isPlainObject(formValue)) return formValue;
    const out: Record<string, unknown> = { ...formValue };
    for (const [k, ps] of Object.entries(schema.properties ?? {})) {
      if (k in out) {
        const v = fromFormValue(ps, out[k], root);
        if (v === undefined) delete out[k];
        else out[k] = v;
      }
    }
    return out;
  }
  if (t === 'array' && Array.isArray(formValue) && schema.items) {
    const items = schema.items;
    return formValue.map((v) => fromFormValue(items, v, root));
  }
  return formValue;
}

/**
 * JSON path → dotted react-hook-form path for the *current* form value (record keys become entry indexes),
 * or `null` when the path cannot be located (unknown property, missing entry, opaque JSON widget).
 */
export function formPathFor(
  raw: JsonSchema,
  formValue: unknown,
  path: JsonPath,
  root: JsonSchema,
): string | null {
  const segs: string[] = [];
  let schema = mergeAllOf(resolveRef(raw, root), root);
  let value = formValue;
  for (const seg of path) {
    const variants = variantsOf(schema);
    if (variants && typeOf(schema) === undefined) {
      const i = matchVariantForm(variants, value, root);
      const pick = i < 0 ? variants.find((v) => typeOf(resolveRef(v, root)) === 'object') : variants[i];
      if (!pick) return null;
      schema = mergeAllOf(resolveRef(pick, root), root);
    }
    const t = typeOf(schema);
    if (t === 'object' && isRecordSchema(schema)) {
      if (!Array.isArray(value)) return null;
      const idx = value.findIndex((e) => isPlainObject(e) && String(e.key) === String(seg));
      if (idx < 0) return null;
      segs.push(String(idx), 'value');
      value = (value[idx] as RecordEntry).value;
      schema = mergeAllOf(resolveRef(recordValueSchema(schema), root), root);
    } else if (t === 'object') {
      const key = String(seg);
      const ps = schema.properties?.[key];
      segs.push(key);
      value = isPlainObject(value) ? value[key] : undefined;
      schema = ps ? mergeAllOf(resolveRef(ps, root), root) : {};
    } else if (t === 'array') {
      segs.push(String(seg));
      value = Array.isArray(value) ? value[Number(seg)] : undefined;
      schema = schema.items ? mergeAllOf(resolveRef(schema.items, root), root) : {};
    } else {
      return null;
    }
  }
  return segs.join('.');
}

export function pointerToFormPath(
  schema: JsonSchema,
  formValue: unknown,
  pointer: string,
  root: JsonSchema,
): string | null {
  return formPathFor(schema, formValue, parsePointer(pointer), root);
}

export interface RecordKeyIssue {
  /** RHF path of the offending `key` input. */
  path: string;
  type: 'duplicate' | 'empty';
}

/** Empty or duplicate record keys — invisible to the JSON validator once entries collapse into an object. */
export function findRecordKeyIssues(
  raw: JsonSchema,
  formValue: unknown,
  root: JsonSchema,
  base = '',
): RecordKeyIssue[] {
  const schema = mergeAllOf(resolveRef(raw, root), root);
  const out: RecordKeyIssue[] = [];
  const variants = variantsOf(schema);
  if (variants && typeOf(schema) === undefined) {
    const i = matchVariantForm(variants, formValue, root);
    const pick = i < 0 ? variants.find((v) => typeOf(resolveRef(v, root)) === 'object') : variants[i];
    return pick ? findRecordKeyIssues(pick, formValue, root, base) : out;
  }
  const t = typeOf(schema);
  const join = (child: string | number) => (base === '' ? String(child) : `${base}.${child}`);
  if (t === 'object' && isRecordSchema(schema)) {
    if (!Array.isArray(formValue)) return out;
    const seen = new Map<string, number>();
    const vs = recordValueSchema(schema);
    (formValue as RecordEntry[]).forEach((entry, i) => {
      if (!isPlainObject(entry)) return;
      const key = String(entry.key ?? '');
      if (key === '') out.push({ path: join(`${i}.key`), type: 'empty' });
      else {
        const first = seen.get(key);
        if (first !== undefined) {
          out.push({ path: join(`${i}.key`), type: 'duplicate' });
          if (!out.some((x) => x.path === join(`${first}.key`))) {
            out.push({ path: join(`${first}.key`), type: 'duplicate' });
          }
        } else seen.set(key, i);
      }
      out.push(...findRecordKeyIssues(vs, entry.value, root, join(`${i}.value`)));
    });
    return out;
  }
  if (t === 'object' && isPlainObject(formValue)) {
    for (const [k, ps] of Object.entries(schema.properties ?? {})) {
      if (k in formValue) out.push(...findRecordKeyIssues(ps, formValue[k], root, join(k)));
    }
    return out;
  }
  if (t === 'array' && Array.isArray(formValue) && schema.items) {
    const items = schema.items;
    formValue.forEach((v, i) => out.push(...findRecordKeyIssues(items, v, root, join(i))));
  }
  return out;
}

function isEmptyContainer(v: unknown): boolean {
  return (Array.isArray(v) && v.length === 0) || (isPlainObject(v) && Object.keys(v).length === 0);
}

/**
 * Fill schema `default`/`const` values into the gaps of an existing JSON value (objects only, recursively),
 * so the form shows what the server will apply. Empty containers are not invented for absent keys.
 */
export function withDefaults(raw: JsonSchema, value: unknown, root: JsonSchema): unknown {
  const schema = mergeAllOf(resolveRef(raw, root), root);
  if (value === undefined) return defaultValueFor(schema, root);
  const variants = variantsOf(schema);
  if (variants && typeOf(schema) === undefined) {
    const i = matchVariant(variants, value, root);
    return i < 0 ? value : withDefaults(variants[i]!, value, root);
  }
  const t = typeOf(schema);
  if (t === 'object' && isPlainObject(value)) {
    if (isRecordSchema(schema)) {
      const vs = recordValueSchema(schema);
      return Object.fromEntries(Object.entries(value).map(([k, v]) => [k, withDefaults(vs, v, root)]));
    }
    const out: Record<string, unknown> = { ...value };
    for (const [k, ps] of Object.entries(schema.properties ?? {})) {
      if (k in out) out[k] = withDefaults(ps, out[k], root);
      else {
        const d = defaultValueFor(ps, root);
        if (d !== undefined && !isEmptyContainer(d)) out[k] = d;
      }
    }
    return out;
  }
  if (t === 'array' && Array.isArray(value) && schema.items) {
    const items = schema.items;
    return value.map((v) => withDefaults(items, v, root));
  }
  return value;
}
