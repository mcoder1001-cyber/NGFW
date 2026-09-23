import { describe, expect, it } from 'vitest';
import { RootConfig } from '../index.js';
import { duplicates, isNetworkPrefix, natValidators, networkOf, poolRange } from './nat.js';
import { sortIssues, type SemanticIssue } from './registry.js';

const interfaces = {
  'Gig0/0/0': { vrf: 'default' },
  'Gig0/0/1': { vrf: 'default' },
  'Gig0/0/2': { vrf: 'cust' },
};
const vrfs = { cust: { id: 10 } }; // group (a) VrfSchema requires the table id (P02a merge)

function run(name: string, nat: unknown, extra: Record<string, unknown> = {}): SemanticIssue[] {
  const v = natValidators.find((x) => x.name === name);
  if (v === undefined) throw new Error(`no validator '${name}'`);
  return sortIssues(v.validate(RootConfig.parse({ interfaces, vrfs, nat, ...extra })));
}

const pointers = (issues: SemanticIssue[]): string[] => issues.map((i) => i.pointer);

const mapping = (
  name: string,
  external: Record<string, unknown>,
  extra: Record<string, unknown> = {},
) => ({
  name,
  local: { ip: '192.168.1.10' },
  external,
  ...extra,
});

describe('helpers', () => {
  it('poolRange parses ranges and single addresses, throws on garbage', () => {
    expect(poolRange('10.0.0.1-10.0.0.5')).toEqual({
      family: 4,
      start: 167772161n,
      end: 167772165n,
    });
    expect(poolRange('10.0.0.1')).toEqual({ family: 4, start: 167772161n, end: 167772161n });
    expect(() => poolRange('nope')).toThrow(/invalid IPv4 range/);
  });

  it('duplicates reports later repeats against the first index and skips undefined keys', () => {
    expect(duplicates([], () => 'k')).toEqual([]);
    expect(duplicates(['a', 'b', 'a', 'a'], (s) => s)).toEqual([
      { index: 2, first: 0, item: 'a' },
      { index: 3, first: 0, item: 'a' },
    ]);
    expect(duplicates([1, 1, 2], (n) => (n === 1 ? undefined : String(n)))).toEqual([]);
  });

  it('isNetworkPrefix / networkOf detect and clear host bits (IPv6 compressed per RFC 5952)', () => {
    expect(isNetworkPrefix('10.0.0.0/24')).toBe(true);
    expect(isNetworkPrefix('10.0.0.1/24')).toBe(false);
    expect(isNetworkPrefix('10.0.0.1/32')).toBe(true);
    expect(isNetworkPrefix('0.0.0.0/0')).toBe(true);
    expect(isNetworkPrefix('64:ff9b::/96')).toBe(true);
    expect(isNetworkPrefix('64:ff9b::1/96')).toBe(false);
    expect(isNetworkPrefix('2001:db8::1/128')).toBe(true);
    expect(networkOf('10.0.0.1/24')).toBe('10.0.0.0/24');
    expect(networkOf('100.64.255.255/10')).toBe('100.64.0.0/10');
    expect(networkOf('255.255.255.255/0')).toBe('0.0.0.0/0');
    expect(networkOf('64:ff9b::1/96')).toBe('64:ff9b::/96');
    expect(networkOf('2001:db8:0:1::5/64')).toBe('2001:db8:0:1::/64');
    expect(networkOf('1:0:0:2:0:0:0:3/128')).toBe('1:0:0:2::3/128');
    expect(networkOf('1:2:3:4:5:6:7:8/128')).toBe('1:2:3:4:5:6:7:8/128');
    expect(networkOf('1:0:2:0:3:0:4:0/128')).toBe('1:0:2:0:3:0:4:0/128');
    expect(networkOf('::1/128')).toBe('::1/128');
    expect(networkOf('ffff::1/0')).toBe('::/0');
  });
});

