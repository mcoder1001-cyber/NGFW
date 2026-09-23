import { describe, expect, it } from 'vitest';
import { servicesValidators } from './services.js';
import { BASE, example, run } from './vpn.fixtures.js';

const IF0 = 'TenGigabitEthernet0/0/0';
const IF1 = 'TenGigabitEthernet0/0/1';
const IF2 = 'TenGigabitEthernet0/0/2';
const withServices = (services: Record<string, unknown>) => ({ ...BASE, services });
const server = (extra: Record<string, unknown> = {}) => ({
  interfaces: [IF1],
  subnets: {
    s: { subnet: '192.168.10.0/24', pools: [{ start: '192.168.10.100', end: '192.168.10.199' }] },
  },
  ...extra,
});

describe('services examples', () => {
  it.each(['services-dhcp-dns.json', 'services-snmp-lldp-ipfix-ntp.json'])(
    '%s is semantically valid',
    (f) => {
      expect(run(example(f))).toEqual([]);
    },
  );
  it('services-semantic-dhcp-subnet-outside-interface.json triggers the subnet rule only', () => {
    expect(run(example('services-semantic-dhcp-subnet-outside-interface.json'))).toEqual([
      {
        pointer: '/services/dhcp/servers/lan/subnets/wrong/subnet',
        message: '192.168.99.0/24 is not within a prefix configured on TenGigabitEthernet0/0/1',
      },
    ]);
  });
  it('validator names are prefixed and unique', () => {
    for (const v of servicesValidators) {
      expect(v.name.startsWith('services.')).toBe(true);
      expect(v.domains).toContain('services');
    }
    expect(new Set(servicesValidators.map((v) => v.name)).size).toBe(servicesValidators.length);
  });
});

describe('services.vrf-exists', () => {
  it('every instance VRF must exist', () => {
    const doc = withServices({
      dhcp: {
        servers: { a: server({ vrf: 'v1' }) },
        relays: {
          r: {
            vrf: 'v2',
            serverVrf: 'v3',
            interfaces: [IF2],
            servers: ['10.0.0.5'],
            sourceAddress: '10.20.0.1',
          },
        },
      },
      dns: { resolvers: { d: { vrf: 'v4', listen: [{ address: '0.0.0.0' }] } } },
      snmp: { vrf: 'v5' },
      ipfix: {
        exporters: {
          e: { vrf: 'v6', collector: { address: '10.0.0.9' }, sourceAddress: '192.168.10.1' },
        },
        sflow: { vrf: 'v7', collectors: [{ address: '10.0.0.9' }] },
      },
      ntp: { vrf: 'v8' },
    });
    expect(run(doc, 'services.vrf-exists').map((i) => i.pointer)).toEqual([
      '/services/dhcp/relays/r/serverVrf',
      '/services/dhcp/relays/r/vrf',
      '/services/dhcp/servers/a/vrf',
      '/services/dns/resolvers/d/vrf',
      '/services/ipfix/exporters/e/vrf',
      '/services/ipfix/sflow/vrf',
      '/services/ntp/vrf',
      '/services/snmp/vrf',
    ]);
  });
});

describe('services.interface-references', () => {
  it('interfaces must exist and (for DHCP) belong to the instance VRF', () => {
    const doc = withServices({
      dhcp: {
        servers: { a: server({ interfaces: [IF1, 'loop9', IF2] }) },
        relays: {
          r: {
            interfaces: ['loop8'],
            servers: ['192.168.10.5'],
            sourceAddress: '10.20.0.1',
            vrf: 'customer-a',
          },
        },
      },
      lldp: { interfaces: [{ interface: IF0 }, { interface: 'loop7' }] },
      ipfix: {
        flowprobe: { interfaces: [{ interface: 'loop6' }] },
        sflow: { collectors: [{ address: '10.0.0.9' }], interfaces: ['loop5', IF0] },
      },
    });
    expect(run(doc, 'services.interface-references')).toEqual([
      {
        pointer: '/services/dhcp/relays/r/interfaces/0',
        message: "interface 'loop8' does not exist",
      },
      {
        pointer: '/services/dhcp/servers/a/interfaces/1',
        message: "interface 'loop9' does not exist",
      },
      {
        pointer: '/services/dhcp/servers/a/interfaces/2',
        message:
          "interface 'TenGigabitEthernet0/0/2' is in VRF 'customer-a', not in the instance VRF 'default'",
      },
      {
        pointer: '/services/ipfix/flowprobe/interfaces/0/interface',
        message: "interface 'loop6' does not exist",
      },
      {
        pointer: '/services/ipfix/sflow/interfaces/0',
        message: "interface 'loop5' does not exist",
      },
      {
        pointer: '/services/lldp/interfaces/1/interface',
        message: "interface 'loop7' does not exist",
      },
    ]);
  });
});

