import { describe, expect, it } from 'vitest';
import { Nat46ClientQuery } from './dto.js';
import { nat46ClientAddress, nat46Json } from './service.js';

describe('nat46Json', () => {
  it('lists the running mappings with their domain names, tolerating an absent or odd subtree', () => {
    expect(
      nat46Json({
        nat46: {
          clientPrefix: 'fd00:1:46::/96',
          interfaces: ['host-w1l0', 3],
          mappings: [{ name: 'web', ipv4: '10.1.2.80', ipv6: 'fd00:1:2::80', mtu: 1500 }, 'junk'],
        },
      }),
    ).toEqual({
      configured: true,
      clientPrefix: 'fd00:1:46::/96',
      interfaces: ['host-w1l0'],
      mappings: [
        { name: 'web', domain: 'nat46-web', ipv4: '10.1.2.80', ipv6: 'fd00:1:2::80', mtu: 1500 },
      ],
    });
    expect(nat46Json(undefined)).toEqual({
      configured: false,
      clientPrefix: '64:ff9b::/96',
      interfaces: [],
      mappings: [],
    });
  });
});

describe('nat46ClientAddress', () => {
  it('embeds the IPv4 address in the last 32 bits (RFC 6052 /96), canonical spelling', () => {
    expect(nat46ClientAddress('64:ff9b::/96', '192.0.2.33')).toBe('64:ff9b::c000:221');
    expect(nat46ClientAddress('fd00:1:46::/96', '10.1.1.2')).toBe('fd00:1:46::a01:102');
    expect(nat46ClientAddress('fd00:1:46::/64', '10.1.1.2')).toBeUndefined();
  });
  it('the query needs an IPv4 address', () => {
    expect(Nat46ClientQuery.safeParse({ ipv4: '10.1.1.2' }).success).toBe(true);
    expect(Nat46ClientQuery.safeParse({ ipv4: 'fd00::1' }).success).toBe(false);
  });
});