describe('nat.interfaces-exist', () => {
  it('reports every reference to an unknown interface', () => {
    const issues = run('nat.interfaces-exist', {
      inside: ['Gig0/0/0', 'nope'],
      outside: ['nope'],
      outputFeature: ['nope'],
      staticMappings: [mapping('m', { interface: 'nope' }), mapping('n', { ip: '1.1.1.1' })],
      identityMappings: [{ interface: 'nope' }, { ip: '1.1.1.1' }],
      nat64: { inside: ['nope'], outside: ['Gig0/0/1'] },
      nat66: { inside: [], outside: ['nope'] },
      det44: { inside: ['nope'], outside: [] },
      nptv6: {
        bindings: [{ interface: 'nope', internal: 'fd00::/48', external: '2001:db8::/48' }],
      },
      pools: [
        { name: 'ok', range: '203.0.113.1' },
        { name: 'wan', interface: 'nope' },
        { name: 'wan2', interface: 'Gig0/0/1' },
      ],
      map: {
        interfaces: [
          { interface: 'nope', mode: 'map-e' },
          { interface: 'Gig0/0/1', mode: 'map-t' },
        ],
      },
      cnat: {
        snat: {
          addresses: { interface: 'nope' },
          interfaces: [{ interface: 'nope', table: 'include-v4' }],
        },
      },
    });
    expect(pointers(issues)).toEqual([
      '/nat/cnat/snat/addresses/interface',
      '/nat/cnat/snat/interfaces/0/interface',
      '/nat/det44/inside/0',
      '/nat/identityMappings/0/interface',
      '/nat/inside/1',
      '/nat/map/interfaces/0/interface',
      '/nat/nat64/inside/0',
      '/nat/nat66/outside/0',
      '/nat/nptv6/bindings/0/interface',
      '/nat/outputFeature/0',
      '/nat/outside/0',
      '/nat/pools/1/interface',
      '/nat/staticMappings/0/external/interface',
    ]);
    expect(issues[0]?.message).toBe("interface 'nope' does not exist");
  });

  it('accepts existing interfaces', () => {
    expect(run('nat.interfaces-exist', { inside: ['Gig0/0/0'], outside: ['Gig0/0/1'] })).toEqual(
      [],
    );
  });
});

describe('nat.inside-outside-disjoint', () => {
  it('rejects repeats and interfaces on both sides, for every translator', () => {
    expect(
      pointers(
        run('nat.inside-outside-disjoint', {
          inside: ['Gig0/0/0', 'Gig0/0/0'],
          outside: ['Gig0/0/0', 'Gig0/0/1', 'Gig0/0/1'],
          outputFeature: ['Gig0/0/1', 'Gig0/0/1'],
          nat64: { inside: ['Gig0/0/2'], outside: ['Gig0/0/2'] },
          nat66: { inside: ['Gig0/0/2'], outside: ['Gig0/0/2'] },
          det44: { inside: ['Gig0/0/2'], outside: ['Gig0/0/2'] },
        }),
      ),
    ).toEqual([
      '/nat/det44/outside/0',
      '/nat/inside/1',
      '/nat/nat64/outside/0',
      '/nat/nat66/outside/0',
      '/nat/outputFeature/1',
      '/nat/outside/0',
      '/nat/outside/2',
    ]);
  });

  it('accepts disjoint sides', () => {
    expect(
      run('nat.inside-outside-disjoint', { inside: ['Gig0/0/0'], outside: ['Gig0/0/1'] }),
    ).toEqual([]);
  });
});

describe('nat.pools-valid', () => {
  it('rejects duplicate names, reversed ranges and overlaps in every pool list', () => {
    const issues = run('nat.pools-valid', {
      pools: [
        { name: 'a', range: '203.0.113.10-203.0.113.20' },
        { name: 'a', range: '203.0.113.20' },
        { name: 'rev', range: '203.0.113.50-203.0.113.40' },
        { name: 'ok', range: '203.0.113.21-203.0.113.30' },
      ],
      nat64: { pools: [{ range: '198.51.100.1-198.51.100.9' }, { range: '198.51.100.5' }] },
      dslite: { pools: [{ range: '192.0.2.1' }, { range: '192.0.2.1' }] },
    });
    expect(issues).toEqual([
      {
        pointer: '/nat/dslite/pools/1/range',
        message: "range '192.0.2.1' overlaps ds-lite pool 0",
      },
      {
        pointer: '/nat/nat64/pools/1/range',
        message: "range '198.51.100.5' overlaps nat64 pool 0",
      },
      { pointer: '/nat/pools/1/name', message: "pool name 'a' is used twice" },
      { pointer: '/nat/pools/1/range', message: "range '203.0.113.20' overlaps pool 'a'" },
      {
        pointer: '/nat/pools/2/range',
        message: "range '203.0.113.50-203.0.113.40' ends before it starts",
      },
    ]);
  });

  it('accepts adjacent, ordered pools', () => {
    expect(
      run('nat.pools-valid', {
        pools: [
          { name: 'a', range: '203.0.113.10-203.0.113.20' },
          { name: 'b', range: '203.0.113.21-203.0.113.30' },
        ],
      }),
    ).toEqual([]);
  });

  it('checks overlap per twice-NAT class (separate VPP lists) and interface pools per interface', () => {
    expect(
      run('nat.pools-valid', {
        pools: [
          { name: 'a', range: '203.0.113.10-203.0.113.20' },
          { name: 'tn', range: '203.0.113.10-203.0.113.20', twiceNat: true },
          { name: 'tn2', range: '203.0.113.15', twiceNat: true },
          { name: 'wan', interface: 'Gig0/0/0' },
          { name: 'wan2', interface: 'Gig0/0/0' },
          { name: 'wan-tn', interface: 'Gig0/0/0', twiceNat: true },
          { name: 'lan', interface: 'Gig0/0/1' },
        ],
      }),
    ).toEqual([
      { pointer: '/nat/pools/2/range', message: "range '203.0.113.15' overlaps pool 'tn'" },
      {
        pointer: '/nat/pools/4/interface',
        message: "interface 'Gig0/0/0' is already used by pool 'wan'",
      },
    ]);
  });
});

