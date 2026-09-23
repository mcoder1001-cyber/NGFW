import { isPlainObject } from './json.js';
import { parsePointer } from './pointer.js';

/**
 * RFC 7386 JSON Merge Patch — the algorithm behind `PATCH /api/v1/config/{path}`.
 *
 * - If `patch` is not an object (scalar, array, null) it replaces `target` wholesale.
 * - Otherwise each patch member is applied recursively; `null` removes the member from the target.
 * - Pure: `target` and `patch` are never mutated (objects along the patched path are shallow-copied).
 * - `undefined` members of the patch are ignored — they cannot occur in JSON.
 *
 * Validation is a separate step: apply the patch to the candidate, then `validateConfig()` the result and answer
 * problem+json with the failing pointers.
 */
export function mergePatch(target: unknown, patch: unknown): unknown {
  if (!isPlainObject(patch)) return patch;
  const result: Record<string, unknown> = isPlainObject(target) ? { ...target } : {};
  for (const [key, value] of Object.entries(patch)) {
    if (value === null) delete result[key];
    else if (value !== undefined) result[key] = mergePatch(result[key], value);
  }
  return result;
}

/**
 * Apply `patch` (RFC 7386) to the node at RFC 6901 `pointer` inside `document` — `PATCH /api/v1/config/{path}` with
 * `{path}` = the pointer. Missing objects along the path are created; array segments must be an existing index or
 * the array's length (append). Pure. Throws only on an invalid pointer or a non-numeric array index.
 */
export function mergePatchAt(document: unknown, pointer: string, patch: unknown): unknown {
  return patchAt(document, parsePointer(pointer), patch);
}

function patchAt(target: unknown, segments: readonly string[], patch: unknown): unknown {
  const head = segments[0];
  if (head === undefined) return mergePatch(target, patch);
  const rest = segments.slice(1);
  if (Array.isArray(target)) {
    const index = Number(head);
    if (!/^(?:0|[1-9][0-9]*)$/.test(head) || index > target.length) {
      throw new Error(`invalid array index '${head}' in JSON pointer (array has ${target.length} items)`);
    }
    const copy: unknown[] = [...target];
    copy[index] = patchAt(target[index], rest, patch);
    return copy;
  }
  const copy: Record<string, unknown> = isPlainObject(target) ? { ...target } : {};
  copy[head] = patchAt(copy[head], rest, patch);
  return copy;
}
