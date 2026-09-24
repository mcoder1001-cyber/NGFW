import { isPlainObject } from './json.js';
import { jsonPointer, parsePointer } from './pointer.js';

/**
 * Keys a patch or a pointer may never contain (D-049): assigning them on a plain object changes its prototype
 * instead of adding a member, so a candidate document could carry attacker-controlled inherited properties.
 * No configuration key is named like this, so rejecting them costs nothing.
 */
export const FORBIDDEN_KEYS: readonly string[] = ['__proto__', 'constructor', 'prototype'];

/** A patch that cannot be applied; the API answers 400 problem+json with `pointer`. */
export class MergePatchError extends Error {
  constructor(
    message: string,
    /** RFC 6901 pointer (into the patched document) of the offending member or segment. */
    readonly pointer: string,
  ) {
    super(message);
    this.name = 'MergePatchError';
  }
}

function assertSafeKey(key: string, path: readonly string[]): void {
  if (FORBIDDEN_KEYS.includes(key)) {
    throw new MergePatchError(`the key '${key}' is not allowed`, jsonPointer(...path, key));
  }
}

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
  return merge(target, patch, []);
}

function merge(target: unknown, patch: unknown, path: readonly string[]): unknown {
  if (!isPlainObject(patch)) return patch;
  const result: Record<string, unknown> = isPlainObject(target) ? { ...target } : {};
  for (const [key, value] of Object.entries(patch)) {
    assertSafeKey(key, path);
    if (value === null) delete result[key];
    else if (value !== undefined) result[key] = merge(result[key], value, [...path, key]);
  }
  return result;
}

/**
 * Apply `patch` (RFC 7386) to the node at RFC 6901 `pointer` inside `document` — `PATCH /api/v1/config/{path}` with
 * `{path}` = the pointer. Missing objects along the path are created; array segments must be an existing index or
 * the array's length (append). Pure. Throws `Error` on a malformed pointer and {@link MergePatchError} on an
 * invalid array index or a forbidden key (`__proto__`, `constructor`, `prototype`) in the pointer or the patch.
 */
export function mergePatchAt(document: unknown, pointer: string, patch: unknown): unknown {
  return patchAt(document, parsePointer(pointer), patch, []);
}

function patchAt(
  target: unknown,
  segments: readonly string[],
  patch: unknown,
  path: readonly string[],
): unknown {
  const head = segments[0];
  if (head === undefined) return merge(target, patch, path);
  const rest = segments.slice(1);
  if (Array.isArray(target)) {
    const index = Number(head);
    if (!/^(?:0|[1-9][0-9]*)$/.test(head) || index > target.length) {
      throw new MergePatchError(
        `invalid array index '${head}' in JSON pointer (array has ${target.length} items)`,
        jsonPointer(...path, head),
      );
    }
    const copy: unknown[] = [...target];
    copy[index] = patchAt(target[index], rest, patch, [...path, head]);
    return copy;
  }
  assertSafeKey(head, path);
  const copy: Record<string, unknown> = isPlainObject(target) ? { ...target } : {};
  copy[head] = patchAt(copy[head], rest, patch, [...path, head]);
  return copy;
}