describe('nat.static-mappings', () => {
  const pools = [{ name: 'p', range: '203.0.113.1' }];

  it('requires exactly one external side and an existing pool', () => {
    expect(
      run('nat.static-mappings', {
        pools,
        staticMappings: [
          mapping('none', {}),
          mapping('two', { ip: '203.0.113.2', pool: 'p' }),
          mapping('three', { ip: '203.0.113.2', pool: 'p', interface: 'Gig0/0/1' }),
          mapping('badpool', { pool: 'nope' }),
        ],
      }),
    ).toEqual([
      {
        pointer: '/nat/staticMappings/0/external',
        message: expect.stringMatching(/exactly one of/),
      },
      {
        pointer: '/nat/staticMappings/1/external',
        message: expect.stringMatching(/exactly one of/),
      },
      {
        pointer: '/nat/staticMappings/2/external',
        message: expect.stringMatching(/exactly one of/),
      },
      {
        pointer: '/nat/staticMappings/3/external/pool',
        message: "pool 'nope' does not exist in nat.pools",
      },
    ]);
  });

  it('ports come in pairs and require a non-ICMP protocol (F-nat44 rule)', () => {
    const issues = run('nat.static-mappings', {
      staticMappings: [
        {
          name: 'ext-only',
          local: { ip: '192.168.1.10' },
          external: { ip: '203.0.113.2', port: 8080 },
        },
        {
          name: 'local-only',
          local: { ip: '192.168.1.10', port: 80 },
          external: { ip: '203.0.113.3' },
          protocol: 'tcp',
        },
        {
          name: 'no-proto',
          local: { ip: '192.168.1.10', port: 80 },
          external: { ip: '203.0.113.4', port: 80 },
        },
        {
          name: 'icmp',
          local: { ip: '192.168.1.10', port: 80 },
          external: { ip: '203.0.113.5', port: 80 },
          protocol: 'icmp',
        },
        {
          name: 'ok',
          local: { ip: '192.168.1.10', port: 80 },
          external: { ip: '203.0.113.6', port: 8080 },
          protocol: 'tcp',
        },
      ],
    });
    expect(issues).toEqual([
      {
        pointer: '/nat/staticMappings/0/local/port',
        message: expect.stringMatching(/set together/),
      },
      {
        pointer: '/nat/staticMappings/0/protocol',
        message: 'protocol is required when ports are set',
      },
      {
        pointer: '/nat/staticMappings/1/external/port',
        message: expect.stringMatching(/set together/),
      },
      {
        pointer: '/nat/staticMappings/2/protocol',
        message: 'protocol is required when ports are set',
      },
      { pointer: '/nat/staticMappings/3/protocol', message: 'ICMP mappings cannot carry ports' },
    ]);
  });

  it('rejects twiceNat+selfTwiceNat, duplicate names and duplicate external tuples', () => {
    const issues = run('nat.static-mappings', {
      pools,
      staticMappings: [
        mapping('dup', { ip: '203.0.113.2' }, { twiceNat: true, selfTwiceNat: true }),
        mapping('dup', { ip: '203.0.113.2' }),
        mapping(
          'pf1',
          { ip: '203.0.113.3', port: 80 },
          { protocol: 'tcp', local: { ip: '10.0.0.1', port: 80 } },
        ),
        mapping(
          'pf2',
          { ip: '203.0.113.3', port: 80 },
          { protocol: 'tcp', local: { ip: '10.0.0.2', port: 80 } },
        ),
        mapping(
          'pf3',
          { ip: '203.0.113.3', port: 80 },
          { protocol: 'udp', local: { ip: '10.0.0.2', port: 80 } },
        ),
        mapping('vrf', { ip: '203.0.113.2' }, { vrf: 'cust' }),
        mapping('pool', { pool: 'p' }),
        mapping('pool2', { pool: 'p' }),
        mapping('if', { interface: 'Gig0/0/1' }),
      ],
    });
    expect(issues).toEqual([
      {
        pointer: '/nat/staticMappings/0/selfTwiceNat',
        message: 'twiceNat and selfTwiceNat are mutually exclusive',
      },
      {
        pointer: '/nat/staticMappings/1/external',
        message: expect.stringMatching(/as mapping 'dup'/),
      },
      { pointer: '/nat/staticMappings/1/name', message: "mapping name 'dup' is used twice" },
      {
        pointer: '/nat/staticMappings/3/external',
        message: expect.stringMatching(/as mapping 'pf1'/),
      },
      {
        pointer: '/nat/staticMappings/7/external',
        message: expect.stringMatching(/as mapping 'pool'/),
      },
    ]);
  });

  it('compares external.pool by its start address / interface, so ip and pool collide (L1)', () => {
    expect(
      run('nat.static-mappings', {
        pools: [
          { name: 'p', range: '203.0.113.1-203.0.113.5' },
          { name: 'wan', interface: 'Gig0/0/1' },
        ],
        staticMappings: [
          mapping('lit', { ip: '203.0.113.1' }),
          mapping('via-pool', { pool: 'p' }),
          mapping('via-if', { interface: 'Gig0/0/1' }),
          mapping('via-if-pool', { pool: 'wan' }),
          mapping('other', { ip: '203.0.113.2' }),
          mapping('unknown-a', { pool: 'nope' }),
          mapping('unknown-b', { pool: 'nope' }),
        ],
      }),
    ).toEqual([
      {
        pointer: '/nat/staticMappings/1/external',
        message: expect.stringMatching(/as mapping 'lit'/),
      },
      {
        pointer: '/nat/staticMappings/3/external',
        message: expect.stringMatching(/as mapping 'via-if'/),
      },
      {
        pointer: '/nat/staticMappings/5/external/pool',
        message: "pool 'nope' does not exist in nat.pools",
      },
      {
        pointer: '/nat/staticMappings/6/external',
        message: expect.stringMatching(/as mapping 'unknown-a'/),
      },
      {
        pointer: '/nat/staticMappings/6/external/pool',
        message: "pool 'nope' does not exist in nat.pools",
      },
    ]);
  });

  it('accepts the three classic scenarios (1:1, port forward, pool/interface external)', () => {
    expect(
      run('nat.static-mappings', {
        pools,
        staticMappings: [
          mapping('one-to-one', { ip: '203.0.113.2' }),
          mapping(
            'web',
            { ip: '203.0.113.3', port: 443 },
            { protocol: 'tcp', local: { ip: '10.0.0.1', port: 8443 } },
          ),
          mapping('via-pool', { pool: 'p' }),
          mapping(
            'via-if',
            { interface: 'Gig0/0/1', port: 22 },
            { protocol: 'tcp', local: { ip: '10.0.0.9', port: 22 } },
          ),
        ],
      }),
    ).toEqual([]);
  });
});

