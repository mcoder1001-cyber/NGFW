import Fastify, { type FastifyInstance } from 'fastify';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { clientKey, requestProtocol, sourceIp } from '../common/principal.js';
import { loadEnv } from '../config.js';
import { isLoopback, secureTransport, tlsRequired } from './transport.js';

/**
 * TD-10b (review 2.3b): the client address, its protocol and the password transport rule, resolved by a REAL Fastify
 * with the trustProxy that app.ts derives from VRX_TRUST_PROXY — not a hand-made request object.
 */
async function server(trust: string): Promise<FastifyInstance> {
  const list = loadEnv({ VRX_TRUST_PROXY: trust }).VRX_TRUST_PROXY;
  const f = Fastify({ trustProxy: list.length > 0 ? list : false });
  f.get('/probe', (req) => ({
    ip: sourceIp(req),
    proto: requestProtocol(req),
    secure: secureTransport(req),
  }));
  await f.ready();
  return f;
}

type Probe = { ip: string; proto: string; secure: boolean };

describe('transport (TD-2 #1, D-100 (1); TD-10b trusted proxy)', () => {
  let loop: FastifyInstance;
  let none: FastifyInstance;
  const probe = async (
    f: FastifyInstance,
    remoteAddress: string,
    headers: Record<string, string> = {},
  ): Promise<Probe> =>
    (await f.inject({ method: 'GET', url: '/probe', remoteAddress, headers })).json<Probe>();

  beforeAll(async () => {
    loop = await server('loopback');
    none = await server('none');
  });
  afterAll(async () => {
    await loop.close();
    await none.close();
  });

  it('loopback: IPv4 127/8, ::1 and IPv4-mapped 127/8 — nothing else', () => {
    for (const ip of ['127.0.0.1', '127.1.2.3', '::1', '::ffff:127.0.0.1', '::FFFF:127.0.0.9'])
      expect(isLoopback(ip), ip).toBe(true);
    for (const ip of [
      undefined,
      '',
      '192.0.2.10',
      '10.0.0.1',
      '::ffff:192.0.2.10',
      '::2',
      '128.0.0.1',
      '1127.0.0.1',
    ])
      expect(isLoopback(ip), String(ip)).toBe(false);
  });

  it('a peer that forwards nothing is the client: loopback is secure, a remote plain-HTTP peer is not', async () => {
    expect(await probe(loop, '127.0.0.1')).toEqual({
      ip: '127.0.0.1',
      proto: 'http',
      secure: true,
    });
    expect(await probe(loop, '::1')).toMatchObject({ secure: true });
    expect(await probe(loop, '192.0.2.10')).toEqual({
      ip: '192.0.2.10',
      proto: 'http',
      secure: false,
    });
  });

  it('a remote peer is not a trusted proxy: its X-Forwarded-* are ignored', async () => {
    expect(
      await probe(loop, '192.0.2.10', {
        'x-forwarded-proto': 'https',
        'x-forwarded-for': '127.0.0.1',
      }),
    ).toEqual({ ip: '192.0.2.10', proto: 'http', secure: false });
  });

  it('behind the loopback proxy the CLIENT counts: plain HTTP from another machine is refused, TLS accepted', async () => {
    // vite (xfwd) relaying a remote browser on plain HTTP — before TD-10b this passed as "loopback"
    expect(
      await probe(loop, '127.0.0.1', {
        'x-forwarded-for': '198.51.100.7',
        'x-forwarded-proto': 'http',
      }),
    ).toEqual({ ip: '198.51.100.7', proto: 'http', secure: false });
    // the product nginx terminating TLS
    expect(
      await probe(loop, '127.0.0.1', {
        'x-forwarded-for': '198.51.100.7',
        'x-forwarded-proto': 'https',
      }),
    ).toEqual({ ip: '198.51.100.7', proto: 'https', secure: true });
    // a browser on this host through the proxy
    expect(
      await probe(loop, '127.0.0.1', {
        'x-forwarded-for': '127.0.0.1',
        'x-forwarded-proto': 'http',
      }),
    ).toEqual({ ip: '127.0.0.1', proto: 'http', secure: true });
    // a proxied WebSocket upgrade over TLS
    expect(
      await probe(loop, '127.0.0.1', {
        'x-forwarded-for': '198.51.100.7',
        'x-forwarded-proto': 'wss',
      }),
    ).toMatchObject({ proto: 'https', secure: true });
  });

  it('a client cannot forge its way in through an appending proxy (vite xfwd appends to what the client sent)', async () => {
    expect(
      await probe(loop, '127.0.0.1', {
        'x-forwarded-for': '127.0.0.1, 198.51.100.7',
        'x-forwarded-proto': 'https,http',
      }),
    ).toEqual({ ip: '198.51.100.7', proto: 'http', secure: false });
  });

  it('two trusted hops (nginx → vite): the outermost trusted hop tells the client protocol', async () => {
    expect(
      await probe(loop, '127.0.0.1', {
        'x-forwarded-for': '198.51.100.7, 127.0.0.1',
        'x-forwarded-proto': 'https,http',
      }),
    ).toEqual({ ip: '198.51.100.7', proto: 'https', secure: true });
  });

  it('a trusted hop that does not say how the client connected → not TLS', async () => {
    expect(await probe(loop, '127.0.0.1', { 'x-forwarded-for': '198.51.100.7' })).toEqual({
      ip: '198.51.100.7',
      proto: 'http',
      secure: false,
    });
  });

  it('VRX_TRUST_PROXY=none: headers are ignored, the socket peer is the client', async () => {
    expect(
      await probe(none, '127.0.0.1', {
        'x-forwarded-for': '198.51.100.7',
        'x-forwarded-proto': 'https',
      }),
    ).toEqual({ ip: '127.0.0.1', proto: 'http', secure: true });
  });

  it('VRX_TRUST_PROXY: loopback by default; addresses, ranges and names; never "everyone"', () => {
    expect(loadEnv({}).VRX_TRUST_PROXY).toEqual(['loopback']);
    expect(loadEnv({ VRX_TRUST_PROXY: 'none' }).VRX_TRUST_PROXY).toEqual([]);
    expect(
      loadEnv({ VRX_TRUST_PROXY: ' loopback , 10.1.2.3, 10.0.0.0/8 ,fd00::/8,uniquelocal' })
        .VRX_TRUST_PROXY,
    ).toEqual(['loopback', '10.1.2.3', '10.0.0.0/8', 'fd00::/8', 'uniquelocal']);
    for (const bad of [
      'true',
      '*',
      '1',
      '0.0.0.0/0',
      '::/0',
      '10.0.0.0/33',
      'none,loopback',
      'lan',
    ])
      expect(() => loadEnv({ VRX_TRUST_PROXY: bad }), bad).toThrow(/VRX_TRUST_PROXY/);
  });

  it('clientKey: IPv4 as is, IPv4-mapped as IPv4, IPv6 by its /64', () => {
    expect(clientKey('198.51.100.7')).toBe('198.51.100.7');
    expect(clientKey('::ffff:198.51.100.7')).toBe('198.51.100.7');
    expect(clientKey('2001:db8:1:2:3:4:5:6')).toBe('2001:db8:1:2::/64');
    expect(clientKey('2001:db8:1:2::9')).toBe('2001:db8:1:2::/64');
    expect(clientKey('2001:db8::1')).toBe('2001:db8:0:0::/64');
    expect(clientKey('2001:0DB8:0001:0002:ffff::1')).toBe('2001:db8:1:2::/64');
    expect(clientKey('fe80::1%eth0')).toBe('fe80:0:0:0::/64');
    expect(clientKey('::1')).toBe('0:0:0:0::/64');
  });

  it('tls-required is a 403 problem with a stable type', () => {
    const p = tlsRequired();
    expect(p.getStatus()).toBe(403);
    expect(p.body('/api/v1/auth/login')).toEqual({
      type: 'https://vrx.dev/problems/tls-required',
      title: 'TLS required',
      status: 403,
      detail: 'passwords are accepted over TLS only (connect through https)',
      instance: '/api/v1/auth/login',
    });
  });
});
