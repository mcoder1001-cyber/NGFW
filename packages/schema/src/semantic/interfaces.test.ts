import { describe, expect, it } from 'vitest';
import { RootConfig, type RootConfig as Root, type RootConfigInput } from '../index.js';
import { interfacesValidators } from './interfaces.js';

const run = (name: string, doc: RootConfigInput) =>
  interfacesValidators.find((v) => v.name === name)!.validate(RootConfig.parse(doc));

const TEN0 = '/interfaces/TenGigabitEthernet0~10~10';
const TEN1 = '/interfaces/TenGigabitEthernet0~10~11';

describe('interfaces.vrf-exists', () => {
  it('accepts the implicit default VRF and declared VRFs on interfaces and sub-interfaces', () => {
    expect(
      run('interfaces.vrf-exists', {
        vrfs: { a: { id: 1 } },
        interfaces: { loop0: { vrf: 'a', subinterfaces: {} }, loop1: { subinterfaces: { '10': { vlanId: 10, vrf: 'a' } } } },
      }),
    ).toEqual([]);
  });
  it('reports unknown VRFs on interfaces and sub-interfaces', () => {
    expect(
      run('interfaces.vrf-exists', {
        interfaces: { 'TenGigabitEthernet0/0/0': { vrf: 'x', subinterfaces: { '10': { vlanId: 10, vrf: 'y' } } } },
      }),
    ).toEqual([
      { pointer: `${TEN0}/vrf`, message: "VRF 'x' does not exist" },
      { pointer: `${TEN0}/subinterfaces/10/vrf`, message: "VRF 'y' does not exist" },
    ]);
  });
});

describe('interfaces.address-no-overlap', () => {
  it('accepts disjoint prefixes, the same prefix in different VRFs and several addresses of one subnet on one interface', () => {
    expect(
      run('interfaces.address-no-overlap', {
        vrfs: { a: { id: 1 } },
        interfaces: {
          'TenGigabitEthernet0/0/0': { ipv4: ['10.0.0.1/24', '10.0.0.2/24'], ipv6: ['2001:db8::1/64'] },
          'TenGigabitEthernet0/0/1': { ipv4: ['10.0.1.1/24'], ipv6: ['2001:db8:1::1/64'] },
          loop0: { ipv4: ['10.0.0.1/24'], vrf: 'a' },
        },
      }),
    ).toEqual([]);
  });
  it('reports overlapping prefixes in the same VRF, across parents and sub-interfaces, for both families', () => {
    expect(
      run('interfaces.address-no-overlap', {
        interfaces: {
          'TenGigabitEthernet0/0/0': { ipv4: ['10.0.0.1/24'], ipv6: ['2001:db8::1/48'] },
          'TenGigabitEthernet0/0/1': {
            ipv4: ['10.0.0.129/25'],
            subinterfaces: { '100': { vlanId: 100, ipv6: ['2001:db8:0:1::1/64'] } },
          },
        },
      }),
    ).toEqual([
      {
        pointer: `${TEN1}/ipv4/0`,
        message: `10.0.0.129/25 overlaps with 10.0.0.1/24 on TenGigabitEthernet0/0/0 (${TEN0}/ipv4/0) in VRF 'default'`,
      },
      {
        pointer: `${TEN1}/subinterfaces/100/ipv6/0`,
        message: `2001:db8:0:1::1/64 overlaps with 2001:db8::1/48 on TenGigabitEthernet0/0/0 (${TEN0}/ipv6/0) in VRF 'default'`,
      },
    ]);
  });
  it('reports the identical address twice, on one interface or on two', () => {
    expect(
      run('interfaces.address-no-overlap', {
        interfaces: { loop0: { ipv4: ['10.255.0.1/32', '10.255.0.1/32'] }, loop1: { ipv4: ['10.255.0.1/32'] } },
      }),
    ).toEqual([
      { pointer: '/interfaces/loop0/ipv4/1', message: 'address 10.255.0.1/32 is already assigned at /interfaces/loop0/ipv4/0' },
      { pointer: '/interfaces/loop1/ipv4/0', message: 'address 10.255.0.1/32 is already assigned at /interfaces/loop0/ipv4/0' },
    ]);
  });
  it('never crashes on an address that slipped past structural validation', () => {
    const broken = RootConfig.parse({ interfaces: { loop0: {} } }) as Root;
    broken.interfaces.loop0!.ipv4.push('not-a-prefix');
    expect(interfacesValidators.find((v) => v.name === 'interfaces.address-no-overlap')!.validate(broken)).toEqual([]);
  });
});

