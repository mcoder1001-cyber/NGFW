import { describe, expect, it } from 'vitest';
import { RootConfig } from '../index.js';
import { validateSemantics } from './index.js';
import { keaDhcpRelayValidators } from './kea-dhcp-relay.js';
import { sortIssues } from './registry.js';
import { BASE, run } from './vpn.fixtures.js';

const IF1 = 'TenGigabitEthernet0/0/1';
const IF2 = 'TenGigabitEthernet0/0/2';

const subnet = (extra: Record<string, unknown> = {}) => ({
  subnet: '192.168.10.0/24',
  pools: [{ start: '192.168.10.100', end: '192.168.10.199' }],
  ...extra,
});
const server = (extra: Record<string, unknown> = {}) => ({
  interfaces: [IF1],
  subnets: { lan: subnet() },
  ...extra,
});
const relay = (extra: Record<string, unknown> = {}) => ({
  vrf: 'customer-a',
  interfaces: [IF2],
  servers: ['192.168.10.1'],
  sourceAddress: '10.20.0.1',
  ...extra,
});
const doc = (dhcp: Record<string, unknown>, extra: Record<string, unknown> = {}) => ({
  ...BASE,
  ...extra,
  services: { dhcp },
});

/** Only this feature's rules. */
function mine(d: unknown) {
  const config = RootConfig.parse(d);
  return sortIssues(keaDhcpRelayValidators.flatMap((v) => v.validate(config)));
}

/** Schema errors (refinements in domains/services.ts) as pointer → message. */
function schemaIssues(d: unknown) {
  const r = RootConfig.safeParse(d);
  return r.success ? [] : r.error.issues.map((i) => ({ path: i.path.join('/'), message: i.message }));
}

describe('F-kea-dhcp-relay rules', () => {
  it('names are prefixed with their root key and unique', () => {
    const names = keaDhcpRelayValidators.map((v) => v.name);
    expect(new Set(names).size).toBe(names.length);
    for (const v of keaDhcpRelayValidators) {
      expect(v.name).toMatch(/^(services|interfaces)\.kea-dhcp-relay-/);
      expect(v.domains).toContain(v.name.split('.')[0]);
    }
  });

  it('a valid LAN server + relay produces no finding (also through the process-wide registry)', () => {
    const d = doc({
      servers: {
        lan: server({
          subnets: {
            lan: subnet({ reservations: { printer: { mac: '02:00:00:00:00:01', ip: '192.168.10.20' } } }),
          },
        }),
      },
      relays: { r: relay() },
    });
    expect(mine(d)).toEqual([]);
    expect(run(d)).toEqual([]);
    expect(validateSemantics(RootConfig.parse(d), ['services'])).toEqual([]);
  });

  it('services.kea-dhcp-relay-reservation-outside-pools', () => {
    const d = doc({
      servers: {
        lan: server({
          subnets: {
            lan: subnet({
              pools: [
                { start: '192.168.10.50', end: '192.168.10.59' },
                { start: '192.168.10.100', end: '192.168.10.199' },
              ],
              reservations: {
                inside: { mac: '02:00:00:00:00:01', ip: '192.168.10.150' },
                edge: { mac: '02:00:00:00:00:02', ip: '192.168.10.59' },
                outside: { mac: '02:00:00:00:00:03', ip: '192.168.10.20' },
              },
            }),
          },
        }),
      },
    });
    expect(mine(d)).toEqual([
      {
        pointer: '/services/dhcp/servers/lan/subnets/lan/reservations/edge/ip',
        message: '192.168.10.59 lies inside pool 0 (192.168.10.50-192.168.10.59); reserve an address outside the pools',
      },
      {
        pointer: '/services/dhcp/servers/lan/subnets/lan/reservations/inside/ip',
        message:
          '192.168.10.150 lies inside pool 1 (192.168.10.100-192.168.10.199); reserve an address outside the pools',
      },
    ]);
  });

  it('services.kea-dhcp-relay-one-vrf-per-family (disabled servers and the other family do not count)', () => {
    const d = doc({
      servers: {
        a: server(),
        b: server({ vrf: 'customer-a', interfaces: [IF2], subnets: { s: subnet({ subnet: '10.20.0.0/24', pools: [{ start: '10.20.0.10', end: '10.20.0.20' }] }) } }),
        c: server({ enabled: false, vrf: 'customer-a', interfaces: [IF2], subnets: {} }),
        v6: server({
          family: 'ipv6',
          vrf: 'customer-a',
          interfaces: [IF2],
          subnets: {},
        }),
      },
    });
    expect(mine(d)).toEqual([
      {
        pointer: '/services/dhcp/servers/b/vrf',
        message:
          "one Kea ipv4 instance serves every enabled ipv4 server: VRF 'customer-a' differs from VRF 'default' of server 'a'",
      },
    ]);
  });

  it('services.kea-dhcp-relay-relay-source-per-vrf', () => {
    const d = doc(
      {
        relays: {
          a: relay(),
          b: relay({ sourceAddress: '10.20.0.2', servers: ['192.168.10.1', '192.168.10.2'] }),
          off: relay({ enabled: false, sourceAddress: '10.20.0.3' }),
        },
      },
      {
        interfaces: {
          ...BASE.interfaces,
          [IF2]: { ipv4: ['10.20.0.1/24', '10.20.0.2/24'], vrf: 'customer-a' },
        },
      },
    );
    expect(mine(d)).toEqual([
      {
        pointer: '/services/dhcp/relays/b/servers/0',
        message: "192.168.10.1 is already a server of relay 'a' for client VRF 'customer-a'",
      },
      {
        pointer: '/services/dhcp/relays/b/sourceAddress',
        message:
          "VPP keeps one relay source address per client VRF and family: relay 'a' already uses 10.20.0.1 in VRF 'customer-a'",
      },
    ]);
  });

  it('interfaces.kea-dhcp-relay-dhcp-client-no-static (interfaces and sub-interfaces)', () => {
    const d = {
      ...BASE,
      interfaces: {
        ...BASE.interfaces,
        [IF1]: {
          ipv4: ['192.168.10.1/24'],
          dhcpClient: {},
          subinterfaces: { '100': { vlanId: 100, ipv4: ['10.1.0.1/24'], dhcpClient: { hostname: 'r1' } } },
        },
        'TenGigabitEthernet0/0/3': { dhcpClient: {} },
      },
    };
    expect(mine(d)).toEqual([
      {
        pointer: '/interfaces/TenGigabitEthernet0~10~11/ipv4',
        message:
          'an interface with a DHCP client obtains its IPv4 address by DHCP: remove the static IPv4 addresses or the DHCP client',
      },
      {
        pointer: '/interfaces/TenGigabitEthernet0~10~11/subinterfaces/100/ipv4',
        message:
          'an interface with a DHCP client obtains its IPv4 address by DHCP: remove the static IPv4 addresses or the DHCP client',
      },
    ]);
  });
});