describe('nat.identity-mappings', () => {
  it('requires exactly one of ip/interface and a non-ICMP protocol with a port', () => {
    expect(
      run('nat.identity-mappings', {
        identityMappings: [
          {},
          { ip: '203.0.113.1', interface: 'Gig0/0/1' },
          { ip: '203.0.113.1', port: 53 },
          { ip: '203.0.113.1', port: 53, protocol: 'icmp' },
          { ip: '203.0.113.1', port: 53, protocol: 'udp' },
          { interface: 'Gig0/0/1' },
        ],
      }),
    ).toEqual([
      { pointer: '/nat/identityMappings/0', message: 'exactly one of ip or interface is required' },
      { pointer: '/nat/identityMappings/1', message: 'exactly one of ip or interface is required' },
      {
        pointer: '/nat/identityMappings/2/protocol',
        message: 'protocol is required when port is set',
      },
      {
        pointer: '/nat/identityMappings/3/protocol',
        message: 'ICMP identity mappings cannot carry a port',
      },
    ]);
  });
});

describe('nat.load-balanced-mappings', () => {
  const lb = (
    name: string,
    port: number,
    locals: { ip: string; port: number; vrf?: string }[],
  ) => ({
    name,
    protocol: 'tcp',
    external: { ip: '203.0.113.1', port },
    locals,
  });

  it('rejects duplicate names, external endpoints and local endpoints', () => {
    expect(
      run('nat.load-balanced-mappings', {
        loadBalancedMappings: [
          lb('a', 80, [
            { ip: '10.0.0.1', port: 80 },
            { ip: '10.0.0.1', port: 80 },
            { ip: '10.0.0.1', port: 80, vrf: 'cust' },
          ]),
          lb('a', 80, [{ ip: '10.0.0.2', port: 80 }]),
          lb('b', 443, [{ ip: '10.0.0.2', port: 443 }]),
        ],
      }),
    ).toEqual([
      {
        pointer: '/nat/loadBalancedMappings/0/locals/1',
        message: 'local endpoint 10.0.0.1:80 is listed twice',
      },
      {
        pointer: '/nat/loadBalancedMappings/1/external',
        message: "same external endpoint as mapping 'a'",
      },
      { pointer: '/nat/loadBalancedMappings/1/name', message: "mapping name 'a' is used twice" },
    ]);
  });
});

