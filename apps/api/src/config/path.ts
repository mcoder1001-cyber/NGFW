import { escapePointerSegment, ROOT_KEYS, unescapePointerSegment } from '@ngfw/schema';
import { problems } from '../common/problem.js';

/**
 * `/api/v1/config/{path}` → RFC 6901 pointer. The path IS the pointer: segments are URL-decoded and then read as
 * pointer tokens, so `TenGigabitEthernet0/0/0` is written `TenGigabitEthernet0~10~10` (or `TenGigabitEthernet0%2F0%2F0`
 * — a decoded `/` inside one URL segment is escaped for you). The first segment must be a top-level key (ROOT_KEYS),
 * so new schema domains need no new controller (P06 §5).
 */
export function pointerFromUrl(url: string, prefix: string): string {
  const path = url.split('?')[0] ?? '';
  if (!path.startsWith(prefix)) throw problems.notFound(`no configuration path in '${path}'`);
  const rest = path.slice(prefix.length).replace(/^\/+/, '').replace(/\/+$/, '');
  if (rest === '') return '';
  const segments = rest.split('/').map((raw) => {
    let decoded: string;
    try {
      decoded = decodeURIComponent(raw);
    } catch {
      throw problems.badRequest(`invalid URL encoding in '${raw}'`);
    }
    // a decoded '/' belongs to the segment (percent-encoded in the URL): keep it inside by pointer-escaping it
    return decoded
      .split('/')
      .map((p, i, all) => (all.length > 1 ? escapePointerSegment(p) : p))
      .join('~1');
  });
  const first = unescapePointerSegment(segments[0] as string);
  if (!(ROOT_KEYS as readonly string[]).includes(first)) {
    throw problems.notFound(`'${first}' is not a configuration domain (${ROOT_KEYS.join(', ')})`);
  }
  for (const s of segments) {
    if (/~(?![01])/.test(s)) throw problems.badRequest(`invalid JSON pointer escape in '${s}'`);
  }
  return '/' + segments.join('/');
}
