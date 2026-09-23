import { isPlainObject } from './json.js';

/**
 * RFC 7386 JSON Merge Patch — the algorithm behind `PATCH /api/v1/config/{path}`.
 *
 * - If `patch` is not an object (scalar, array, null) it replaces `target` wholesale.
 * - Otherwise each patch member is applied recursively; `null` removes the member from the target.
 * - Pure: `target` and `patch` are never mutated (objects along the patched path are shallow-copied).
 * - `undefined` members of the patch are ignored — they cannot occur in JSON.
 *
 * TODO(P02a): typed wrapper `mergePatchConfig(target: RootConfig, patch)` that re-validates the result with
 * RootConfig and reports the failing `pointer`.
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