describe('nat.mode-ed-features', () => {
  const doc = {
    pools: [{ name: 'p', range: '203.0.113.1', twiceNat: true }],
    staticMappings: [
      mapping('m', { ip: '203.0.113.2' }, { twiceNat: true, selfTwiceNat: true, out2inOnly: true }),
    ],
    loadBalancedMappings: [
      {
        name: 'lb',
        protocol: 'tcp',
        external: { ip: '203.0.113.3', port: 80 },
        locals: [{ ip: '10.0.0.1', port: 80 }],
      },
    ],
  };

  it('rejects ED-only features in EI mode and accepts them in ED mode', () => {
    expect(pointers(run('nat.mode-ed-features', { ...doc, mode: 'ei' }))).toEqual([
      '/nat/loadBalancedMappings/0',
      '/nat/pools/0/twiceNat',
      '/nat/staticMappings/0/out2inOnly',
      '/nat/staticMappings/0/selfTwiceNat',
      '/nat/staticMappings/0/twiceNat',
    ]);
    expect(run('nat.mode-ed-features', { ...doc, mode: 'ed' })).toEqual([]);
    expect(
      run('nat.mode-ed-features', { mode: 'ei', pools: [{ name: 'p', range: '203.0.113.1' }] }),
    ).toEqual([]);
  });
});

describe('nat.vrfs-exist', () => {
  it('reports every unknown VRF reference and implies default', () => {
    const issues = run('nat.vrfs-exist', {
      insideVrf: 'nope',
      outsideVrf: 'default',
      pools: [
        { name: 'p', range: '203.0.113.1', vrf: 'cust' },
        { name: 'q', range: '203.0.113.2', vrf: 'nope' },
      ],
      staticMappings: [mapping('m', { ip: '203.0.113.3' }, { vrf: 'nope' })],
      identityMappings: [{ ip: '203.0.113.3', vrf: 'nope' }],
      loadBalancedMappings: [
        {
          name: 'lb',
          protocol: 'tcp',
          external: { ip: '203.0.113.4', port: 80 },
          locals: [{ ip: '10.0.0.1', port: 80, vrf: 'nope' }],
        },
      ],
      nat64: {
        prefixes: [{ prefix: '64:ff9b::/96', vrf: 'nope' }],
        pools: [{ range: '198.51.100.1', vrf: 'nope' }],
        staticBibs: [
          {
            protocol: 'tcp',
            inside: { ip: '2001:db8::1', port: 80 },
            outside: { ip: '198.51.100.1', port: 80 },
            vrf: 'nope',
          },
        ],
      },
      nat66: { staticMappings: [{ local: 'fd00::1', external: '2001:db8::1', vrf: 'nope' }] },
      det44: { insideVrf: 'nope', outsideVrf: 'cust' },
    });
    expect(pointers(issues)).toEqual([
      '/nat/det44/insideVrf',
      '/nat/identityMappings/0/vrf',
      '/nat/insideVrf',
      '/nat/loadBalancedMappings/0/locals/0/vrf',
      '/nat/nat64/pools/0/vrf',
      '/nat/nat64/prefixes/0/vrf',
      '/nat/nat64/staticBibs/0/vrf',
      '/nat/nat66/staticMappings/0/vrf',
      '/nat/pools/1/vrf',
      '/nat/staticMappings/0/vrf',
    ]);
    expect(issues[0]?.message).toBe("VRF 'nope' does not exist");
  });
});

describe('nat.nat64-valid', () => {
  const bib = (inPort: number, outPort: number) => ({
    protocol: 'tcp',
    inside: { ip: '2001:db8::1', port: inPort },
    outside: { ip: '198.51.100.1', port: outPort },
  });

  it('rejects bad prefix lengths, a second prefix per VRF and duplicate BIB tuples', () => {
    expect(
      run('nat.nat64-valid', {
        nat64: {
          prefixes: [
            { prefix: '64:ff9b::/96' },
            { prefix: '2001:db8::/50' },
            { prefix: '2001:db8:1::/48', vrf: 'cust' },
            { prefix: '2001:db8:2::/48', vrf: 'cust' },
          ],
          staticBibs: [bib(80, 80), bib(80, 8080), bib(81, 80)],
        },
      }),
    ).toEqual([
      { pointer: '/nat/nat64/prefixes/1', message: "VRF 'default' already has a NAT64 prefix" },
      {
        pointer: '/nat/nat64/prefixes/1/prefix',
        message: expect.stringMatching(/32, 40, 48, 56, 64 or 96/),
      },
      { pointer: '/nat/nat64/prefixes/3', message: "VRF 'cust' already has a NAT64 prefix" },
      {
        pointer: '/nat/nat64/staticBibs/1/inside',
        message: 'inside 2001:db8::1:80/tcp already has a BIB entry',
      },
      {
        pointer: '/nat/nat64/staticBibs/2/outside',
        message: 'outside 198.51.100.1:80/tcp already has a BIB entry',
      },
    ]);
  });
});

