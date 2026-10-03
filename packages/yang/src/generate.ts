import { z } from 'zod';
import { RootConfig, ROOT_KEYS, type RootKey } from '@ngfw/schema';

/**
 * F-restconf-yang: YANG 1.1 modules generated from the Zod root schema — one module per root key. The Zod schema is
 * the single source of truth (00-CONTEXT rule 5); this walks the JSON Schema `pnpm gen` already derives from it
 * (`io: 'input'`, so optional-with-default keys carry their `default`) and maps it to YANG statements:
 *
 *   strictObject (properties)         -> container
 *   record (additionalProperties=schema) -> list, key "name"  (the map key becomes the `name` key leaf)
 *   array of objects (x-ngfw-ui.itemKey) -> list keyed by those leaves
 *   array of objects (no itemKey)     -> keyless list + a documented deviation comment (YANG needs a key for config)
 *   array of scalars                  -> leaf-list
 *   enum                              -> type enumeration
 *   integer + min/max                 -> int32/uint32 + range
 *   number                            -> decimal64
 *   string + pattern                  -> type string + pattern (XSD-anchored; JS-only constructs dropped w/ a comment)
 *   union (anyOf)                     -> type union
 *   secret leaf (writeOnly)           -> nacm:default-deny-all (never returned by RESTCONF; enforced in the API too)
 *
 * The output is deterministic and checked in (`generated/*.yang`); a golden test fails CI on drift.
 */

export const ORG = 'NGFW';
export const CONTACT = 'https://ngfw.dev';
export const NS_BASE = 'urn:ngfw';
export const REVISION = '2026-01-01';

type Json = Record<string, unknown>;

const isObject = (v: unknown): v is Json => typeof v === 'object' && v !== null && !Array.isArray(v);

/** A YANG identifier: JSON names are already `[A-Za-z][\w-]*`-ish, but camelCase is legal in YANG identifiers. */
function ident(name: string): string {
  return /^[A-Za-z_][A-Za-z0-9_.-]*$/.test(name) ? name : `"${name}"`;
}

/** A double-quoted YANG string, escaped. */
function qstr(s: string): string {
  return `"${s.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`;
}

class Writer {
  private lines: string[] = [];
  constructor(private depth = 0) {}
  line(s = ''): void {
    this.lines.push(s === '' ? '' : '  '.repeat(this.depth) + s);
  }
  open(s: string): void {
    this.line(`${s} {`);
    this.depth++;
  }
  close(): void {
    this.depth--;
    this.line('}');
  }
  toString(): string {
    return this.lines.join('\n');
  }
}

function description(node: Json): string | undefined {
  const ui = isObject(node['x-ngfw-ui']) ? (node['x-ngfw-ui'] as Json) : {};
  const parts = [node['description'], ui['title'], ui['help']].filter(
    (x): x is string => typeof x === 'string' && x.length > 0,
  );
  // description wins; else the UI title, appending help when it adds information
  if (typeof node['description'] === 'string' && node['description']) return node['description'];
  if (typeof ui['title'] === 'string' && ui['title']) {
    return typeof ui['help'] === 'string' && ui['help'] ? `${ui['title']} — ${ui['help']}` : ui['title'];
  }
  return parts[0];
}

function emitDescription(w: Writer, node: Json): void {
  const d = description(node);
  if (d) w.line(`description ${qstr(d)};`);
}