describe('interfaces.vlan-unique', () => {
  it('distinguishes single vs double tags, 802.1Q vs 802.1ad, and parents', () => {
    expect(
      run('interfaces.vlan-unique', {
        interfaces: {
          'TenGigabitEthernet0/0/0': {
            subinterfaces: {
              '100': { vlanId: 100 },
              '101': { vlanId: 100, innerVlanId: 10 },
              '102': { vlanId: 100, dot1ad: true },
            },
          },
          'TenGigabitEthernet0/0/1': { subinterfaces: { '100': { vlanId: 100 } } },
        },
      }),
    ).toEqual([]);
  });
  it('reports a repeated tag combination on the same parent', () => {
    expect(
      run('interfaces.vlan-unique', {
        interfaces: {
          'TenGigabitEthernet0/0/0': {
            subinterfaces: {
              '100': { vlanId: 100 },
              '200': { vlanId: 100 },
              '300': { vlanId: 300, innerVlanId: 30 },
              '301': { vlanId: 300, innerVlanId: 30 },
            },
          },
        },
      }),
    ).toEqual([
      { pointer: `${TEN0}/subinterfaces/200/vlanId`, message: 'VLAN dot1q 100 is already used by sub-interface TenGigabitEthernet0/0/0.100' },
      { pointer: `${TEN0}/subinterfaces/301/vlanId`, message: 'VLAN dot1q 300.30 is already used by sub-interface TenGigabitEthernet0/0/0.300' },
    ]);
  });
});

describe('interfaces.unnumbered-target-exists', () => {
  it('accepts parents and sub-interfaces as targets', () => {
    expect(
      run('interfaces.unnumbered-target-exists', {
        interfaces: {
          loop0: { ipv4: ['10.255.0.1/32'], subinterfaces: { '5': { vlanId: 5 } } },
          'TenGigabitEthernet0/0/0': { unnumbered: 'loop0', subinterfaces: { '100': { vlanId: 100, unnumbered: 'loop0.5' } } },
        },
      }),
    ).toEqual([]);
  });
  it('rejects self-references and unknown targets', () => {
    expect(
      run('interfaces.unnumbered-target-exists', {
        interfaces: {
          loop0: { unnumbered: 'loop0' },
          'TenGigabitEthernet0/0/0': { subinterfaces: { '100': { vlanId: 100, unnumbered: 'loop9' } } },
        },
      }),
    ).toEqual([
      { pointer: '/interfaces/loop0/unnumbered', message: 'an interface cannot be unnumbered to itself' },
      { pointer: `${TEN0}/subinterfaces/100/unnumbered`, message: "interface 'loop9' does not exist" },
    ]);
  });
});

describe('interfaces.mac-unique', () => {
  it('ignores interfaces without a MAC and accepts distinct MACs', () => {
    expect(run('interfaces.mac-unique', { interfaces: { loop0: {}, loop1: { mac: '02:00:00:00:00:01' }, loop2: { mac: '02:00:00:00:00:02' } } })).toEqual([]);
  });
  it('reports the same MAC written differently', () => {
    expect(run('interfaces.mac-unique', { interfaces: { loop1: { mac: '02:00:00:00:00:AA' }, loop2: { mac: '02-00-00-00-00-aa' } } })).toEqual([
      { pointer: '/interfaces/loop2/mac', message: 'MAC address 02:00:00:00:00:aa is already used by loop1' },
    ]);
  });
});