describe('nat.nptv6-valid', () => {
  it('rejects unequal prefix lengths and two bindings per interface', () => {
    expect(
      run('nat.nptv6-valid', {
        nptv6: {
          bindings: [
            { interface: 'Gig0/0/0', internal: 'fd00::/48', external: '2001:db8::/56' },
            { interface: 'Gig0/0/0', internal: 'fd01::/48', external: '2001:db8:1::/48' },
          ],
        },
      }),
    ).toEqual([
      { pointer: '/nat/nptv6/bindings/0/external', message: expect.stringMatching(/same length/) },
      {
        pointer: '/nat/nptv6/bindings/1/interface',
        message: "interface 'Gig0/0/0' already has an NPTv6 binding",
      },
    ]);
  });
});

describe('nat.det44-valid', () => {
  it('rejects a too-short outside prefix, an impossible sharing ratio and overlaps', () => {
    expect(
      run('nat.det44-valid', {
        det44: {
          mappings: [
            { inside: '10.0.0.0/24', outside: '203.0.113.0/16' },
            { inside: '172.16.0.0/8', outside: '198.51.100.0/24' },
            { inside: '172.16.5.0/16', outside: '198.51.100.0/28' },
          ],
        },
      }),
    ).toEqual([
      {
        pointer: '/nat/det44/mappings/0/outside',
        message: expect.stringMatching(/at least as long/),
      },
      { pointer: '/nat/det44/mappings/1/outside', message: expect.stringMatching(/2\^15/) },
      { pointer: '/nat/det44/mappings/2/inside', message: "'172.16.5.0/16' overlaps entry 1" },
      { pointer: '/nat/det44/mappings/2/outside', message: "'198.51.100.0/28' overlaps entry 1" },
    ]);
    expect(
      run('nat.det44-valid', {
        det44: { mappings: [{ inside: '10.0.0.0/16', outside: '203.0.113.0/28' }] },
      }),
    ).toEqual([]);
  });
});

describe('nat.map-valid', () => {
  const domain = (name: string, extra: Record<string, unknown>) => ({
    name,
    mode: 'map-e',
    ipv4Prefix: '192.0.2.0/24',
    ipv6Prefix: '2001:db8::/40',
    ipv6Source: '2001:db8:ffff::1/128',
    ...extra,
  });

  it('ties mode to the ipv6Source length and binds each interface once (M2)', () => {
    expect(
      run('nat.map-valid', {
        map: {
          interfaces: [
            { interface: 'Gig0/0/0', mode: 'map-e' },
            { interface: 'Gig0/0/0', mode: 'map-t' },
            { interface: 'Gig0/0/1', mode: 'map-t' },
          ],
          domains: [
            domain('e-ok', {}),
            domain('e-bad', { ipv6Source: '2001:db8:ffff::/64' }),
            domain('lw-bad', { mode: 'lw4o6', ipv6Source: '2001:db8:ffff::/96' }),
            domain('t-64', { mode: 'map-t', ipv6Source: '2001:db8:ffff::/64' }),
            domain('t-96', { mode: 'map-t', ipv6Source: '64:ff9b::/96' }),
            domain('t-bad', { mode: 'map-t', ipv6Source: '2001:db8:ffff::1/128' }),
            domain('t-bad2', { mode: 'map-t', ipv6Source: '2001:db8::/48' }),
          ],
        },
      }),
    ).toEqual([
      {
        pointer: '/nat/map/domains/1/ipv6Source',
        message: expect.stringMatching(/^MAP-E needs the BR address as ipv6Source/),
      },
      {
        pointer: '/nat/map/domains/2/ipv6Source',
        message: expect.stringMatching(/^lw4o6 needs the BR address as ipv6Source/),
      },
      {
        pointer: '/nat/map/domains/5/ipv6Source',
        message: expect.stringMatching(/^MAP-T needs the DMR prefix/),
      },
      {
        pointer: '/nat/map/domains/6/ipv6Source',
        message: expect.stringMatching(/^MAP-T needs the DMR prefix/),
      },
      {
        pointer: '/nat/map/interfaces/1/interface',
        message: "interface 'Gig0/0/0' is already bound to MAP",
      },
    ]);
  });

  it('rejects duplicate names, EA bits past /64, PSID overflow and rules with EA bits', () => {
    expect(
      run('nat.map-valid', {
        map: {
          domains: [
            domain('d', {
              ipv6Prefix: '2001:db8::/56',
              eaBitsLength: 10,
              psidOffset: 10,
              psidLength: 8,
            }),
            domain('d', {
              eaBitsLength: 4,
              psidLength: 4,
              rules: [
                { psid: 16, ipv6Destination: '2001:db8::1' },
                { psid: 16, ipv6Destination: '2001:db8::2' },
              ],
            }),
            domain('lw', {
              mode: 'lw4o6',
              psidLength: 6,
              rules: [{ psid: 5, ipv6Destination: '2001:db8::5' }],
            }),
            domain('zero', { rules: [{ psid: 0, ipv6Destination: '2001:db8::1' }] }),
          ],
        },
      }),
    ).toEqual([
      {
        pointer: '/nat/map/domains/0/eaBitsLength',
        message: 'ipv6Prefix length plus EA bits must not exceed 64',
      },
      {
        pointer: '/nat/map/domains/0/psidLength',
        message: 'psidOffset plus psidLength must not exceed 16',
      },
      { pointer: '/nat/map/domains/1/name', message: "domain name 'd' is used twice" },
      {
        pointer: '/nat/map/domains/1/rules',
        message: expect.stringMatching(/require eaBitsLength 0/),
      },
      {
        pointer: '/nat/map/domains/1/rules/0/psid',
        message: 'PSID 16 does not fit in 4 PSID bits',
      },
      {
        pointer: '/nat/map/domains/1/rules/1/psid',
        message: 'PSID 16 does not fit in 4 PSID bits',
      },
      { pointer: '/nat/map/domains/1/rules/1/psid', message: 'PSID 16 is mapped twice' },
    ]);
  });
});