describe('services.dhcp-subnet-within-interface-prefix / services.dhcp-subnets-unique', () => {
  it('subnets lie within a prefix of one of the server interfaces; unknown interfaces are skipped', () => {
    const doc = withServices({
      dhcp: {
        servers: {
          ok: server(),
          narrower: server({
            interfaces: [IF0, IF1],
            subnets: {
              s: {
                subnet: '192.168.10.128/25',
                pools: [{ start: '192.168.10.130', end: '192.168.10.140' }],
              },
            },
          }),
          wrongIf: server({
            interfaces: [IF0],
            subnets: {
              s: {
                subnet: '192.168.10.0/24',
                pools: [{ start: '192.168.10.100', end: '192.168.10.110' }],
              },
            },
          }),
          v6: server({
            family: 'ipv6',
            subnets: {
              s: {
                subnet: '2001:db8:10::/64',
                pools: [{ start: '2001:db8:10::100', end: '2001:db8:10::1ff' }],
              },
            },
          }),
          unknownIf: server({ interfaces: ['loop9'] }),
        },
      },
    });
    expect(run(doc, 'services.dhcp-subnet-within-interface-prefix')).toEqual([
      {
        pointer: '/services/dhcp/servers/wrongIf/subnets/s/subnet',
        message: '192.168.10.0/24 is not within a prefix configured on TenGigabitEthernet0/0/0',
      },
    ]);
  });
  it('subnets of different servers in the same VRF must not overlap', () => {
    const doc = withServices({
      dhcp: {
        servers: {
          a: server(),
          b: server({
            subnets: {
              s: {
                subnet: '192.168.10.128/25',
                pools: [{ start: '192.168.10.130', end: '192.168.10.140' }],
              },
            },
          }),
          c: server({
            vrf: 'customer-a',
            interfaces: [IF2],
            subnets: {
              s: {
                subnet: '192.168.10.0/24',
                pools: [{ start: '192.168.10.1', end: '192.168.10.2' }],
              },
            },
          }),
        },
      },
    });
    expect(run(doc, 'services.dhcp-subnets-unique')).toEqual([
      {
        pointer: '/services/dhcp/servers/b/subnets/s/subnet',
        message: "192.168.10.128/25 overlaps a subnet of DHCP server 'a' in VRF 'default'",
      },
    ]);
  });
});

describe('services.bind-address-configured', () => {
  it('relay source, DNS/SNMP/NTP bind addresses, IPFIX source and sFlow agent must be configured in the VRF; wildcards pass', () => {
    const doc = withServices({
      dhcp: {
        relays: {
          r: {
            vrf: 'customer-a',
            interfaces: [IF2],
            servers: ['192.168.10.5'],
            sourceAddress: '10.20.0.9',
          },
        },
      },
      dns: {
        resolvers: {
          d: {
            listen: [
              { address: '0.0.0.0' },
              { address: '::' },
              { address: '192.168.10.1' },
              { address: '10.20.0.1' },
            ],
          },
        },
      },
      snmp: { listen: [{ address: '192.168.10.1' }, { address: '192.168.10.2' }] },
      ipfix: {
        exporters: { e: { collector: { address: '10.0.0.9' }, sourceAddress: '192.168.10.7' } },
        sflow: { collectors: [{ address: '10.0.0.9' }], agentAddress: '192.168.10.8' },
      },
      ntp: { listen: ['192.168.10.1', '2001:db8:10::9'] },
    });
    expect(run(doc, 'services.bind-address-configured').map((i) => i.pointer)).toEqual([
      '/services/dhcp/relays/r/sourceAddress',
      '/services/dns/resolvers/d/listen/3/address',
      '/services/ipfix/exporters/e/sourceAddress',
      '/services/ipfix/sflow/agentAddress',
      '/services/ntp/listen/1',
      '/services/snmp/listen/1/address',
    ]);
  });
});

