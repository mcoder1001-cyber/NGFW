import { diff, isPlainObject, parsePointer } from '@ngfw/schema';
import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import { rootSchema } from '../schema/registry';
import type { DiffChange } from './DiffView';

type Node = Record<string, unknown>;

function resolve(schema: unknown, root: Node, depth = 0): Node | undefined {
  if (!isPlainObject(schema) || depth > 32) return undefined;
  const ref = schema['$ref'];
  if (typeof ref === 'string') {
    if (ref === '#') return root;
    const defs = root['$defs'];
    return ref.startsWith('#/$defs/') && isPlainObject(defs) ? resolve(defs[ref.slice(8)], root, depth + 1) : undefined;
  }
  return schema;
}

/** Alternatives of a schema node (`anyOf`/`oneOf`/`allOf` flattened), each resolved. */
function alternatives(schema: Node | undefined, root: Node): Node[] {
  if (!schema) return [];
  const out = [schema];
  for (const k of ['anyOf', 'oneOf', 'allOf'] as const) {
    const list = schema[k];
    if (Array.isArray(list)) for (const s of list) out.push(...alternatives(resolve(s, root), root));
  }
  return out;
}

/** The schema node at a JSON pointer (properties, records, array items), or undefined. */
export function schemaAt(root: Node, pointer: string): Node | undefined {
  let nodes: Node[] = [root];
  for (const seg of parsePointer(pointer)) {
    const next: Node[] = [];
    for (const n of nodes.flatMap((x) => alternatives(x, root))) {
      const props = n['properties'];
      if (isPlainObject(props) && seg in props) next.push(...alternatives(resolve(props[seg], root), root));
      else if (/^\d+$/.test(seg) && n['items'] !== undefined) next.push(...alternatives(resolve(n['items'], root), root));
      else if (isPlainObject(n['additionalProperties'])) next.push(...alternatives(resolve(n['additionalProperties'], root), root));
    }
    if (next.length === 0) return undefined;
    nodes = next;
  }
  return nodes[0];
}

/** `x-vrx-ui.itemKey` of the array at `pointer` (e.g. `['username']` for `management.users`). */
function itemKeyAt(root: Node, pointer: string): string[] | undefined {
  for (const n of alternatives(schemaAt(root, pointer), root)) {
    const key = (n['x-vrx-ui'] as { itemKey?: unknown } | undefined)?.itemKey;
    if (Array.isArray(key) && key.every((k) => typeof k === 'string')) return key as string[];
  }
  return undefined;
}

/**
 * P06 diffs treat arrays as leaves (contracts-v1, D-021): a changed `management.users` is ONE `replace` of the whole
 * list. For display, pair the items on the schema's `x-vrx-ui.itemKey` (as packages/schema/src/diff.ts suggests) and
 * show per-item adds/removes/field changes. Pointers of changed/added items use the index in the NEW list, removed
 * items their index in the old one. Arrays without an item key, with duplicate keys or with reordered items stay one
 * `replace`.
 */
export function refineChanges(changes: readonly DiffChange[], root: JsonSchema = rootSchema): DiffChange[] {
  const r = root as Node;
  return changes.flatMap((c) => refineOne(c, r, 0));
}

function refineOne(c: DiffChange, root: Node, depth: number): DiffChange[] {
  if (c.op !== 'replace' || !Array.isArray(c.from) || !Array.isArray(c.to) || depth > 8) return [c];
  const from = c.from as unknown[];
  const to = c.to as unknown[];
  if (![...from, ...to].every(isPlainObject)) return [c];
  const keyNames = itemKeyAt(root, c.pointer);
  if (!keyNames) return [c];
  const keyOf = (x: unknown) => JSON.stringify(keyNames.map((k) => (x as Node)[k]));
  const fromIdx = new Map<string, number>();
  for (const [i, x] of from.entries()) fromIdx.set(keyOf(x), i);
  const toKeys = to.map(keyOf);
  if (fromIdx.size !== from.length || new Set(toKeys).size !== to.length) return [c];
  // a reorder is a change of its own (order matters in some lists): keep the whole-list replace then
  const common = toKeys.filter((k) => fromIdx.has(k));
  const commonFromOrder = [...fromIdx.keys()].filter((k) => toKeys.includes(k));
  if (common.join('\n') !== commonFromOrder.join('\n')) return [c];
  const out: DiffChange[] = [];
  for (const [j, item] of to.entries()) {
    const i = fromIdx.get(toKeys[j]!);
    const base = `${c.pointer}/${j}`;
    if (i === undefined) out.push({ op: 'add', pointer: base, to: item });
    else for (const sub of diff(from[i], item)) out.push(...refineOne({ ...sub, pointer: base + sub.pointer }, root, depth + 1));
  }
  const toSet = new Set(toKeys);
  for (const [i, item] of from.entries()) if (!toSet.has(keyOf(item))) out.push({ op: 'remove', pointer: `${c.pointer}/${i}`, from: item });
  return out;
}
