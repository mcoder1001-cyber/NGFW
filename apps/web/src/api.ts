import { createApiClient } from '@ngfw/api-client';
import { AUTH_PREFIX, session as appSession, type Session } from './auth/session';

type Fetch = (input: Request) => Promise<Response>;

/** Auth routes that authenticate with the httpOnly refresh cookie (or credentials) instead of the bearer token. */
const COOKIE_ROUTES = new Set([`${AUTH_PREFIX}login`, `${AUTH_PREFIX}refresh`, `${AUTH_PREFIX}logout`]);

/**
 * openapi-fetch middleware: attaches `Authorization: Bearer <access token>` and, on a 401, refreshes the session once
 * and retries the request. The middleware performs the request itself (`onRequest` returning a Response), so the retry
 * can reuse a clone of the original body and tests can stub `globalThis.fetch` at any time.
 */
export function authMiddleware(session: Session, fetchImpl: Fetch = (r) => globalThis.fetch(r)) {
  return {
    async onRequest({ request }: { request: Request }): Promise<Response> {
      const path = new URL(request.url, 'http://localhost').pathname;
      const token = session.accessToken;
      if (COOKIE_ROUTES.has(path) || token === null) return fetchImpl(request);
      const retry = request.clone();
      const withToken = (r: Request, t: string) => {
        const headers = new Headers(r.headers);
        headers.set('authorization', `Bearer ${t}`);
        return new Request(r, { headers });
      };
      const res = await fetchImpl(withToken(request, token));
      if (res.status !== 401) return res;
      if (!(await session.handleUnauthorized(token)) || session.accessToken === null) return res;
      return fetchImpl(withToken(retry, session.accessToken));
    },
  };
}

export function createAppApi(session: Session = appSession) {
  const client = createApiClient('');
  client.use(authMiddleware(session));
  return client;
}

/** The one typed API client of the SPA (generated types from packages/api-client, never hand-written paths). */
export const api = createAppApi();
export type Api = typeof api;
