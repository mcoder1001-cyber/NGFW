import { afterEach, describe, expect, it, vi } from 'vitest';
import { fetchWithTimeout } from '../net';
import { Session, type AuthSignal, type ChannelLike } from './session';

const body = (id = 1, username = 'admin') => ({ accessToken: `t.${id}.x`, tokenType: 'Bearer', expiresIn: 900, user: { id, username, role: 'admin' } });
const res = (status: number, b?: unknown, raw?: string) =>
  new Response(raw ?? (b === undefined ? null : JSON.stringify(b)), { status, headers: { 'content-type': 'application/json' } });

/** In-memory stand-in for BroadcastChannel between "tabs" (sessions) of one test. */
function bus() {
  const members: ChannelLike[] = [];
  return () => {
    const ch: ChannelLike = {
      onmessage: null,
      postMessage(msg: AuthSignal) {
        for (const m of members) if (m !== ch) m.onmessage?.({ data: msg });
      },
      close() {
        members.splice(members.indexOf(ch), 1);
      },
    };
    members.push(ch);
    return ch;
  };
}

afterEach(() => vi.useRealTimers());

describe('session resilience (review M2/M3/L7): only an explicit refusal ends a session', () => {
  it.each([500, 502, 503, 504, 429])('a %i on refresh keeps the signed-in session (retried later)', async (status) => {
    let answer = res(200, body());
    const s = new Session(async () => answer, undefined, { channel: null });
    await s.restore();
    expect(s.state.status).toBe('authenticated');
    answer = res(status, { detail: 'down' });
    expect(await s.refresh()).toBe(false);
    expect(s.state.status).toBe('authenticated');
    expect(s.accessToken).toBe('t.1.x');
    s.dispose();
  });

  it('a 401 on refresh ends it ("expired")', async () => {
    let answer = res(200, body());
    const s = new Session(async () => answer, undefined, { channel: null });
    await s.restore();
    answer = res(401, { detail: 'refresh token revoked' });
    expect(await s.refresh()).toBe(false);
    expect(s.state).toMatchObject({ status: 'anonymous', endReason: 'expired' });
  });

  it('a reload while the device does not answer → offline (not the login page), retried until it answers', async () => {
    let up = false;
    const s = new Session(async () => (up ? res(200, body()) : Promise.reject(new TypeError('Failed to fetch'))), undefined, { channel: null, retryMs: 20 });
    await s.restore();
    expect(s.state.status).toBe('offline');
    up = true;
    await vi.waitFor(() => expect(s.state.status).toBe('authenticated'));
    s.dispose();
  });

  it('a 503 or a non-JSON 200 (captive portal) on reload is offline too', async () => {
    for (const r of [res(503, { detail: 'valkey down' }), res(200, undefined, '<html>portal</html>')]) {
      const s = new Session(async () => r.clone(), undefined, { channel: null, retryMs: 60_000 });
      await s.restore();
      expect(s.state.status).toBe('offline');
      s.dispose();
    }
  });

  it('a refresh that never gets an answer gives up at its deadline (the cross-tab lock cannot hang)', async () => {
    vi.useFakeTimers();
    const s = new Session(() => new Promise<Response>(() => {}), undefined, { channel: null, retryMs: 60_000 });
    const p = s.restore();
    await vi.advanceTimersByTimeAsync(10_001);
    await p;
    expect(s.state.status).toBe('offline');
    s.dispose();
  });

  it('fetchWithTimeout rejects a silent request at its deadline', async () => {
    vi.useFakeTimers();
    const p = fetchWithTimeout(() => new Promise<Response>(() => {}), new Request('http://localhost/api/v1/config/diff'), 6000);
    const assertion = expect(p).rejects.toThrow('no answer within 6000 ms');
    await vi.advanceTimersByTimeAsync(6001);
    await assertion;
  });
});

describe('cross-tab coherence (review M4): the channel carries no token', () => {
  it('sign-out in one tab signs out the others', async () => {
    const channel = bus();
    const f = async (r: Request) => (new URL(r.url).pathname.endsWith('/logout') ? res(204) : res(200, body()));
    const a = new Session(f, undefined, { channel: channel() });
    const b = new Session(f, undefined, { channel: channel() });
    await a.login('admin', 'pw');
    await b.restore();
    expect(b.state.status).toBe('authenticated');
    await a.logout();
    expect(b.state).toMatchObject({ status: 'anonymous', endReason: 'signedOut' });
    expect(b.accessToken).toBeNull();
  });

  it('a sign-in as another user makes the other tabs reload; a signed-out tab picks the new session up', async () => {
    const channel = bus();
    let who = body(1, 'alice');
    const f = async () => res(200, who);
    const reloads: string[] = [];
    const a = new Session(f, undefined, { channel: channel(), onForeignLogin: () => reloads.push('a') });
    const b = new Session(f, undefined, { channel: channel(), onForeignLogin: () => reloads.push('b') });
    await b.restore(); // b: alice
    who = body(2, 'bob');
    await a.login('bob', 'pw');
    expect(reloads).toEqual(['b']);
    const sent: AuthSignal[] = [];
    const spy = channel();
    spy.onmessage = (ev) => sent.push(ev.data as AuthSignal);
    await a.login('bob', 'pw');
    expect(JSON.stringify(sent)).not.toContain('t.2.x'); // never a token on the channel
    a.dispose();
    b.dispose();
  });

  it('a scheduled refresh that returns a different user reloads instead of silently switching identity', async () => {
    let who = body(1, 'alice');
    const reloads: number[] = [];
    const s = new Session(async () => res(200, who), undefined, { channel: null, onForeignLogin: () => reloads.push(1) });
    await s.restore();
    who = body(2, 'bob');
    await s.refresh();
    expect(reloads).toEqual([1]);
    expect(s.state.user?.username).toBe('alice');
    s.dispose();
  });
});