/** JS regex source -> XSD pattern, or undefined when it uses constructs XSD cannot express. */
export function toXsdPattern(source: string): string | undefined {
  let p = source;
  if (p.startsWith('^')) p = p.slice(1);
  if (p.endsWith('$')) p = p.slice(0, -1);
  // XSD regex has no lookahead/lookbehind, backreferences, or anchors mid-pattern
  if (/\(\?[=!<]/.test(p) || /\\[bB]/.test(p) || /[$^]/.test(p)) return undefined;
  // `\/` is a JS escape for a literal slash; XSD does not define it, so use a bare slash.
  // XSD groups are always non-capturing, written `(...)`, so `(?:` becomes `(`.
  return p.replace(/\\\//g, '/').replace(/\(\?:/g, '(');
}

function integerType(node: Json): string {
  const min = typeof node['minimum'] === 'number' ? (node['minimum'] as number) : undefined;
  const max = typeof node['maximum'] === 'number' ? (node['maximum'] as number) : undefined;
  const base = min !== undefined && min >= 0 ? (max !== undefined && max > 0xffffffff ? 'uint64' : 'uint32') : 'int32';
  return base;
}

/** Emit the `type ...;` (or `type ... { ... }`) for a leaf-ish node. Returns nothing; writes to `w`. */
function emitType(w: Writer, node: Json): void {
  // union of scalars
  if (Array.isArray(node['anyOf'])) {
    const members = (node['anyOf'] as Json[]).filter(isObject);
    w.open('type union');
    for (const m of members) emitType(w, m);
    w.close();
    return;
  }
  if (Array.isArray(node['enum'])) {
    w.open('type enumeration');
    for (const v of node['enum'] as unknown[]) w.line(`enum ${qstr(String(v))};`);
    w.close();
    return;
  }
  // Zod numeric literals use JSON Schema number+const, even for integers.
  // Preserve the literal before the general number branch can widen it.
  const literal = node['const'];
  if (typeof literal === 'number') {
    if (!Number.isSafeInteger(literal)) {
      throw new Error(`YANG numeric literal must be a safe integer: ${literal}`);
    }
    const base = literal >= 0
      ? (literal > 0xffffffff ? 'uint64' : 'uint32')
      : (literal < -0x80000000 ? 'int64' : 'int32');
    w.open(`type ${base}`);
    w.line(`range ${qstr(String(literal))};`);
    w.close();
    return;
  }
  const t = node['type'];
  if (t === 'boolean') {
    w.line('type boolean;');
    return;
  }
  if (t === 'integer') {
    const base = integerType(node);
    const min = node['minimum'];
    const max = node['maximum'];
    if (typeof min === 'number' && typeof max === 'number') {
      w.open(`type ${base}`);
      w.line(`range ${qstr(`${min}..${max}`)};`);
      w.close();
    } else {
      w.line(`type ${base};`);
    }
    return;
  }
  if (t === 'number') {
    w.open('type decimal64');
    w.line('fraction-digits 6;');
    w.close();
    return;
  }
  // string (default)
  const pattern = typeof node['pattern'] === 'string' ? toXsdPattern(node['pattern'] as string) : undefined;
  const minLen = node['minLength'];
  const maxLen = node['maxLength'];
  const facets: string[] = [];
  if (pattern !== undefined) facets.push(`pattern ${qstr(pattern)};`);
  if (typeof minLen === 'number' || typeof maxLen === 'number') {
    const lo = typeof minLen === 'number' ? minLen : 0;
    const hi = typeof maxLen === 'number' ? String(maxLen) : 'max';
    facets.push(`length ${qstr(`${lo}..${hi}`)};`);
  }
  if (facets.length === 0) {
    if (typeof node['pattern'] === 'string' && pattern === undefined) {
      w.line('type string; // pattern dropped: not XSD-expressible');
    } else {
      w.line('type string;');
    }
    return;
  }
  w.open('type string');
  if (typeof node['pattern'] === 'string' && pattern === undefined) {
    w.line('// pattern dropped: not XSD-expressible');
  }
  for (const f of facets) w.line(f);
  w.close();
}

function isSecret(node: Json): boolean {
  return node['writeOnly'] === true;
}

function emitLeafExtras(w: Writer, node: Json, required: boolean): void {
  if (isSecret(node)) w.line('nacm:default-deny-all;');
  if ('default' in node) {
    const d = node['default'];
    if (typeof d === 'string' || typeof d === 'number' || typeof d === 'boolean') {
      w.line(`default ${qstr(String(d))};`);
    }
  } else if (required) {
    w.line('mandatory true;');
  }
  emitDescription(w, node);
}

/** Is this object node a record/map (value schema under additionalProperties, no fixed properties)? */
function recordValue(node: Json): Json | undefined {
  if (node['type'] !== 'object') return undefined;
  const ap = node['additionalProperties'];
  if (isObject(ap) && !isObject(node['properties'])) return ap;
  // record with a value that is itself a discriminated union etc.: still a map
  if (isObject(ap) && isObject(node['properties']) && Object.keys(node['properties'] as Json).length === 0)
    return ap;
  return undefined;
}

function itemKey(node: Json): string[] {
  const ui = isObject(node['x-ngfw-ui']) ? (node['x-ngfw-ui'] as Json) : {};
  const k = ui['itemKey'];
  return Array.isArray(k) ? (k as unknown[]).filter((x): x is string => typeof x === 'string') : [];
}

function isScalar(node: Json): boolean {
  return (
    Array.isArray(node['enum']) ||
    node['type'] === 'string' ||
    node['type'] === 'boolean' ||
    node['type'] === 'integer' ||
    node['type'] === 'number' ||
    Array.isArray(node['anyOf'])
  );
}

/** Emit a data node (`name`: `node`) into `w`. `required` = the parent lists it as required. */
function emitNode(w: Writer, name: string, node: Json, required: boolean): void {
  // object: container or list(record)
  const recVal = recordValue(node);
  if (recVal !== undefined) {
    w.open(`list ${ident(name)}`);
    w.line('key "name";');
    w.line('leaf name {');
    w.line('  type string;');
    w.line('  description "The map key.";');
    w.line('}');
    emitDescription(w, node);
    emitObjectChildren(w, recVal);
    w.close();
    return;
  }
  if (node['type'] === 'object' && isObject(node['properties'])) {
    w.open(`container ${ident(name)}`);
    emitDescription(w, node);
    emitObjectChildren(w, node);
    w.close();
    return;
  }
  if (node['type'] === 'array') {
    const items = isObject(node['items']) ? (node['items'] as Json) : {};
    if (isScalar(items)) {
      w.open(`leaf-list ${ident(name)}`);
      emitType(w, items);
      emitDescription(w, node);
      w.close();
      return;
    }
    // array of objects
    const keys = itemKey(node);
    const itemRecVal = recordValue(items);
    w.open(`list ${ident(name)}`);
    if (keys.length > 0) {
      w.line(`key ${qstr(keys.join(' '))};`);
    } else {
      w.line('// keyless: no itemKey meta — config lists need a key (documented deviation, RESTCONF orders by index)');
      w.line('ordered-by user;');
    }
    emitDescription(w, node);
    if (itemRecVal !== undefined) {
      // array whose items are themselves maps — rare; fall back to a nested map list
      emitNode(w, 'entry', items, false);
    } else if (isObject(items['properties']) || Array.isArray(items['anyOf'])) {
      emitObjectChildren(w, items);
    }
    w.close();
    return;
  }
  // scalar leaf
  w.open(`leaf ${ident(name)}`);
  emitType(w, node);
  emitLeafExtras(w, node, required);
  w.close();
}

/** Emit the child leaves/containers of an object node (its `properties`, honouring `required`). */
function emitObjectChildren(w: Writer, node: Json): void {
  // a discriminated union / union of objects flattens to the union of all members' properties
  if (Array.isArray(node['anyOf'])) {
    const merged: Record<string, Json> = {};
    const req = new Set<string>();
    for (const m of (node['anyOf'] as Json[]).filter(isObject)) {
      for (const [k, v] of Object.entries((m['properties'] as Json) ?? {})) {
        if (isObject(v)) merged[k] = v;
      }
      // a field required by every member stays required; keep it simple: never mandatory in a union
    }
    for (const [k, v] of Object.entries(merged)) emitNode(w, k, v, req.has(k));
    return;
  }
  const props = isObject(node['properties']) ? (node['properties'] as Json) : {};
  const required = new Set(Array.isArray(node['required']) ? (node['required'] as string[]) : []);
  for (const [k, v] of Object.entries(props)) {
    if (isObject(v)) emitNode(w, k, v, required.has(k));
  }
}

const anySecret = (json: string): boolean => json.includes('"writeOnly":true');

/** Generate the YANG module text for one root key. */
export function generateModule(key: RootKey): string {
  const schema = z.toJSONSchema(RootConfig.shape[key], { target: 'draft-2020-12', io: 'input' }) as Json;
  const moduleName = `ngfw-${key}`;
  const w = new Writer();
  w.open(`module ${moduleName}`);
  w.line('yang-version 1.1;');
  w.line(`namespace ${qstr(`${NS_BASE}:${key}`)};`);
  w.line(`prefix ${moduleName};`);
  w.line('');
  const secret = anySecret(JSON.stringify(schema));
  if (secret) {
    w.open('import ietf-netconf-acm');
    w.line('prefix nacm;');
    w.close();
    w.line('');
  }
  w.line(`organization ${qstr(ORG)};`);
  w.line(`contact ${qstr(CONTACT)};`);
  w.line(
    `description ${qstr(`Configuration of the '${key}' domain, generated from the NGFW Zod schema (F-restconf-yang).`)};`,
  );
  w.line('');
  w.open(`revision ${REVISION}`);
  w.line(`description ${qstr('Generated from the NGFW schema.')};`);
  w.close();
  w.line('');
  // top-level container mirrors the root key
  emitNode(w, key, schema, false);
  w.close();
  return w.toString() + '\n';
}

/** All modules, keyed by module name (`ngfw-<key>`). Deterministic in ROOT_KEYS order. */
export function generateModules(): Record<string, string> {
  const out: Record<string, string> = {};
  for (const key of ROOT_KEYS) out[`ngfw-${key}`] = generateModule(key);
  return out;
}

/** The ietf-yang-library module-set list content (RFC 8525), as data for the RESTCONF endpoint. */
export function moduleList(): { name: string; namespace: string; revision: string }[] {
  return ROOT_KEYS.map((key) => ({
    name: `ngfw-${key}`,
    namespace: `${NS_BASE}:${key}`,
    revision: REVISION,
  }));
}
