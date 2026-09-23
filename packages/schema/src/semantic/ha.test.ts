import { describe, expect, it } from 'vitest';
import { haValidators } from './ha.js';
import { BASE, example, run } from './vpn.fixtures.js';

const IF0 = 'TenGigabitEthernet0/0/0';
const IF1 = 'TenGigabitEthernet0/0/1';
const IF2 = 'TenGigabitEthernet0/0/2';
const vr = (extra: Record<string, unknown> = {}) => ({
  interface: IF1,
  vrId: 10,
  addresses: ['192.168.10.254'],
  ...extra,
});
/** `ha.vrrp` is a record keyed by name (D-053): `vrs(a, b)` → `{ vr0: a, vr1: b }`. */
const vrs = (...items: Record<string, unknown>[]) =>
  Object.fromEntries(items.map((v, i) => [`vr${i}`, v]));
const withHa = (ha: Record<string, unknown>) => ({ ...BASE, ha });
const cluster = (extra: Record<string, unknown> = {}) => ({
  nodeName: 'vrx-a',
  peers: [{ name: 'vrx-b', address: '192.168.10.3' }],
  secretRef: 'key/cluster',
  ...extra,
});

describe('ha examples', () => {
  it('ha-vrrp.json is semantically valid', () => {
    expect(run(example('ha-vrrp.json'))).toEqual([]);
  });
  it('ha-semantic-duplicate-vrid.json triggers ha.vrrp-vrid-unique only', () => {
    expect(run(example('ha-semantic-duplicate-vrid.json'))).toEqual([
      {
        pointer: '/ha/vrrp/lan-v4-b/vrId',
        message:
          "VRID 10 (ipv4) on TenGigabitEthernet0/0/1 is already used by virtual router 'lan-v4'",
      },
    ]);
  });
  it('validator names are prefixed and unique', () => {
    for (const v of haValidators) {
      expect(v.name.startsWith('ha.')).toBe(true);
      expect(v.domains).toContain('ha');
    }
    expect(new Set(haValidators.map((v) => v.name)).size).toBe(haValidators.length);
  });
});

describe('ha.vrrp-vrid-unique', () => {
  it('the same VRID is fine on another interface or for the other address family', () => {
    const doc = withHa({
      vrrp: vrs(
        vr(),
        vr({ addressFamily: 'ipv6', addresses: ['2001:db8:10::fe'] }),
        vr({ interface: IF2, vrf: 'customer-a', addresses: ['10.20.0.254'] }),
        vr({ addresses: ['192.168.10.253'] }),
      ),
    });
    expect(run(doc, 'ha.vrrp-vrid-unique')).toEqual([
      {
        pointer: '/ha/vrrp/vr3/vrId',
        message:
          "VRID 10 (ipv4) on TenGigabitEthernet0/0/1 is already used by virtual router 'vr0'",
      },
    ]);
  });
});

describe('ha.interface-exists', () => {
  it('interfaces exist and match the VRF; tracked and cluster interfaces exist', () => {
    const doc = withHa({
      vrrp: vrs(
        vr({ interface: 'loop9' }),
        vr({ interface: IF2, addresses: ['10.20.0.254'] }),
        vr({ track: [{ interface: 'loop8' }, { interface: IF0 }] }),
      ),
      cluster: cluster({ interface: 'loop7' }),
    });
    expect(run(doc, 'ha.interface-exists')).toEqual([
      { pointer: '/ha/cluster/interface', message: "interface 'loop7' does not exist" },
      { pointer: '/ha/vrrp/vr0/interface', message: "interface 'loop9' does not exist" },
      {
        pointer: '/ha/vrrp/vr1/vrf',
        message: "interface 'TenGigabitEthernet0/0/2' is in VRF 'customer-a', not 'default'",
      },
      { pointer: '/ha/vrrp/vr2/track/0/interface', message: "interface 'loop8' does not exist" },
    ]);
  });
  it('an existing cluster interface is fine', () => {
    expect(run(withHa({ cluster: cluster({ interface: IF0 }) }), 'ha.interface-exists')).toEqual(
      [],
    );
  });
});

describe('ha.vrrp-virtual-addresses', () => {
  it('virtual addresses are claimed once per VRF and lie within an interface prefix', () => {
    const doc = withHa({
      vrrp: vrs(
        vr(),
        vr({ vrId: 11, addresses: ['192.168.10.254', '192.168.11.1'] }),
        vr({ vrId: 12, addressFamily: 'ipv6', addresses: ['2001:db8:10::fe'] }),
        vr({ interface: IF2, vrf: 'customer-a', addresses: ['192.168.10.254'] }),
        vr({
          interface: IF2,
          vrf: 'customer-a',
          vrId: 2,
          addressFamily: 'ipv6',
          addresses: ['2001:db8:20::fe'],
        }),
        vr({ interface: 'loop9', vrId: 3, addresses: ['10.9.9.9'] }),
      ),
    });
    expect(run(doc, 'ha.vrrp-virtual-addresses')).toEqual([
      {
        pointer: '/ha/vrrp/vr1/addresses/0',
        message:
          "192.168.10.254 is already a virtual address of virtual router 'vr0' in VRF 'default'",
      },
      {
        pointer: '/ha/vrrp/vr1/addresses/1',
        message: '192.168.11.1 is not within a prefix configured on TenGigabitEthernet0/0/1',
      },
      {
        pointer: '/ha/vrrp/vr3/addresses/0',
        message: '192.168.10.254 is not within a prefix configured on TenGigabitEthernet0/0/2',
      },
    ]);
  });
});

describe('ha.vrf-exists', () => {
  it('VRFs of virtual routers and the cluster must exist', () => {
    const doc = withHa({ vrrp: vrs(vr({ vrf: 'nope' })), cluster: cluster({ vrf: 'gone' }) });
    expect(run(doc, 'ha.vrf-exists')).toEqual([
      { pointer: '/ha/cluster/vrf', message: "VRF 'gone' does not exist" },
      { pointer: '/ha/vrrp/vr0/vrf', message: "VRF 'nope' does not exist" },
    ]);
  });
});