describe('services.dns-listen-unique', () => {
  it('a socket (and a wildcard covering it) is served by one resolver per VRF', () => {
    const doc = withServices({
      dns: {
        resolvers: {
          a: { listen: [{ address: '192.168.10.1' }] },
          b: { listen: [{ address: '192.168.10.1' }] },
          c: { listen: [{ address: '0.0.0.0' }] },
          d: { listen: [{ address: '192.168.10.1', port: 5353 }] },
          e: { vrf: 'customer-a', listen: [{ address: '10.20.0.1' }, { address: '0.0.0.0' }] },
          f: { listen: [{ address: '::' }] },
        },
      },
    });
    expect(run(doc, 'services.dns-listen-unique')).toEqual([
      {
        pointer: '/services/dns/resolvers/b/listen/0',
        message: "192.168.10.1:53 in VRF 'default' is already served by resolver 'a'",
      },
      {
        pointer: '/services/dns/resolvers/c/listen/0',
        message: "0.0.0.0:53 in VRF 'default' is already served by resolver 'a'",
      },
    ]);
  });
});

describe('services.bind-address-configured — DHCP relay source VRF (review F13)', () => {
  const relay = (extra: Record<string, unknown>) =>
    withServices({
      dhcp: {
        relays: {
          r: { vrf: 'customer-a', interfaces: [IF2], servers: ['198.51.100.9'], ...extra },
        },
      },
    });
  it('the source address lives in serverVrf when set (VPP dhcp_proxy_config server_vrf_id)', () => {
    expect(
      run(
        relay({ serverVrf: 'default', sourceAddress: '198.51.100.2' }),
        'services.bind-address-configured',
      ),
    ).toEqual([]);
    expect(
      run(
        relay({ serverVrf: 'default', sourceAddress: '10.20.0.1' }),
        'services.bind-address-configured',
      ),
    ).toEqual([
      {
        pointer: '/services/dhcp/relays/r/sourceAddress',
        message: "10.20.0.1 is not configured on any interface in VRF 'default'",
      },
    ]);
  });
  it('falls back to the client VRF without serverVrf', () => {
    expect(run(relay({ sourceAddress: '10.20.0.1' }), 'services.bind-address-configured')).toEqual(
      [],
    );
  });
});

describe('services.qos-references / services.qos-consistency (D-052)', () => {
  const qos = (extra: Record<string, unknown>) =>
    withServices({
      qos: {
        policers: { p: { cir: 1000, cb: 1500 } },
        shapers: { s: { rateKbps: 100000 } },
        maps: {
          m: { id: 1, rows: { ip: [{ from: 46, to: 46 }], ext: [{ from: 1, to: 63 }] } },
          n: { id: 1, rows: { vlan: [{ from: 1, to: 2 }] } },
        },
        ...extra,
      },
    });
  it('attachments reference existing interfaces, policers, shapers and maps', () => {
    const doc = qos({
      interfaces: {
        [IF0]: { policer: { input: 'p', output: 'gone' } },
        [IF1]: { shaper: 'nope', mark: { map: 'missing', output: 'ip' } },
        loop9: { policer: { input: 'p' } },
      },
    });
    expect(run(doc, 'services.qos-references')).toEqual(
      [
        {
          pointer: '/services/qos/interfaces/TenGigabitEthernet0~10~11/mark/map',
          message: "map 'missing' does not exist in qos.maps",
        },
        {
          pointer: '/services/qos/interfaces/TenGigabitEthernet0~10~11/shaper',
          message: "shaper 'nope' does not exist in qos.shapers",
        },
        {
          pointer: '/services/qos/interfaces/TenGigabitEthernet0~10~10/policer/output',
          message: "policer 'gone' does not exist in qos.policers",
        },
        { pointer: '/services/qos/interfaces/loop9', message: "interface 'loop9' does not exist" },
      ].sort((a, b) => a.pointer.localeCompare(b.pointer)),
    );
  });
  it('map ids are unique and marked values fit the output header', () => {
    const doc = qos({
      interfaces: {
        [IF0]: { mark: { map: 'm', output: 'vlan' } },
        [IF1]: { mark: { map: 'm', output: 'ip' } },
        [IF2]: { mark: { map: 'gone', output: 'ip' } },
      },
    });
    expect(run(doc, 'services.qos-consistency')).toEqual([
      {
        pointer: '/services/qos/maps/m/rows/ext/0/to',
        message: expect.stringContaining('vlan header (0–7)'),
      },
      {
        pointer: '/services/qos/maps/m/rows/ip/0/to',
        message: expect.stringContaining('vlan header (0–7)'),
      },
      { pointer: '/services/qos/maps/n/id', message: "map id 1 is already used by map 'm'" },
    ]);
  });
});
