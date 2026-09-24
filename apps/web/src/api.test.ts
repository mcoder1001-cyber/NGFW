import { describe, expect, it } from 'vitest';
import { authMiddleware } from './api';
import { Session } from './auth/session';

const sessionBody = (t: string) => ({ accessToken: t, tokenType: 'Bearer', expiresIn: 900, user: { id: 1, username: 'admin', role: 'admin' } });

describe('authMiddleware (bearer token + one refresh-and-retry on 401)', () => {
  it('attaches the token, refreshes once on 401 and retries with the new token and the same body', async () => {
    let refreshed = 0;
    const s = new Session(async (r) => {
      const p = new URL(r.url).pathname;
      if (p.endsWith('/login')) return new Response(JSON.stringify(sessionBody('old.t.k')), { status: 200 });
      if (p.endsWith('/refresh')) {
        refreshed++;
        return new Response(JSON.stringify(sessionBody('new.t.k')), { status: 200 });
      }
      throw new Error('unexpected');
    });
    await s.login('admin', 'pw');
    const seen: { auth: string | null; body: string }[] = [];
    const mw = authMiddleware(s, async (r) => {
      seen.push({ auth: r.headers.get('authorization'), body: await r.text() });
      return new Response(null, { status: seen.length === 1 ? 401 : 200 });
    });
    const req = new Request('http://localhost/api/v1/config/management', { method: 'PATCH', body: JSON.stringify({ users: [] }) });
    const res = await mw.onRequest({ request: req });
    expect(res.status).toBe(200);
    expect(refreshed).toBe(1);
    expect(seen).toEqual([
      { auth: 'Bearer old.t.k', body: '{"users":[]}' },
      { auth: 'Bearer new.t.k', body: '{"users":[]}' },
    ]);
    s.dispose();
  });

  it('sends cookie-authenticated auth routes without a bearer token, and nothing when signed out', async () => {
    const s = new Session(async () => new Response(null, { status: 401 }));
    const seen: (string | null)[] = [];
    const mw = authMiddleware(s, async (r) => {
      seen.push(r.headers.get('authorization'));
      return new Response(null, { status: 401 });
    });
    const res = await mw.onRequest({ request: new Request('http://localhost/api/v1/config/diff') });
    expect(res.status).toBe(401); // no token → no refresh attempt, the caller sees the 401
    await mw.onRequest({ request: new Request('http://localhost/api/v1/auth/refresh', { method: 'POST' }) });
    expect(seen).toEqual([null, null]);
  });
});
