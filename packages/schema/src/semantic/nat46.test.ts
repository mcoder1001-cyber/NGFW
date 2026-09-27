import { describe, expect, it } from 'vitest';
import { RootConfig } from '../index.js';
import { validateSemantics } from './index.js';

const interfaces = {
  'host-w4l0': { ipv4: ['10.4.1.1/24'] },
  'host-w4w0': { ipv6: ['2001:db8:1::1/64'] },
};
const nat46 = (over: Record<string, unknown> = {}) => ({
  clientPrefix: '64:ff9b::/96',
  interfaces: ['host-w4l0', 'host-w4w0'],
  mappings: [{ name: 'web', ipv4: '198.51.100.10', ipv6: '2001:db8:1::10' }],
  ...over,
});
const parse = (nat: unknown) => RootConfig.safeParse({ interfaces, nat });
const paths = (nat: unknown) => {
  const r = parse(nat);
  return r.success ? [] : r.error.issues.map((i) => i.path.join('/'));
};
const at = (nat: unknown) =>
  validateSemantics(RootConfig.parse({ interfaces, nat }), ['nat', 'interfaces'])
    .filter((i) => i.pointer.startsWith('/nat'))
    .map((i) => i.pointer);

describe('nat.nat46 refinements', () => {
  it('accepts a valid object and defaults the client prefix', () => {
    expect(paths({ nat46: nat46() })).toEqual([]);
    const r = parse({ nat46: { interfaces: [], mappings: [] } });
    expect(r.success && r.data.nat.nat46?.clientPrefix).toBe('64:ff9b::/96');
  });
  it('requires a /96 with host bits zero', () => {
    expect(paths({ nat46: nat46({ clientPrefix: '64:ff9b::/64' }) })).toEqual(['nat/nat46/clientPrefix']);
    expect(paths({ nat46: nat46({ clientPrefix: '64:ff9b::1/96' }) })).toEqual(['nat/nat46/clientPrefix']);
  });
  it('refuses a duplicate IPv4 service address and IPv6 server', () => {
    const m = [
      { name: 'a', ipv4: '198.51.100.10', ipv6: '2001:db8:1::10' },
      { name: 'b', ipv4: '198.51.100.10', ipv6: '2001:db8:1::10' },
    ];
    expect(paths({ nat46: nat46({ mappings: m }) })).toEqual(['nat/nat46/mappings/1/ipv4', 'nat/nat46/mappings/1/ipv6']);
  });
  it('refuses a server inside the client prefix, non-unicast and missing interfaces', () => {
    expect(paths({ nat46: nat46({ mappings: [{ name: 'a', ipv4: '198.51.100.1', ipv6: '64:ff9b::1' }] }) })).toEqual([
      'nat/nat46/mappings/0/ipv6',
    ]);
    expect(paths({ nat46: nat46({ mappings: [{ name: 'a', ipv4: '224.0.0.1', ipv6: 'fe80::1' }] }) })).toEqual([
      'nat/nat46/mappings/0/ipv4',
      'nat/nat46/mappings/0/ipv6',
    ]);
    expect(paths({ nat46: nat46({ interfaces: [] }) })).toEqual(['nat/nat46/interfaces']);
  });
});

describe('nat.nat46 semantic rules', () => {
  it('accepts the valid object', () => {
    expect(at({ nat46: nat46() })).toEqual([]);
  });
  it('refuses an unknown interface', () => {
    expect(at({ nat46: nat46({ interfaces: ['host-nope'] }) })).toEqual(['/nat/nat46/interfaces/0']);
  });
  const map = (over: Record<string, unknown>) => ({
    domains: [
      {
        name: 'd1',
        mode: 'map-t',
        ipv4Prefix: '198.51.100.0/24',
        ipv6Prefix: '2001:db8:2::/48',
        ipv6Source: '2001:db8:ffff::/96',
        ...over,
      },
    ],
  });
  it('refuses a service address inside a nat.map IPv4 prefix', () => {
    expect(at({ nat46: nat46(), map: map({}) })).toEqual(['/nat/nat46/mappings/0/ipv4']);
    expect(at({ nat46: nat46(), map: map({ ipv4Prefix: '203.0.113.0/24' }) })).toEqual([]);
  });
  it('enforces the nat46- name guard both ways', () => {
    const p = at({ nat46: nat46(), map: map({ ipv4Prefix: '203.0.113.0/24', name: 'nat46-web' }) });
    expect(p).toContain('/nat/nat46/mappings/0/name');
    expect(p).toContain('/nat/map/domains/0/name');
  });
  it('refuses a NAT46 interface bound map-e in nat.map, accepts map-t', () => {
    expect(at({ nat46: nat46(), map: { interfaces: [{ interface: 'host-w4l0', mode: 'map-e' }] } })).toEqual([
      '/nat/nat46/interfaces/0',
    ]);
    expect(at({ nat46: nat46(), map: { interfaces: [{ interface: 'host-w4l0', mode: 'map-t' }] } })).toEqual([]);
  });
});
