import { describe, expect, it } from 'vitest';
import {
  ipFamily,
  isNetworkAddress,
  networkAddress,
  parseCidr,
  parseIpv4,
  parseIpv6,
  prefixContains,
  prefixesOverlap,
} from './ip.js';

describe('parseIpv4', () => {
  it.each([
    ['0.0.0.0', 0n],
    ['10.0.0.1', 0x0a000001n],
    ['255.255.255.255', 0xffffffffn],
    ['192.168.100.200', 0xc0a864c8n],
  ])('%s → %s', (text, value) => expect(parseIpv4(text)).toBe(value));

  it.each(['', '1.2.3', '1.2.3.4.5', '256.0.0.1', '01.2.3.4', '1.2.3.-4', 'a.b.c.d', '1..2.3', ' 1.2.3.4'])(
    'rejects %j',
    (text) => expect(parseIpv4(text)).toBeUndefined(),
  );
});

describe('parseIpv6', () => {
  it.each([
    ['::', 0n],
    ['::1', 1n],
    ['1::', 1n << 112n],
    ['2001:db8::1', (0x2001n << 112n) | (0xdb8n << 96n) | 1n],
    ['2001:DB8:0:0:0:0:0:1', (0x2001n << 112n) | (0xdb8n << 96n) | 1n],
    ['::ffff:10.0.0.1', (0xffffn << 32n) | 0x0a000001n],
    ['64:ff9b::192.0.2.33', (0x64n << 112n) | (0xff9bn << 96n) | 0xc0000221n],
    ['1:2:3:4:5:6:7:8', 0x00010002000300040005000600070008n],
    ['1:2:3:4:5:6:1.2.3.4', 0x00010002000300040005000601020304n],
  ])('%s → %s', (text, value) => expect(parseIpv6(text)).toBe(value));

  it.each([
    '',
    ':',
    ':::',
    '1:::2',
    ':1',
    '1:',
    '1:2:3:4:5:6:7',
    '1:2:3:4:5:6:7:8:9',
    '1:2:3:4:5:6:7::',
    '::1:2:3:4:5:6:7:8',
    '12345::',
    'g::1',
    'fe80::1%eth0',
    '::1.2.3.4:ffff',
    '1.2.3.4::ffff',
    '::256.0.0.1',
    '1.2.3.4',
  ])('rejects %j', (text) => expect(parseIpv6(text)).toBeUndefined());
});

describe('ipFamily', () => {
  it.each([
    ['10.0.0.1', 4],
    ['::1', 6],
    ['::ffff:10.0.0.1', 6],
    ['10.0.0.1/24', undefined],
    ['host', undefined],
  ])('%s → %s', (text, family) => expect(ipFamily(text)).toBe(family));
});

describe('parseCidr / networkAddress / isNetworkAddress', () => {
  it('parses both families and keeps host bits', () => {
    expect(parseCidr('10.0.0.1/24')).toEqual({ family: 4, address: 0x0a000001n, length: 24 });
    expect(parseCidr('0.0.0.0/0')).toEqual({ family: 4, address: 0n, length: 0 });
    expect(parseCidr('2001:db8::1/64')).toEqual({
      family: 6,
      address: (0x2001n << 112n) | (0xdb8n << 96n) | 1n,
      length: 64,
    });
    expect(parseCidr('::/128')).toEqual({ family: 6, address: 0n, length: 128 });
  });

  it.each(['10.0.0.1', '10.0.0.1/33', '10.0.0.1/-1', '10.0.0.1/08', '10.0.0.1/1.5', '::1/129', 'x/24', '/24', '10.0.0.1/', '10.0.0.1/24/'])(
    'rejects %j',
    (text) => expect(parseCidr(text)).toBeUndefined(),
  );

  it('clears host bits', () => {
    expect(networkAddress({ family: 4, address: 0x0a0000ffn, length: 24 })).toBe(0x0a000000n);
    expect(networkAddress({ family: 4, address: 0x0a0000ffn, length: 32 })).toBe(0x0a0000ffn);
    expect(networkAddress({ family: 4, address: 0xffffffffn, length: 0 })).toBe(0n);
    expect(networkAddress({ family: 6, address: (1n << 127n) | 1n, length: 64 })).toBe(1n << 127n);
  });

  it.each([
    ['10.0.0.0/24', true],
    ['10.0.0.1/24', false],
    ['10.0.0.1/32', true],
    ['0.0.0.0/0', true],
    ['128.0.0.0/0', false],
    ['2001:db8::/64', true],
    ['2001:db8::1/64', false],
    ['::/0', true],
    ['not-a-prefix', false],
  ])('isNetworkAddress(%s) = %s', (text, expected) => expect(isNetworkAddress(text)).toBe(expected));
});

describe('prefixesOverlap / prefixContains', () => {
  const p = (text: string) => parseCidr(text)!;
  it.each([
    ['10.0.0.0/24', '10.0.0.128/25', true],
    ['10.0.0.128/25', '10.0.0.0/24', true],
    ['10.0.0.0/24', '10.0.1.0/24', false],
    ['10.0.0.1/24', '10.0.0.2/24', true],
    ['0.0.0.0/0', '192.168.1.1/32', true],
    ['10.0.0.0/24', '2001:db8::/32', false],
    ['2001:db8::/32', '2001:db8:1::/48', true],
    ['2001:db8::/48', '2001:db9::/48', false],
  ])('%s vs %s → %s', (a, b, expected) => expect(prefixesOverlap(p(a), p(b))).toBe(expected));

  it('prefixContains checks a bare address against a prefix', () => {
    expect(prefixContains(p('10.0.0.0/24'), 4, parseIpv4('10.0.0.254')!)).toBe(true);
    expect(prefixContains(p('10.0.0.0/24'), 4, parseIpv4('10.0.1.1')!)).toBe(false);
    expect(prefixContains(p('10.0.0.0/24'), 6, parseIpv6('::a00:1')!)).toBe(false);
    expect(prefixContains(p('2001:db8::/64'), 6, parseIpv6('2001:db8::dead:beef')!)).toBe(true);
  });
});
