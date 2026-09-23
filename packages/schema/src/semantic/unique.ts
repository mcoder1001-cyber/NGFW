import { jsonPointer } from '../pointer.js';
import type { SemanticIssue } from './registry.js';

/**
 * Uniqueness of the natural key of list items (the `x-vrx-ui.itemKey` of the list, review M2). Every occurrence
 * after the first is reported at `path(item, index)` with `message(item)` plus the pointer of the first one.
 * `key` must normalise spellings that mean the same thing (`canonicalIp`, `canonicalPrefix`, lower-cased host
 * names) so that `2001:DB8::1` and `2001:db8::1` collide (D-049).
 */
export function duplicateIssues<T>(
  items: readonly T[],
  key: (item: T) => string,
  path: (item: T, index: number) => readonly (string | number)[],
  message: (item: T) => string,
): SemanticIssue[] {
  const issues: SemanticIssue[] = [];
  const first = new Map<string, number>();
  for (const [i, item] of items.entries()) {
    const k = key(item);
    const seen = first.get(k);
    if (seen === undefined) {
      first.set(k, i);
      continue;
    }
    issues.push({
      pointer: jsonPointer(...path(item, i)),
      message: `${message(item)} (first defined at ${jsonPointer(...path(items[seen] as T, seen))})`,
    });
  }
  return issues;
}
