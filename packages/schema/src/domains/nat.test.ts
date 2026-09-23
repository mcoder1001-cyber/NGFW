import { describe, expect, it } from 'vitest';
import { z } from 'zod';
import {
  CnatSchema,
  Det44Schema,
  DsliteSchema,
  ipv4AddressRange,
  MapDomainSchema,
  Nat64Schema,
  NatIdentityMappingSchema,
  NatLoadBalancedMappingSchema,
  NatPoolSchema,
  NatSchema,
  NatStaticMappingSchema,
  NatTimeoutsSchema,
  Nptv6Schema,
  splitIpv4Range,
} from './nat.js';

const ok = (schema: z.ZodType, value: unknown): void =>
  expect(schema.safeParse(value).success, JSON.stringify(value)).toBe(true);
const bad = (schema: z.ZodType, value: unknown): void =>
  expect(schema.safeParse(value).success, JSON.stringify(value)).toBe(false);

describe('NatSchema', () => {
  it('accepts {} and fills NAT44 defaults (F-nat44 contract shape)', () => {
    const nat = NatSchema.parse({});
    expect(nat).toMatchObject({
      enabled: false,
      mode: 'ed',
      inside: [],
      outside: [],
      outputFeature: [],
      forwarding: false,
      staticMappingOnly: false,
      connectionTracking: false,
      pools: [],
      staticMappings: [],
      identityMappings: [],
      loadBalancedMappings: [],
      timeouts: { udp: 300, tcpEstablished: 7440, tcpTransitory: 240, icmp: 60 },
      ipfix: { enabled: false },
      nat64: { enabled: false, inside: [], outside: [], prefixes: [], pools: [], staticBibs: [] },
      nat66: { enabled: false, staticMappings: [] },
      nptv6: { bindings: [] },
      det44: { enabled: false, mappings: [] },
      dslite: { enabled: false, pools: [] },
      map: { domains: [], parameters: { fragmentation: { inner: false, ignoreDf: false } } },
      cnat: { translations: [], snat: { policy: 'none', interfaces: [], excludePrefixes: [] } },
    });
    expect(nat.sessionLimit).toBeUndefined();
    expect(nat.insideVrf).toBeUndefined();
  });

  it('is strict about keys, mode and sessionLimit (≥ 1024)', () => {
    bad(NatSchema, { insideInterfaces: [] });
    bad(NatSchema, { mode: 'ED' });
    bad(NatSchema, { mode: 'endpoint-dependent' });
    ok(NatSchema, { mode: 'ei' });
    bad(NatSchema, { sessionLimit: 1023 });
    bad(NatSchema, { sessionLimit: 1024.5 });
    bad(NatSchema, { sessionLimit: '2048' });
    bad(NatSchema, { sessionLimit: 2 ** 31 });
    ok(NatSchema, { sessionLimit: 1024 });
    bad(NatSchema, { inside: ['Gig 0/0/0'] });
    bad(NatSchema, { inside: 'Gig0/0/0' });
    ok(NatSchema, {
      inside: ['Gig0/0/0', 'host-w5-eth0'],
      outside: ['TenGigabitEthernet0/0/0.100'],
    });
    bad(NatSchema, { insideVrf: 'a b' });
    ok(NatSchema, { insideVrf: 'customer-a', outsideVrf: 'default' });
    bad(NatSchema, null);
    bad(NatSchema, []);
  });

  it('exposes title, description and navigation order for the UI', () => {
    const js = z.toJSONSchema(NatSchema, { target: 'draft-2020-12', io: 'input' });
    expect(js.title).toBe('NAT');
    expect(js['x-vrx-ui']).toMatchObject({ order: 60 });
    const props = js.properties as Record<string, Record<string, unknown>>;
    expect(props.pools?.['x-vrx-ui']).toMatchObject({ group: 'Pools' });
    expect(props.sessionLimit).toMatchObject({ minimum: 1024 });
  });
});

