import { describe, expect, it } from 'vitest';
import { tunnelsValidators } from './tunnels.js';
import { BASE, example, run } from './vpn.fixtures.js';

const gre = (extra: Record<string, unknown> = {}) => ({
  src: '198.51.100.2',
  dst: '203.0.113.30',
  ...extra,
});
const withTunnels = (tunnels: Record<string, unknown>) => ({ ...BASE, tunnels });

describe('tunnels examples', () => {
  it('tunnels-gre-vxlan-ipip.json is semantically valid', () => {
    expect(run(example('tunnels-gre-vxlan-ipip.json'))).toEqual([]);
  });
  it('tunnels-semantic-source-not-configured.json triggers tunnels.source-address-configured only', () => {
    expect(run(example('tunnels-semantic-source-not-configured.json'))).toEqual([
      {
        pointer: '/tunnels/gre/gre-x/src',
        message: "198.51.100.9 is not configured on any interface in VRF 'default'",
      },
    ]);
  });
  it('validator names are prefixed and unique', () => {
    for (const v of tunnelsValidators) {
      expect(v.name.startsWith('tunnels.')).toBe(true);
      expect(v.domains).toContain('tunnels');
    }
    expect(new Set(tunnelsValidators.map((v) => v.name)).size).toBe(tunnelsValidators.length);
  });
});

describe('tunnels.name-unique', () => {
  it('a name may be used by one kind only', () => {
    const doc = withTunnels({
      gre: { a: gre() },
      vxlan: { a: { src: '192.168.10.1', dst: '192.168.10.9', vni: 1 } },
      ipip: { a: gre() },
    });
    expect(run(doc, 'tunnels.name-unique')).toEqual([
      { pointer: '/tunnels/ipip/a', message: "tunnel name 'a' is already used by tunnels.gre" },
      { pointer: '/tunnels/vxlan/a', message: "tunnel name 'a' is already used by tunnels.gre" },
    ]);
  });
});

describe('tunnels.vrf-exists / tunnels.source-address-configured', () => {
  it('both VRFs must exist', () => {
    expect(
      run(withTunnels({ gre: { a: gre({ vrf: 'v1', underlayVrf: 'v2' }) } }), 'tunnels.vrf-exists'),
    ).toEqual([
      { pointer: '/tunnels/gre/a/underlayVrf', message: "VRF 'v2' does not exist" },
      { pointer: '/tunnels/gre/a/vrf', message: "VRF 'v1' does not exist" },
    ]);
  });
  it('the source must be an address of the underlay VRF; instanced tunnel addresses count', () => {
    const doc = withTunnels({
      gre: {
        ok: gre(),
        ok6: gre({ src: '2001:db8:0:1::2', dst: '2001:db8:ffff::1' }),
        wrongVrf: gre({ underlayVrf: 'customer-a' }),
        viaSub: gre({
          src: '10.20.0.1',
          underlayVrf: 'customer-a',
          instance: 7,
          ipv4: ['10.254.7.1/30'],
        }),
        overGre: gre({ src: '10.254.7.1', dst: '10.254.7.2' }),
      },
    });
    expect(run(doc, 'tunnels.source-address-configured')).toEqual([
      {
        pointer: '/tunnels/gre/wrongVrf/src',
        message: "198.51.100.2 is not configured on any interface in VRF 'customer-a'",
      },
    ]);
  });
});

describe('tunnels.endpoints-unique / tunnels.instance-unique', () => {
  it('same endpoints in the same underlay clash; VXLAN adds the VNI', () => {
    const doc = withTunnels({
      gre: { a: gre(), b: gre(), c: gre({ underlayVrf: 'customer-a', src: '10.20.0.1' }) },
      vxlan: {
        v1: { src: '192.168.10.1', dst: '192.168.10.9', vni: 1 },
        v2: { src: '192.168.10.1', dst: '192.168.10.9', vni: 2 },
        v3: { src: '192.168.10.1', dst: '192.168.10.9', vni: 1 },
      },
    });
    expect(run(doc, 'tunnels.endpoints-unique')).toEqual([
      { pointer: '/tunnels/gre/b/dst', message: "same endpoints as tunnel 'a'" },
      { pointer: '/tunnels/vxlan/v3/dst', message: "same endpoints and VNI as tunnel 'v1'" },
    ]);
  });
  it('instances are unique per kind', () => {
    const doc = withTunnels({
      gre: { a: gre({ instance: 0 }), b: gre({ dst: '203.0.113.31', instance: 0 }) },
      ipip: { c: gre({ instance: 0 }) },
    });
    expect(run(doc, 'tunnels.instance-unique')).toEqual([
      { pointer: '/tunnels/gre/b/instance', message: "instance 0 is already used by tunnel 'a'" },
    ]);
  });
});

