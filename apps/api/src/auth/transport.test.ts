import type { FastifyRequest } from 'fastify';
import { describe, expect, it } from 'vitest';
import { isLoopback, secureTransport, tlsRequired } from './transport.js';

const req = (protocol: 'http' | 'https', ip: string, headers: Record<string, string> = {}) =>
  ({ protocol, ip, headers }) as unknown as FastifyRequest;

describe('transport (TD-2 #1, D-100 (1))', () => {
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

  it('secure = a TLS socket of this process or a loopback peer; X-Forwarded-* are not trusted', () => {
    expect(secureTransport(req('https', '192.0.2.10'))).toBe(true);
    expect(secureTransport(req('http', '127.0.0.1'))).toBe(true);
    expect(secureTransport(req('http', '::1'))).toBe(true);
    expect(secureTransport(req('http', '192.0.2.10'))).toBe(false);
    expect(
      secureTransport(
        req('http', '192.0.2.10', { 'x-forwarded-proto': 'https', 'x-forwarded-for': '127.0.0.1' }),
      ),
    ).toBe(false);
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