describe('ipv4AddressRange / splitIpv4Range', () => {
  it.each([
    '10.0.0.1',
    '10.0.0.1-10.0.0.9',
    '203.0.113.255-203.0.113.255',
    '0.0.0.0-255.255.255.255',
  ])('accepts %s', (v) => ok(ipv4AddressRange, v));

  it.each([
    '',
    '10.0.0.1/24',
    '10.0.0.256-10.0.0.9',
    '10.0.0.1-10.0.0',
    '10.0.0.1 - 10.0.0.9',
    ' 10.0.0.1',
    '10.0.0.1-',
    '-10.0.0.1',
    '10.0.0.1-10.0.0.2-10.0.0.3',
    '2001:db8::1-2001:db8::9',
    '10.0.0.01-10.0.0.9',
    'a.b.c.d',
  ])('rejects %j', (v) => bad(ipv4AddressRange, v));

  it('splits ranges and single addresses; undefined on garbage', () => {
    expect(splitIpv4Range('10.0.0.1-10.0.0.9')).toEqual({ start: '10.0.0.1', end: '10.0.0.9' });
    expect(splitIpv4Range('10.0.0.1')).toEqual({ start: '10.0.0.1', end: '10.0.0.1' });
    expect(splitIpv4Range('10.0.0.1/24')).toBeUndefined();
    expect(splitIpv4Range('')).toBeUndefined();
  });
});

describe('pools and mappings', () => {
  it('NatPoolSchema', () => {
    expect(NatPoolSchema.parse({ name: 'p', range: '203.0.113.1' })).toEqual({
      name: 'p',
      range: '203.0.113.1',
      twiceNat: false,
    });
    bad(NatPoolSchema, { range: '203.0.113.1' });
    bad(NatPoolSchema, { name: 'p q', range: '203.0.113.1' });
    bad(NatPoolSchema, { name: 'p', range: '203.0.113.0/24' });
    bad(NatPoolSchema, { name: 'p', range: '203.0.113.1', extra: true });
    bad(NatPoolSchema, { name: 'p', range: '203.0.113.1', description: 'x'.repeat(256) });
    ok(NatPoolSchema, {
      name: 'p',
      range: '203.0.113.1',
      description: 'x'.repeat(255),
      vrf: 'default',
    });
  });

  it('NatStaticMappingSchema — 1:1, port forward, pool, interface', () => {
    const one = NatStaticMappingSchema.parse({
      name: 'm',
      local: { ip: '10.0.0.1' },
      external: { ip: '203.0.113.1' },
    });
    expect(one).toMatchObject({ twiceNat: false, selfTwiceNat: false, out2inOnly: false });
    expect(one.protocol).toBeUndefined();
    ok(NatStaticMappingSchema, {
      name: 'pf',
      protocol: 'tcp',
      local: { ip: '10.0.0.1', port: 80 },
      external: { pool: 'wan', port: 8080 },
      vrf: 'default',
      twiceNat: true,
    });
    ok(NatStaticMappingSchema, {
      name: 'if',
      protocol: 'udp',
      local: { ip: '10.0.0.1', port: 53 },
      external: { interface: 'TenGigabitEthernet0/0/0', port: 53 },
    });
    bad(NatStaticMappingSchema, { local: { ip: '10.0.0.1' }, external: { ip: '203.0.113.1' } });
    bad(NatStaticMappingSchema, {
      name: 'm',
      local: { ip: '2001:db8::1' },
      external: { ip: '203.0.113.1' },
    });
    bad(NatStaticMappingSchema, {
      name: 'm',
      local: { ip: '10.0.0.1' },
      external: { ip: '203.0.113.1', port: 0 },
    });
    bad(NatStaticMappingSchema, {
      name: 'm',
      local: { ip: '10.0.0.1', port: 65536 },
      external: { ip: '203.0.113.1' },
    });
    bad(NatStaticMappingSchema, {
      name: 'm',
      local: { ip: '10.0.0.1', port: '80' },
      external: { ip: '203.0.113.1' },
    });
    bad(NatStaticMappingSchema, {
      name: 'm',
      protocol: 'gre',
      local: { ip: '10.0.0.1' },
      external: { ip: '203.0.113.1' },
    });
    bad(NatStaticMappingSchema, {
      name: 'm',
      local: { ip: '10.0.0.1' },
      external: { ip: '203.0.113.1' },
      external_port: 1,
    });
    bad(NatStaticMappingSchema, {
      name: 'm',
      local: { ip: '10.0.0.1' },
      external: { pool: 'bad name' },
    });
    bad(NatStaticMappingSchema, {
      name: 'm',
      local: { ip: '10.0.0.1' },
      external: { interface: 'eth 0' },
    });
    bad(NatStaticMappingSchema, { name: 'm', local: '10.0.0.1', external: { ip: '203.0.113.1' } });
  });

  it('NatIdentityMappingSchema / NatLoadBalancedMappingSchema', () => {
    ok(NatIdentityMappingSchema, { ip: '203.0.113.1', protocol: 'tcp', port: 22 });
    ok(NatIdentityMappingSchema, { interface: 'Gig0/0/0' });
    bad(NatIdentityMappingSchema, { ip: '203.0.113.1', port: 70000 });
    bad(NatIdentityMappingSchema, { ip: '203.0.113.1', protocol: 'sctp' });
    const lb = {
      name: 'lb',
      protocol: 'tcp',
      external: { ip: '203.0.113.1', port: 80 },
      locals: [{ ip: '10.0.0.1', port: 8080 }],
    };
    expect(NatLoadBalancedMappingSchema.parse(lb)).toMatchObject({
      affinity: 0,
      locals: [{ probability: 1 }],
    });
    bad(NatLoadBalancedMappingSchema, { ...lb, locals: [] });
    bad(NatLoadBalancedMappingSchema, { ...lb, protocol: 'icmp' });
    bad(NatLoadBalancedMappingSchema, {
      ...lb,
      locals: [{ ip: '10.0.0.1', port: 80, probability: 0 }],
    });
    bad(NatLoadBalancedMappingSchema, {
      ...lb,
      locals: [{ ip: '10.0.0.1', port: 80, probability: 256 }],
    });
    bad(NatLoadBalancedMappingSchema, { ...lb, affinity: -1 });
    bad(NatLoadBalancedMappingSchema, { ...lb, external: { ip: '203.0.113.1' } });
  });

  it('NatTimeoutsSchema', () => {
    expect(NatTimeoutsSchema.parse({})).toEqual({
      udp: 300,
      tcpEstablished: 7440,
      tcpTransitory: 240,
      icmp: 60,
    });
    bad(NatTimeoutsSchema, { udp: 0 });
    bad(NatTimeoutsSchema, { udp: 1.5 });
    bad(NatTimeoutsSchema, { udp: 2 ** 31 });
    bad(NatTimeoutsSchema, { tcp: 10 });
    ok(NatTimeoutsSchema, { udp: 1, tcpEstablished: 2 ** 31 - 1 });
  });
});

