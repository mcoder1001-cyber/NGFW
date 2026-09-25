import type { FastifyReply } from 'fastify';

/** The rotating refresh token (P06 §6): httpOnly, SameSite=Strict, only ever sent to the auth routes. */
export const REFRESH_COOKIE = 'vrx_refresh';
export const REFRESH_COOKIE_PATH = '/api/v1/auth';

/**
 * TD-10b (review 2.3g, option (a)): the browser's credential for the API description at `/api/docs` — the access token
 * of the session, set next to the refresh cookie by login and refresh, cleared by logout. httpOnly, SameSite=Strict,
 * path `/api/docs`: the browser sends it only to the Swagger UI (its page, assets and the spec inlined in
 * `swagger-ui-init.js`), and only the docs hook accepts it (GET/HEAD) — every other route reads the Authorization
 * header only. It dies with the session: per-sid logout, revocation, the access TTL and VRX_SESSION_MAX_SEC.
 * The refresh cookie itself cannot serve: it is scoped to /api/v1/auth, and presenting it rotates the chain (a second
 * tab refreshing in parallel would trip reuse detection and end the web UI's session).
 */
export const DOCS_COOKIE = 'vrx_docs';
export const DOCS_COOKIE_PATH = '/api/docs';

export interface CookieOpts {
  secure: boolean;
}

/** Set both session cookies after a login or refresh. `refreshMaxAge`/`accessMaxAge` are seconds. */
export function setSessionCookies(
  reply: FastifyReply,
  s: { refreshToken: string; accessToken: string; refreshMaxAge: number; accessMaxAge: number },
  o: CookieOpts,
): void {
  void reply.setCookie(REFRESH_COOKIE, s.refreshToken, {
    httpOnly: true,
    sameSite: 'strict',
    secure: o.secure,
    path: REFRESH_COOKIE_PATH,
    maxAge: s.refreshMaxAge,
  });
  void reply.setCookie(DOCS_COOKIE, s.accessToken, {
    httpOnly: true,
    sameSite: 'strict',
    secure: o.secure,
    path: DOCS_COOKIE_PATH,
    maxAge: s.accessMaxAge,
  });
}

/** Logout: both cookies go. */
export function clearSessionCookies(reply: FastifyReply): void {
  void reply.clearCookie(REFRESH_COOKIE, { path: REFRESH_COOKIE_PATH });
  void reply.clearCookie(DOCS_COOKIE, { path: DOCS_COOKIE_PATH });
}
