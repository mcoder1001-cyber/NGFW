import { z } from 'zod';
import { RootConfig } from './index.js';
import { isPlainObject } from './json.js';
import { jsonPointer } from './pointer.js';
import { X_VRX_UI, type UiHints } from './ui.js';

/**
 * Secret leaves of the configuration document (D-046, review M4). A leaf is secret when its schema carries
 * `x-vrx-ui.secret` (emitted as JSON Schema `writeOnly: true`) — today `management.users[].passwordHash`; every
 * other secret is a `secretRef` into the secret store and is not secret itself.
 *
 * The API uses these helpers wherever a document leaves the write path: `GET /config/**`, `GET /config/diff`
 * (diff redacted documents), revisions shown to clients, export, `audit_log.before/after`, and before
 * `DesiredState.fromJSON` (D-040). Round-trip rule (docs/contracts/schema.md): on PUT/PATCH an **absent**
 * write-only member keeps the stored value, an explicit `null` (merge-patch) clears it — so a client that GETs a
 * redacted document and PUTs it back never wipes a password hash.
 *
 * The walk follows the JSON Schema of `schema` (default `RootConfig`) alongside the document: properties, records
 * (`additionalProperties`), arrays (`items`) and every branch of `anyOf`/`oneOf`/`allOf`, so it also covers
 * secret fields that groups (b)/(c) add later without any change here.
 */

type Node = Record<string, unknown>;
type Path = readonly (string | number)[];

const cache = new WeakMap<z.ZodType, Node>();

function jsonSchemaOf(schema: z.ZodType): Node {
  let js = cache.get(schema);
  if (js === undefined) {
    js = z.toJSONSchema(schema, { io: 'output', unrepresentable: 'any' }) as Node;
    cache.set(schema, js);
  }
  return js;
}

/** `schema` with `$ref` resolved and composition keywords flattened into a list of alternatives. */
function alternatives(schema: unknown, root: Node, depth = 0): Node[] {
  if (!isPlainObject(schema) || depth > 32) return [];
  const ref = schema['$ref'];
  if (typeof ref === 'string' && ref.startsWith('#/')) {
    const target = ref
      .slice(2)
      .split('/')
      .reduce<unknown>((node, key) => (isPlainObject(node) ? node[key] : undefined), root);
    return alternatives(target, root, depth + 1);
  }
  const out: Node[] = [schema];
  for (const keyword of ['anyOf', 'oneOf', 'allOf'] as const) {
    const list = schema[keyword];
    if (Array.isArray(list)) for (const item of list) out.push(...alternatives(item, root, depth + 1));
  }
  return out;
}

function isSecret(schemas: readonly Node[]): boolean {
  return schemas.some(
    (s) => s['writeOnly'] === true || (s[X_VRX_UI] as UiHints | undefined)?.secret === true,
  );
}

function memberSchemas(schemas: readonly Node[], key: string, root: Node): Node[] {
  const out: Node[] = [];
  for (const s of schemas) {
    const properties = s['properties'];
    if (isPlainObject(properties) && Object.hasOwn(properties, key)) {
      out.push(...alternatives(properties[key], root));
    } else if (isPlainObject(s['additionalProperties'])) {
      out.push(...alternatives(s['additionalProperties'], root));
    }
  }
  return out;
}

function itemSchemas(schemas: readonly Node[], index: number, root: Node): Node[] {
  const out: Node[] = [];
  for (const s of schemas) {
    const prefix = s['prefixItems'];
    if (Array.isArray(prefix) && index < prefix.length) out.push(...alternatives(prefix[index], root));
    else out.push(...alternatives(s['items'], root));
  }
  return out;
}

/** Visit every secret leaf present in `value`; `found` gets its path, its parent object and its key. */
function visit(
  value: unknown,
  schemas: readonly Node[],
  root: Node,
  path: Path,
  found: (path: Path, parent: Record<string, unknown>, key: string) => void,
): void {
  if (Array.isArray(value)) {
    value.forEach((item, i) => visit(item, itemSchemas(schemas, i, root), root, [...path, i], found));
  } else if (isPlainObject(value)) {
    for (const [key, member] of Object.entries(value)) {
      const sub = memberSchemas(schemas, key, root);
      if (isSecret(sub)) found([...path, key], value, key);
      else visit(member, sub, root, [...path, key], found);
    }
  }
}

/** RFC 6901 pointers of the secret leaves present in `document` (e.g. `/management/users/0/passwordHash`). */
export function secretPointers(document: unknown, schema: z.ZodType = RootConfig): string[] {
  const root = jsonSchemaOf(schema);
  const pointers: string[] = [];
  visit(document, alternatives(root, root), root, [], (path) => pointers.push(jsonPointer(...path)));
  return pointers;
}

/**
 * A deep copy of `document` without its secret leaves — what GET, diff, revisions, export and the audit log
 * show. Pure; the input is not modified.
 */
export function redactSecrets<T>(document: T, schema: z.ZodType = RootConfig): T {
  const copy = structuredClone(document);
  const root = jsonSchemaOf(schema);
  visit(copy, alternatives(root, root), root, [], (_path, parent, key) => {
    delete parent[key];
  });
  return copy;
}