describe('nat.cnat-valid', () => {
  const t = (name: string, vip: string, backends: { ip: string; port: number }[]) => ({
    name,
    protocol: 'tcp',
    vip: { ip: vip, port: 80 },
    backends,
  });

  it('rejects duplicate names/VIPs, mixed families and duplicate backends', () => {
    expect(
      run('nat.cnat-valid', {
        cnat: {
          translations: [
            t('a', '203.0.113.1', [
              { ip: '10.0.0.1', port: 8080 },
              { ip: '2001:db8::1', port: 8080 },
              { ip: '10.0.0.1', port: 8080 },
            ]),
            t('a', '203.0.113.1', [{ ip: '10.0.0.2', port: 80 }]),
            t('b', '2001:db8::10', [{ ip: '2001:db8::2', port: 80 }]),
          ],
        },
      }),
    ).toEqual([
      {
        pointer: '/nat/cnat/translations/0/backends/1/ip',
        message: 'backend must be the same address family as the VIP',
      },
      {
        pointer: '/nat/cnat/translations/0/backends/2',
        message: 'backend 10.0.0.1:8080 is listed twice',
      },
      { pointer: '/nat/cnat/translations/1/name', message: "translation name 'a' is used twice" },
      {
        pointer: '/nat/cnat/translations/1/vip',
        message: 'VIP 203.0.113.1:80/tcp is translated twice',
      },
    ]);
  });

  it('rejects an interface twice in one SNAT table and tables the policy never consults (M3)', () => {
    expect(
      run('nat.cnat-valid', {
        cnat: {
          snat: {
            policy: 'interface',
            interfaces: [
              { interface: 'Gig0/0/0', table: 'include-v4' },
              { interface: 'Gig0/0/0', table: 'include-v6' },
              { interface: 'Gig0/0/0', table: 'include-v4' },
              { interface: 'Gig0/0/1', table: 'pod' },
            ],
          },
        },
      }),
    ).toEqual([
      {
        pointer: '/nat/cnat/snat/interfaces/2',
        message: "interface 'Gig0/0/0' is already in SNAT table 'include-v4'",
      },
      {
        pointer: '/nat/cnat/snat/interfaces/3/table',
        message: expect.stringMatching(/policy 'interface' never consults table 'pod'/),
      },
    ]);
    expect(
      run('nat.cnat-valid', {
        cnat: {
          snat: {
            policy: 'k8s',
            interfaces: [
              { interface: 'Gig0/0/0', table: 'pod' },
              { interface: 'Gig0/0/1', table: 'host' },
              { interface: 'Gig0/0/2', table: 'include-v4' },
            ],
          },
        },
      }),
    ).toEqual([
      {
        pointer: '/nat/cnat/snat/interfaces/2/table',
        message: expect.stringMatching(/policy 'k8s' never consults table 'include-v4'/),
      },
    ]);
    expect(
      run('nat.cnat-valid', {
        cnat: {
          snat: {
            policy: 'none',
            interfaces: [
              { interface: 'Gig0/0/0', table: 'pod' },
              { interface: 'Gig0/0/0', table: 'include-v6' },
            ],
          },
        },
      }),
    ).toEqual([]);
  });
});