// The rules below already exist (P02c: DhcpSubnetSchema refinements and semantic/services.ts); F-kea-dhcp-relay
// depends on them, so they are exercised here with the shapes the DHCP screen submits.
describe('existing DHCP rules this feature relies on', () => {
  it('a pool outside its subnet is a schema error at the pool (400 problem+json pointer)', () => {
    const d = doc({
      servers: { lan: server({ subnets: { lan: subnet({ pools: [{ start: '192.168.11.10', end: '192.168.11.20' }] }) } }) },
    });
    expect(schemaIssues(d)).toEqual([
      { path: 'services/dhcp/servers/lan/subnets/lan/pools/0/start', message: '192.168.11.10 is outside 192.168.10.0/24' },
      { path: 'services/dhcp/servers/lan/subnets/lan/pools/0/end', message: '192.168.11.20 is outside 192.168.10.0/24' },
    ]);
  });

  it('pools: same family, ordered, non-overlapping', () => {
    const d = doc({
      servers: {
        lan: server({
          subnets: {
            lan: subnet({
              pools: [
                { start: '192.168.10.100', end: '192.168.10.150' },
                { start: '192.168.10.140', end: '192.168.10.160' },
                { start: '192.168.10.90', end: '192.168.10.80' },
                { start: '2001:db8::1', end: '2001:db8::2' },
              ],
            }),
          },
        }),
      },
    });
    const paths = schemaIssues(d).map((i) => `${i.path}: ${i.message}`);
    expect(paths).toContain('services/dhcp/servers/lan/subnets/lan/pools/1: pool overlaps pool 0');
    expect(paths).toContain('services/dhcp/servers/lan/subnets/lan/pools/2/end: pool end is lower than pool start');
    expect(paths).toContain('services/dhcp/servers/lan/subnets/lan/pools/3/start: pool family does not match subnet 192.168.10.0/24');
  });

  it('reservations: inside the subnet, one address once, one MAC once', () => {
    const d = doc({
      servers: {
        lan: server({
          subnets: {
            lan: subnet({
              reservations: {
                a: { mac: '02:00:00:00:00:01', ip: '192.168.10.20' },
                b: { mac: '02:00:00:00:00:01', ip: '192.168.10.20' },
                c: { mac: '02:00:00:00:00:03', ip: '192.168.99.1' },
              },
            }),
          },
        }),
      },
    });
    const paths = schemaIssues(d).map((i) => `${i.path}: ${i.message}`);
    expect(paths).toContain('services/dhcp/servers/lan/subnets/lan/reservations/b/ip: 192.168.10.20 is reserved twice');
    expect(paths).toContain('services/dhcp/servers/lan/subnets/lan/reservations/b: client 02:00:00:00:00:01 is reserved twice');
    expect(paths).toContain('services/dhcp/servers/lan/subnets/lan/reservations/c/ip: 192.168.99.1 is outside 192.168.10.0/24');
  });

  it('subnets: unique per VRF across servers, inside an interface prefix; relay source configured in the server VRF', () => {
    const d = doc({
      servers: { a: server(), b: server(), c: server({ subnets: { far: subnet({ subnet: '172.16.0.0/24', pools: [{ start: '172.16.0.10', end: '172.16.0.20' }] }) } }) },
      relays: { r: relay({ sourceAddress: '10.99.0.1' }) },
    });
    expect(run(d).map((i) => i.pointer)).toEqual([
      '/services/dhcp/relays/r/sourceAddress',
      '/services/dhcp/servers/b/subnets/lan/subnet',
      '/services/dhcp/servers/c/subnets/far/subnet',
    ]);
  });
});