describe('other translators', () => {
  it('Nat64Schema', () => {
    ok(Nat64Schema, { prefixes: [{ prefix: '64:ff9b::/96' }], pools: [{ range: '198.51.100.1' }] });
    bad(Nat64Schema, { prefixes: [{ prefix: '10.0.0.0/8' }] });
    bad(Nat64Schema, { pools: [{ range: '2001:db8::/64' }] });
    bad(Nat64Schema, {
      staticBibs: [
        {
          protocol: 'tcp',
          inside: { ip: '10.0.0.1', port: 80 },
          outside: { ip: '198.51.100.1', port: 80 },
        },
      ],
    });
    bad(Nat64Schema, {
      staticBibs: [
        {
          protocol: 'tcp',
          inside: { ip: '2001:db8::1', port: 80 },
          outside: { ip: '2001:db8::2', port: 80 },
        },
      ],
    });
    ok(Nat64Schema, {
      staticBibs: [
        {
          protocol: 'icmp',
          inside: { ip: '2001:db8::1', port: 1 },
          outside: { ip: '198.51.100.1', port: 1 },
        },
      ],
    });
  });

  it('Nptv6Schema / Det44Schema / DsliteSchema', () => {
    ok(Nptv6Schema, {
      bindings: [{ interface: 'Gig0/0/0', internal: 'fd00::/48', external: '2001:db8::/48' }],
    });
    bad(Nptv6Schema, {
      bindings: [{ interface: 'Gig0/0/0', internal: '10.0.0.0/8', external: '2001:db8::/48' }],
    });
    bad(Nptv6Schema, { bindings: [{ internal: 'fd00::/48', external: '2001:db8::/48' }] });
    ok(Det44Schema, {
      mappings: [{ inside: '10.0.0.0/16', outside: '203.0.113.0/28' }],
      insideVrf: 'a',
    });
    bad(Det44Schema, { mappings: [{ inside: 'fd00::/48', outside: '203.0.113.0/28' }] });
    bad(Det44Schema, { mappings: [{ inside: '10.0.0.0/16' }] });
    ok(DsliteSchema, { aftr: { ipv6: '2001:db8::1', ipv4: '192.0.0.1' } });
    bad(DsliteSchema, { aftr: { ipv6: '192.0.0.1' } });
    bad(DsliteSchema, { b4: { ipv4: '192.0.0.2' } });
    bad(DsliteSchema, { pools: [{ range: '192.0.2.0/24' }] });
  });

  it('MapDomainSchema', () => {
    const d = {
      name: 'd',
      mode: 'map-e',
      ipv4Prefix: '192.0.2.0/24',
      ipv6Prefix: '2001:db8::/40',
      ipv6Source: '2001:db8:ffff::1/128',
    };
    expect(MapDomainSchema.parse(d)).toMatchObject({
      eaBitsLength: 0,
      psidOffset: 0,
      psidLength: 0,
      rules: [],
    });
    ok(MapDomainSchema, {
      ...d,
      mode: 'map-t',
      eaBitsLength: 64,
      psidOffset: 16,
      psidLength: 0,
      mtu: 1280,
    });
    ok(MapDomainSchema, {
      ...d,
      mode: 'lw4o6',
      rules: [{ psid: 65535, ipv6Destination: '2001:db8::1' }],
    });
    bad(MapDomainSchema, { ...d, mode: 'map-x' });
    bad(MapDomainSchema, { ...d, eaBitsLength: 65 });
    bad(MapDomainSchema, { ...d, psidLength: 17 });
    bad(MapDomainSchema, { ...d, psidOffset: -1 });
    bad(MapDomainSchema, { ...d, mtu: 1279 });
    bad(MapDomainSchema, { ...d, mtu: 9217 });
    bad(MapDomainSchema, { ...d, rules: [{ psid: 65536, ipv6Destination: '2001:db8::1' }] });
    bad(MapDomainSchema, { ...d, rules: [{ psid: 1, ipv6Destination: '10.0.0.1' }] });
    bad(MapDomainSchema, { ...d, ipv4Prefix: '2001:db8::/32' });
  });

  it('CnatSchema', () => {
    const t = {
      name: 't',
      protocol: 'tcp',
      vip: { ip: '203.0.113.1', port: 80 },
      backends: [{ ip: '10.0.0.1', port: 80 }],
    };
    expect(CnatSchema.parse({ translations: [t] }).translations[0]).toMatchObject({
      lbType: 'default',
    });
    ok(CnatSchema, {
      translations: [{ ...t, vip: { ip: '2001:db8::1', port: 80 }, lbType: 'maglev' }],
    });
    bad(CnatSchema, { translations: [{ ...t, backends: [] }] });
    bad(CnatSchema, { translations: [{ ...t, lbType: 'round-robin' }] });
    bad(CnatSchema, { translations: [{ ...t, protocol: 'icmp' }] });
    bad(CnatSchema, { translations: [{ ...t, vip: { ip: '203.0.113.1' } }] });
    ok(CnatSchema, {
      snat: {
        policy: 'k8s',
        interfaces: [{ interface: 'Gig0/0/0', side: 'inside' }],
        excludePrefixes: ['10.0.0.0/8', 'fd00::/8'],
      },
    });
    bad(CnatSchema, { snat: { policy: 'bar' } });
    bad(CnatSchema, { snat: { interfaces: [{ interface: 'Gig0/0/0', side: 'both' }] } });
    bad(CnatSchema, { snat: { excludePrefixes: ['10.0.0.1'] } });
    bad(CnatSchema, { snat: { addresses: { ipv4: '2001:db8::1' } } });
  });

  it('ipfix', () => {
    ok(NatSchema, { ipfix: { enabled: true, domainId: 1, sourcePort: 4739 } });
    bad(NatSchema, { ipfix: { domainId: 0 } });
    bad(NatSchema, { ipfix: { domainId: 2 ** 32 } });
    bad(NatSchema, { ipfix: { sourcePort: 0 } });
  });
});