describe('nat.prefixes-are-networks', () => {
  it('rejects host bits in every network-prefix field and names the network (M4)', () => {
    expect(
      run('nat.prefixes-are-networks', {
        nat64: { prefixes: [{ prefix: '64:ff9b::1/96' }, { prefix: '64:ff9b::/96', vrf: 'cust' }] },
        nptv6: {
          bindings: [
            { interface: 'Gig0/0/0', internal: 'fd00::1/48', external: '2001:db8:0:1::/48' },
          ],
        },
        det44: { mappings: [{ inside: '100.64.0.1/16', outside: '203.0.113.0/28' }] },
        map: {
          domains: [
            {
              name: 'd',
              mode: 'map-t',
              ipv4Prefix: '192.0.2.1/24',
              ipv6Prefix: '2001:db8::1/40',
              ipv6Source: '64:ff9b::1/96',
            },
          ],
        },
        cnat: { snat: { excludePrefixes: ['10.0.0.1/8', '10.0.0.0/8'] } },
      }),
    ).toEqual([
      {
        pointer: '/nat/cnat/snat/excludePrefixes/0',
        message: "'10.0.0.1/8' has host bits set; the network is 10.0.0.0/8",
      },
      {
        pointer: '/nat/det44/mappings/0/inside',
        message: "'100.64.0.1/16' has host bits set; the network is 100.64.0.0/16",
      },
      {
        pointer: '/nat/map/domains/0/ipv4Prefix',
        message: "'192.0.2.1/24' has host bits set; the network is 192.0.2.0/24",
      },
      {
        pointer: '/nat/map/domains/0/ipv6Prefix',
        message: "'2001:db8::1/40' has host bits set; the network is 2001:db8::/40",
      },
      {
        pointer: '/nat/map/domains/0/ipv6Source',
        message: "'64:ff9b::1/96' has host bits set; the network is 64:ff9b::/96",
      },
      {
        pointer: '/nat/nat64/prefixes/0/prefix',
        message: "'64:ff9b::1/96' has host bits set; the network is 64:ff9b::/96",
      },
      {
        pointer: '/nat/nptv6/bindings/0/external',
        message: "'2001:db8:0:1::/48' has host bits set; the network is 2001:db8::/48",
      },
      {
        pointer: '/nat/nptv6/bindings/0/internal',
        message: "'fd00::1/48' has host bits set; the network is fd00::/48",
      },
    ]);
  });

  it('accepts real networks and single-address prefixes (/32, /128)', () => {
    expect(
      run('nat.prefixes-are-networks', {
        nat64: { prefixes: [{ prefix: '64:ff9b::/96' }] },
        nptv6: {
          bindings: [{ interface: 'Gig0/0/0', internal: 'fd00::/48', external: '2001:db8::/48' }],
        },
        det44: { mappings: [{ inside: '100.64.0.0/16', outside: '203.0.113.0/28' }] },
        map: {
          domains: [
            {
              name: 'd',
              mode: 'map-e',
              ipv4Prefix: '192.0.2.0/24',
              ipv6Prefix: '2001:db8::/40',
              ipv6Source: '2001:db8:ffff::1/128',
            },
          ],
        },
        cnat: { snat: { excludePrefixes: ['10.0.0.0/8', '192.0.2.1/32', '2001:db8::1/128'] } },
      }),
    ).toEqual([]);
    expect(run('nat.prefixes-are-networks', {})).toEqual([]);
  });
});

describe('nat.dslite-valid', () => {
  it('rejects AFTR+B4 together and enabled without either', () => {
    expect(
      run('nat.dslite-valid', {
        dslite: { aftr: { ipv6: '2001:db8::1' }, b4: { ipv6: '2001:db8::2' } },
      }),
    ).toEqual([
      {
        pointer: '/nat/dslite/b4',
        message: 'configure either the AFTR side or the B4 side, not both',
      },
    ]);
    expect(run('nat.dslite-valid', { dslite: { enabled: true } })).toEqual([
      { pointer: '/nat/dslite/enabled', message: 'DS-Lite requires aftr or b4' },
    ]);
    expect(
      run('nat.dslite-valid', { dslite: { enabled: true, aftr: { ipv6: '2001:db8::1' } } }),
    ).toEqual([]);
    expect(run('nat.dslite-valid', {})).toEqual([]);
  });
});
