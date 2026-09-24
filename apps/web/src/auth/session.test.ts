import { describe, expect, it } from 'vitest';
import { Session, streamProtocols } from './session';

const body = (token = 'a.b.c', role = 'admin') => ({ accessToken: token, tokenType: 'Bearer', expiresIn: 900, user: { id: 1, username: 'admin', role } });
const res = (status: number, b?: unknown) =>
  new Response(b === undefined ? null : JSON.stringify(b), { status, headers: { 'content-type': 'application/json' } });

function fake(routes: Record<string, () => Response | Promise<Response>>) {
  const calls: string[] = [];
  const f = async (r: Request) => {
    const path = new URL(r.url, 'http://localhost').pathname;
    calls.push(`${r.method} ${path}`);
    const h = routes[`${r.method} ${path}`];
    if (!h) throw new TypeError('Failed to fetch');
    return h();
  };
  return { f, calls };
}

describe('Session (P06 auth: access token in memory, rotating refresh cookie)', () => {
  it('logs in, keeps the token in memory and offers it as WS subprotocols', async () => {
    const { f } = fake({ 'POST /api/v1/auth/login': () => res(200, body('x.y.z')) });
    const s = new Session(f);
    expect(streamProtocols(s)).toBeUndefined();
    expect(await s.login('admin', 'pw')).toBeNull();
    expect(s.state).toMatchObject({ status: 'authenticated', user: { username: 'admin', role: 'admin' } });
    expect(s.accessToken).toBe('x.y.z');
    expect(streamProtocols(s)).toEqual(['vrx.v1', 'bearer.x.y.z']);
    expect(localStorage.length).toBe(0); // never persisted
    s.dispose();
  });

  it('reports login failures by status, including an unreachable server', async () => {
    const s = new Session(
      fake({
        'POST /api/v1/auth/login': () => res(429, { detail: 'retry in 60 s' }),
      }).f,
    );
    expect(await s.login('admin', 'pw')).toEqual({ status: 429, detail: 'retry in 60 s' });
    expect(s.state.status).toBe('unknown');
    const down = new Session(fake({}).f);
    expect(await down.login('admin', 'pw')).toEqual({ status: 0, detail: undefined });
  });

  it('refreshes once for concurrent callers (single flight) — a replayed refresh token would revoke the family', async () => {
    let n = 0;
    const { f, calls } = fake({ 'POST /api/v1/auth/refresh': () => res(200, body(`t.${++n}.x`)) });
    const s = new Session(f);
    const [a, b, c] = await Promise.all([s.refresh(), s.refresh(), s.refresh()]);
    expect([a, b, c]).toEqual([true, true, true]);
    expect(calls.filter((c2) => c2.endsWith('/refresh'))).toHaveLength(1);
    expect(s.accessToken).toBe('t.1.x');
    s.dispose();
  });

  it('restore without a valid cookie → anonymous; an expired session → anonymous with reason "expired"', async () => {
    const s = new Session(fake({ 'POST /api/v1/auth/refresh': () => res(401, { detail: 'no refresh token' }) }).f);
    await s.restore();
    expect(s.state).toEqual({ status: 'anonymous', user: null, endReason: undefined });

    let ok = true;
    const s2 = new Session(fake({ 'POST /api/v1/auth/login': () => res(200, body()), 'POST /api/v1/auth/refresh': () => (ok ? res(200, body('n.e.w')) : res(401)) }).f);
    await s2.login('admin', 'pw');
    ok = false;
    expect(await s2.handleUnauthorized('a.b.c')).toBe(false);
    expect(s2.state).toEqual({ status: 'anonymous', user: null, endReason: 'expired' });
    expect(s2.accessToken).toBeNull();
  });

  it('keeps the session when the server is unreachable during a refresh (reconnecting, not signed out)', async () => {
    const routes: Record<string, () => Response> = { 'POST /api/v1/auth/login': () => res(200, body()) };
    const s = new Session(fake(routes).f);
    await s.login('admin', 'pw');
    expect(await s.refresh()).toBe(false); // no route → network error
    expect(s.state.status).toBe('authenticated');
    expect(s.accessToken).toBe('a.b.c');
    s.dispose();
  });

  it('does not refresh again when another request already rotated the token', async () => {
    const { f, calls } = fake({ 'POST /api/v1/auth/login': () => res(200, body('new.t.k')) });
    const s = new Session(f);
    await s.login('admin', 'pw');
    expect(await s.handleUnauthorized('old.t.k')).toBe(true);
    expect(calls.filter((c) => c.endsWith('/refresh'))).toHaveLength(0);
    s.dispose();
  });

  it('logout forgets the token even when the server is down', async () => {
    const { f } = fake({ 'POST /api/v1/auth/login': () => res(200, body()) });
    const s = new Session(f);
    await s.login('admin', 'pw');
    await s.logout();
    expect(s.state).toEqual({ status: 'anonymous', user: null, endReason: 'signedOut' });
    expect(s.accessToken).toBeNull();
  });
});
