/**
 * URL path segment for the generic candidate-config pointer routes (`GET/PATCH/PUT/DELETE /api/v1/config/{path}`,
 * `GET /api/v1/config/candidate/{path}`) — hand-written, NOT generated (review D-UDE-1). The server's wildcard
 * route already accepts a JSON pointer of any depth (`apps/api/src/config/config.controller.ts` `@Get('*')` and
 * friends, `apps/api/src/config/path.ts` `pointerFromUrl`): it splits the URL on real `/` characters, one per
 * pointer segment. The generated client's own path serializer (`openapi-fetch`) instead percent-encodes a path
 * parameter's *whole* value in one piece — including any `/` between segments, since this version has no
 * `allowReserved` for path params — which turns a multi-segment pointer into one opaque URL segment the server
 * cannot split back apart (it fails the `ROOT_KEYS` check on the first segment and 404s). Only that client-side
 * encoding was ever the limit; this restores the server's real behaviour to callers.
 *
 * `pointer` is an RFC 6901 JSON pointer with its own `~0`/`~1` escapes already applied (e.g.
 * `/interfaces/TenGigabitEthernet0~10~10/mtu`, matching every other `pointer` in this codebase — never build one by
 * hand, use `@ngfw/schema`'s `jsonPointer()`). Splitting it on `/` is safe here: RFC 6901 never leaves a literal,
 * un-escaped `/` inside a segment (a name containing one is `~1`-escaped), so every `/` remaining in the string is
 * a genuine segment boundary. Each segment is then percent-encoded on its own — so a `~0`/`~1` escape, being just
 * `~`, `0`/`1`, survives untouched (exactly what `pointerFromUrl` expects back), while a literal `%`, non-ASCII
 * text or other reserved character in a segment's own name stays transport-safe.
 */
export function configPointerPath(pointer: string): string {
  const trimmed = pointer.startsWith('/') ? pointer.slice(1) : pointer;
  if (trimmed === '') return '';
  return trimmed
    .split('/')
    .map((segment) => encodeURIComponent(segment))
    .join('/');
}

/**
 * Full request path for one of the generic config routes: `configUrl('/api/v1/config', pointer)` (GET running,
 * PATCH/PUT/DELETE candidate) or `configUrl('/api/v1/config/candidate', pointer)` (GET candidate). The empty/root
 * pointer (`''` or `'/'`) resolves to the prefix alone (the whole-document routes, no trailing segment).
 */
export function configUrl(prefix: '/api/v1/config' | '/api/v1/config/candidate', pointer: string): string {
  const suffix = configPointerPath(pointer);
  return suffix === '' ? prefix : `${prefix}/${suffix}`;
}
