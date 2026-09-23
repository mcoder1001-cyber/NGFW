import { EventKind } from '@ngfw/proto';
import { randomBytes } from 'node:crypto';
import { request, type IncomingMessage } from 'node:http';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { startHarness, type Harness } from '../support/harness.js';

type Msg = { type: string; topic?: string; data?: any; [k: string]: unknown };

/** A WebSocket client (Node 22 global WebSocket) collecting JSON messages. */
function connect(url: string, protocols: string[]) {
  const ws = new WebSocket(url, protocols);
  const messages: Msg[] = [];
  ws.addEventListener('message', (e) => messages.push(JSON.parse(String(e.data)) as Msg));
  const opened = new Promise<void>((resolve, reject) => {
    ws.addEventListener('open', () => resolve());
    ws.addEventListener('error', () => reject(new Error('websocket error')));
  });
  const until = async (pred: (m: Msg) => boolean, timeout = 5000): Promise<Msg> => {
    const t0 = Date.now();
    for (;;) {
      const m = messages.find(pred);
      if (m) return m;
      if (Date.now() - t0 > timeout)
        throw new Error(`timeout; got ${JSON.stringify(messages.slice(-5))}`);
      await new Promise((r) => setTimeout(r, 50));
    }
  };
  return { ws, messages, opened, until, send: (m: unknown) => ws.send(JSON.stringify(m)) };
}

describe('telemetry relay e2e (WS /api/v1/stream)', () => {
  let h: Harness;
  let token: string;
  let base: string;

  beforeAll(async () => {
    h = await startHarness({ VRX_ACCESS_TTL_SEC: '8' });
    token = await h.login('admin', h.adminPassword);
    await h.app.listen({ port: h.env.VRX_HTTP_PORT, host: '127.0.0.1' });
    base = `ws://127.0.0.1:${h.env.VRX_HTTP_PORT}/api/v1/stream`;
    await h.call(token, 'PUT', '/api/v1/config/interfaces/loop110', { ipv4: ['10.1.110.1/24'] });
    await h.call(token, 'POST', '/api/v1/config/commit');
  });
  afterAll(async () => h?.close());

  it('refuses an upgrade without credentials (401)', async () => {
    // a raw upgrade request: the server must answer 401 problem+json instead of switching protocols
    const res = await new Promise<IncomingMessage>((resolve, reject) => {
      const req = request(base.replace('ws:', 'http:'), {
        headers: {
          connection: 'Upgrade',
          upgrade: 'websocket',
          'sec-websocket-version': '13',
          'sec-websocket-key': randomBytes(16).toString('base64'),
        },
      });
      req.on('response', resolve);
      req.on('upgrade', () => reject(new Error('upgraded without credentials')));
      req.on('error', reject);
      req.end();
    });
    res.resume();
    expect(res.statusCode).toBe(401);
    expect(res.headers['content-type']).toMatch(/problem\+json/);
    const bad = connect(base, ['vrx.v1', 'bearer.a.b.c']);
    await expect(bad.opened).rejects.toThrow();
  });

  it('per-connection topics, counters, events, heartbeats', async () => {
    const a = connect(base, ['vrx.v1', `bearer.${token}`]);
    const b = connect(base, ['vrx.v1', `bearer.${token}`]);
    await Promise.all([a.opened, b.opened]);
    expect(a.ws.protocol).toBe('vrx.v1');
    await a.until((m) => m.type === 'welcome');
    a.send({ subscribe: ['iface.counters', 'reconcile.events', 'commit.events'] });
    b.send({ subscribe: ['link.events'] });
    expect((await a.until((m) => m.type === 'subscribed')).topics).toEqual([
      'iface.counters',
      'reconcile.events',
      'commit.events',
    ]);
    await b.until((m) => m.type === 'subscribed');

    const counters = await a.until((m) => m.topic === 'iface.counters');
    expect(counters.data.interfaces).toContainEqual(
      expect.objectContaining({ name: 'loop110', rxPackets: expect.any(String) }),
    );

    h.fake.emit(EventKind.EVENT_KIND_LINK_DOWN, { interface: 'loop110', message: 'carrier lost' });
    const link = await b.until((m) => m.topic === 'link.events');
    expect(link.data).toMatchObject({
      kind: 'link_down',
      interface: 'loop110',
      message: 'carrier lost',
    });

    await h.call(token, 'PATCH', '/api/v1/config/system', { hostname: 'stream-w1' });
    await h.call(token, 'POST', '/api/v1/config/commit');
    await a.until((m) => m.topic === 'reconcile.events' && m.data.kind === 'reconcile_done');
    await a.until((m) => m.topic === 'commit.events' && m.data.type === 'applied');

    // filters: b never sees counters or commits, a never sees link events
    expect(
      b.messages.some((m) => m.topic === 'iface.counters' || m.topic === 'commit.events'),
    ).toBe(false);
    expect(a.messages.some((m) => m.topic === 'link.events')).toBe(false);

    await a.until((m) => m.type === 'heartbeat', 3000);
    a.send({ subscribe: ['bogus.topic'] });
    await a.until((m) => m.type === 'error');
    a.send({ unsubscribe: ['iface.counters'] });
    await a.until(
      (m) =>
        m.type === 'subscribed' &&
        Array.isArray(m.topics) &&
        !(m.topics as string[]).includes('iface.counters'),
    );
    a.ws.close();
    b.ws.close();
  });

  function closed(c: ReturnType<typeof connect>): Promise<number> {
    return new Promise((resolve) => c.ws.addEventListener('close', (e) => resolve(e.code)));
  }

  it("review L3: logout closes the session's WebSockets; the token's expiry closes the rest", async () => {
    const l = await h.call(undefined, 'POST', '/api/v1/auth/login', {
      username: 'admin',
      password: h.adminPassword,
    });
    const t = l.body.accessToken as string;
    const cookie = [l.headers['set-cookie']]
      .flat()
      .find((c) => String(c).startsWith('vrx_refresh=')) as string;
    const s1 = connect(base, ['vrx.v1', `bearer.${t}`]);
    const s2 = connect(base, ['vrx.v1', `bearer.${t}`]);
    await Promise.all([s1.opened, s2.opened]);
    const c1 = closed(s1);
    expect(
      (
        await h.call(undefined, 'POST', '/api/v1/auth/logout', undefined, {
          cookie: cookie.split(';')[0]!,
        })
      ).status,
    ).toBe(204);
    expect(await c1).toBe(4403);
    // a fresh login's socket closes by itself when its access token expires (VRX_ACCESS_TTL_SEC=8 here)
    const t2 = await h.login('admin', h.adminPassword);
    const s3 = connect(base, ['vrx.v1', `bearer.${t2}`]);
    await s3.opened;
    const t0 = Date.now();
    expect(await closed(s3)).toBe(4401);
    expect(Date.now() - t0).toBeLessThan(9500);
  }, 20_000);
});
