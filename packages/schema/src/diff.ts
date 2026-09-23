import { deepEqual, isPlainObject } from './json.js';
import { jsonPointer } from './pointer.js';

export type DiffOp = 'add' | 'remove' | 'replace';

/** One change from `a` to `b`. `pointer` is RFC 6901; ops follow RFC 6902 naming. */
export interface Change {
  op: DiffOp;
  pointer: string;
  /** Previous value — present for `remove` and `replace`. */
  from?: unknown;
  /** New value — present for `add` and `replace`. */
  to?: unknown;
}

/**
 * Structural diff of two JSON documents (candidate ↔ running) as JSON-pointer changes. Pure.
 *
 * - Objects are recursed; keys are visited in sorted order so the output is deterministic.
 * - Arrays and scalars are leaves: a changed array yields ONE `replace` at the array's pointer.
 * - A key whose value is `undefined` counts as absent (JSON has no undefined).
 * - Identical inputs yield `[]`; two different scalars at the root yield one change with pointer `''`.
 *
 * TODO(P02a): element-wise diffs for keyed collections (routing.static, management.users, acl.attachments …)
 * so a single edited route does not show up as a whole-array replace.
 */
export function diff(a: unknown, b: unknown, base = ''): Change[] {
  if (isPlainObject(a) && isPlainObject(b)) {
    const changes: Change[] = [];
    const keys = [...new Set([...Object.keys(a), ...Object.keys(b)])].sort();
    for (const key of keys) {
      const pointer = base + jsonPointer(key);
      const from = a[key];
      const to = b[key];
      if (from !== undefined && to === undefined) changes.push({ op: 'remove', pointer, from });
      else if (from === undefined && to !== undefined) changes.push({ op: 'add', pointer, to });
      else if (from !== undefined && to !== undefined) changes.push(...diff(from, to, pointer));
    }
    return changes;
  }
  if (deepEqual(a, b)) return [];
  return [{ op: 'replace', pointer: base, from: a, to: b }];
}
