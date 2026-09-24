/**
 * UNIT-TEST ONLY: a scripted stand-in for the P06 HTTP API behind `globalThis.fetch`, so screen logic can be tested in
 * jsdom. The real API is exercised by `test/e2e/flow.e2e.mjs` (UI honesty rule: no mocks outside unit tests).
 */
import { vi } from 'vitest';
import { session } from './auth/session';
import { confirmStore } from './config/confirm-store';

export type Handler = (req: Request, body: unknown) => { status?: number; body?: unknown; headers?: Record<string, string> } | 'network-error';

export interface FakeApi {
  calls: { method: string; path: string; search: string; body: unknown; auth: string | null }[];
  on(route: string, h: Handler | { status?: number; body?: unknown }): void;
}

const json = (status: number, body: unknown, headers: Record<string, string> = {}) =>
  new Response(body === undefined ? null : JSON.stringify(body), {
    status,
    headers: { 'content-type': status >= 400 ? 'application/problem+json' : 'application/json', date: new Date().toUTCString(), ...headers },
  });

/** Install the fake; unknown routes answer 404 problem+json. Route keys are `METHOD /path` (no query string). */
export function installFakeApi(role: 'admin' | 'operator' | 'readonly' = 'admin', username = 'admin'): FakeApi {
  const handlers = new Map<string, Handler>();
  const api: FakeApi = {
    calls: [],
    on(route, h) {
      handlers.set(route, typeof h === 'function' ? h : () => h);
    },
  };
  const sessionBody = { accessToken: 'h.p.s', tokenType: 'Bearer', expiresIn: 900, user: { id: 1, username, role } };
  api.on('POST /api/v1/auth/login', { body: sessionBody });
  api.on('POST /api/v1/auth/refresh', { body: sessionBody });
  api.on('POST /api/v1/auth/logout', { status: 204 });
  api.on('GET /api/v1/config/diff', { body: { baseRevision: 1, changes: [] } });
  api.on('GET /api/v1/config/lock', { body: { locked: false, owner: null, ownerId: null, lockedAt: null, lastActivity: null, expiresAt: null } });
  api.on('GET /api/v1/config/commit/pending', { body: { pending: null } });
  api.on('GET /api/v1/state/system', {
    body: { api: { version: 't', startedAt: '', wsClients: 0 }, agent: { reachable: true }, runningRevision: 1, pendingCommit: null, sync: { state: 'in-sync', reason: '', txnId: null, since: '' } },
  });
  api.on('GET /api/v1/config/revisions', { body: { items: [], total: 0 } });
  vi.stubGlobal('fetch', async (input: Request) => {
    const url = new URL(input.url, 'http://localhost');
    const text = input.method === 'GET' || input.method === 'HEAD' ? '' : await input.clone().text();
    const body = text ? (JSON.parse(text) as unknown) : undefined;
    api.calls.push({ method: input.method, path: url.pathname, search: url.search, body, auth: input.headers.get('authorization') });
    const h = handlers.get(`${input.method} ${url.pathname}`);
    if (!h) return json(404, { type: 'https://vrx.dev/problems/not-found', title: 'Not found', status: 404 });
    const r = h(input, body);
    if (r === 'network-error') throw new TypeError('Failed to fetch');
    return json(r.status ?? 200, r.body, r.headers);
  });
  return api;
}

/** Sign the app-wide session in through the fake API. */
export async function signIn(): Promise<void> {
  await session.login('admin', 'x');
}

export async function resetSession(): Promise<void> {
  await session.logout();
  confirmStore.reset();
  vi.unstubAllGlobals();
}
