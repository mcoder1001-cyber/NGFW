/**
 * RFC 6901 JSON Pointer helpers. Every `pointer` in this package (semantic issues, diff changes, RFC 9457
 * problem+json bodies) is built with these — VPP interface names contain `/` (TenGigabitEthernet0/0/0) and must
 * be escaped as `~1`, so never concatenate pointers by hand.
 */

/** Escape one reference token: `~` → `~0`, `/` → `~1`. */
export function escapePointerSegment(segment: string): string {
  return segment.replace(/~/g, '~0').replace(/\//g, '~1');
}

/** Inverse of {@link escapePointerSegment} (order matters: `~1` first, then `~0`). */
export function unescapePointerSegment(segment: string): string {
  return segment.replace(/~1/g, '/').replace(/~0/g, '~');
}

/**
 * Build a pointer from path segments. `jsonPointer()` is the whole document (`''`).
 * `jsonPointer('interfaces', 'TenGigabitEthernet0/0/0', 'mtu')` → `/interfaces/TenGigabitEthernet0~10~10/mtu`
 */
export function jsonPointer(...segments: readonly (string | number)[]): string {
  return segments.map((s) => '/' + escapePointerSegment(String(s))).join('');
}

/** Split a pointer into unescaped segments. Throws on a pointer that is neither empty nor starts with `/`. */
export function parsePointer(pointer: string): string[] {
  if (pointer === '') return [];
  if (!pointer.startsWith('/')) {
    throw new Error(`invalid JSON pointer '${pointer}': must be empty or start with '/'`);
  }
  return pointer.slice(1).split('/').map(unescapePointerSegment);
}
