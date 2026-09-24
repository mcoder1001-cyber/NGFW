import {
  FORBIDDEN_KEYS,
  isPlainObject,
  jsonPointer,
  MergePatchError,
  parsePointer,
} from '@ngfw/schema';
import { createHash } from 'node:crypto';

/**
 * Pointer operations the schema package does not ship (it has mergePatchAt): read, replace and remove the node at an
 * RFC 6901 pointer. Pure — inputs are never mutated; objects/arrays along the path are shallow-copied. Invalid paths
 * throw {@link MergePatchError} (→ 400 with pointer, D-070); a missing node on read/remove is `undefined` / NotFound.
 */

export class PointerNotFoundError extends Error {
  constructor(readonly pointer: string) {
    super(`nothing at '${pointer}'`);
    this.name = 'PointerNotFoundError';
  }
}

function assertSafe(segment: string, path: readonly string[]): void {
  if (FORBIDDEN_KEYS.includes(segment)) {
    throw new MergePatchError(`the key '${segment}' is not allowed`, jsonPointer(...path, segment));
  }
}

function arrayIndex(
  head: string,
  arr: readonly unknown[],
  path: readonly string[],
  allowAppend: boolean,
): number {
  const index = Number(head);
  const max = allowAppend ? arr.length : arr.length - 1;
  if (!/^(?:0|[1-9][0-9]*)$/.test(head) || index > max) {
    throw new MergePatchError(
      `invalid array index '${head}' in JSON pointer (array has ${arr.length} items)`,
      jsonPointer(...path, head),
    );
  }
  return index;
}

/** The value at `pointer`, or `undefined` when any segment is missing. */
export function getAt(doc: unknown, pointer: string): unknown {
  let node: unknown = doc;
  for (const seg of parsePointer(pointer)) {
    if (Array.isArray(node)) {
      if (!/^(?:0|[1-9][0-9]*)$/.test(seg)) return undefined;
      node = node[Number(seg)];
    } else if (isPlainObject(node) && Object.hasOwn(node, seg)) {
      node = node[seg];
    } else return undefined;
  }
  return node;
}

/** Replace (or create) the node at `pointer` with `value`; missing objects along the path are created. */
export function setAt(doc: unknown, pointer: string, value: unknown): unknown {
  return set(doc, parsePointer(pointer), value, []);
}

function set(target: unknown, segs: readonly string[], value: unknown, path: string[]): unknown {
  const head = segs[0];
  if (head === undefined) return value;
  const rest = segs.slice(1);
  if (Array.isArray(target)) {
    const i = arrayIndex(head, target, path, true);
    const copy = [...target];
    copy[i] = set(target[i], rest, value, [...path, head]);
    return copy;
  }
  assertSafe(head, path);
  const copy: Record<string, unknown> = isPlainObject(target) ? { ...target } : {};
  copy[head] = set(copy[head], rest, value, [...path, head]);
  return copy;
}

/** Remove the node at `pointer` (array items are spliced). Throws {@link PointerNotFoundError} when absent. */
export function removeAt(doc: unknown, pointer: string): unknown {
  const segs = parsePointer(pointer);
  if (segs.length === 0) throw new MergePatchError('cannot delete the whole document', '');
  return remove(doc, segs, [], pointer);
}

function remove(target: unknown, segs: readonly string[], path: string[], full: string): unknown {
  const head = segs[0] as string;
  const rest = segs.slice(1);
  if (Array.isArray(target)) {
    if (!/^(?:0|[1-9][0-9]*)$/.test(head) || Number(head) >= target.length) {
      throw new PointerNotFoundError(full);
    }
    const i = Number(head);
    const copy = [...target];
    if (rest.length === 0) copy.splice(i, 1);
    else copy[i] = remove(target[i], rest, [...path, head], full);
    return copy;
  }
  assertSafe(head, path);
  if (!isPlainObject(target) || !Object.hasOwn(target, head)) throw new PointerNotFoundError(full);
  const copy: Record<string, unknown> = { ...target };
  if (rest.length === 0) delete copy[head];
  else copy[head] = remove(target[head], rest, [...path, head], full);
  return copy;
}

/** JSON with object keys sorted recursively — the input of revision hashes. */
export function canonicalJson(value: unknown): string {
  return JSON.stringify(sortKeys(value));
}

function sortKeys(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(sortKeys);
  if (isPlainObject(value)) {
    const out: Record<string, unknown> = {};
    for (const k of Object.keys(value).sort()) out[k] = sortKeys(value[k]);
    return out;
  }
  return value;
}

export function sha256Hex(data: string): string {
  return createHash('sha256').update(data).digest('hex');
}

export function documentHash(doc: unknown): string {
  return sha256Hex(canonicalJson(doc));
}
