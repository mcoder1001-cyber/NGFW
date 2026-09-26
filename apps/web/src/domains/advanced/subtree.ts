import { jsonPointer } from '@ngfw/schema';

/** True when `pointer` is `base`, inside it, or an ancestor of it (an issue reported higher up still applies here). */
export function pointerInSubtree(pointer: string, base: string): boolean {
  return pointer === base || pointer.startsWith(`${base}/`) || base.startsWith(`${pointer}/`);
}

/** The absolute RFC 6901 pointer of a config path (`domainKey`, `segments`). */
export function absolutePointer(domainKey: string, segments: readonly string[]): string {
  return jsonPointer(domainKey, ...segments);
}

/** Is `domainKey` one of a pending/last commit's unimplemented domains (`ApplySummary.notApplied`, D-P06-15)? */
export function notAppliedForDomain(notApplied: readonly string[], domainKey: string): boolean {
  return notApplied.includes(domainKey);
}

/** Items (diff changes, `agent.unsupported-field` warnings, …) whose pointer is at or under the subtree. */
export function inSubtree<T extends { pointer: string }>(items: readonly T[], base: string): T[] {
  return items.filter((i) => pointerInSubtree(i.pointer, base));
}