describe('tunnels.mcast-interface-exists / tunnels.address-overlap', () => {
  it('the multicast interface must exist', () => {
    const doc = withTunnels({
      vxlan: {
        ok: {
          src: '192.168.10.1',
          dst: '239.1.1.1',
          vni: 1,
          mcastInterface: 'TenGigabitEthernet0/0/1',
        },
        bad: { src: '192.168.10.1', dst: '239.1.1.2', vni: 2, mcastInterface: 'loop9' },
      },
    });
    expect(run(doc, 'tunnels.mcast-interface-exists')).toEqual([
      { pointer: '/tunnels/vxlan/bad/mcastInterface', message: "interface 'loop9' does not exist" },
    ]);
  });
  it('tunnel interface addresses must not overlap within a VRF', () => {
    const doc = withTunnels({
      gre: {
        a: gre({ ipv4: ['10.254.0.1/30'], ipv6: ['fd00::1/64'] }),
        b: gre({ dst: '203.0.113.31', ipv4: ['10.254.0.2/30'] }),
        c: gre({ dst: '203.0.113.32', ipv4: ['10.254.0.1/30'], vrf: 'customer-a' }),
        d: gre({ dst: '203.0.113.33', ipv6: ['fd00::2/64'] }),
      },
    });
    expect(run(doc, 'tunnels.address-overlap')).toEqual([
      {
        pointer: '/tunnels/gre/b/ipv4/0',
        message: "10.254.0.2/30 overlaps an address of tunnel 'a' in VRF 'default'",
      },
      {
        pointer: '/tunnels/gre/d/ipv6/0',
        message: "fd00::2/64 overlaps an address of tunnel 'a' in VRF 'default'",
      },
    ]);
  });
});

describe('review F10 / F11', () => {
  it('tunnel addresses must not overlap physical, sub-interface or WireGuard prefixes of the VRF (F10)', () => {
    const doc = {
      ...withTunnels({
        gre: {
          dc: gre({ ipv4: ['192.168.10.77/24'] }),
          wgClash: gre({ dst: '203.0.113.31', ipv4: ['10.200.0.2/24'] }),
          otherVrf: gre({ dst: '203.0.113.32', ipv4: ['192.168.10.78/24'], vrf: 'customer-a' }),
          inst: gre({ dst: '203.0.113.33', instance: 7, ipv4: ['10.254.7.1/30'] }),
        },
      }),
      vpn: {
        wireguard: {
          interfaces: {
            w: {
              instance: 0,
              listenAddress: '198.51.100.2',
              privateKeyRef: 'key/w',
              address: ['10.200.0.1/24'],
            },
          },
        },
      },
    };
    expect(run(doc, 'tunnels.address-overlap')).toEqual([
      {
        pointer: '/tunnels/gre/dc/ipv4/0',
        message:
          "192.168.10.77/24 overlaps an address of interface 'TenGigabitEthernet0/0/1' in VRF 'default'",
      },
      {
        pointer: '/tunnels/gre/wgClash/ipv4/0',
        message: "10.200.0.2/24 overlaps an address of interface 'wg0' in VRF 'default'",
      },
    ]);
  });
  it('GRE endpoints are keyed by type and ERSPAN session (F11)', () => {
    const erspan = (sessionId: number, extra: Record<string, unknown> = {}) =>
      gre({ type: 'erspan', sessionId, ...extra });
    const doc = withTunnels({
      gre: {
        mirror: erspan(1),
        mirror2: erspan(2),
        l3: gre(),
        teb: gre({ type: 'teb' }),
        dup: erspan(1),
      },
    });
    expect(run(doc, 'tunnels.endpoints-unique')).toEqual([
      {
        pointer: '/tunnels/gre/dup/dst',
        message: "same endpoints and ERSPAN session as tunnel 'mirror'",
      },
    ]);
  });
});
