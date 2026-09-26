import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { bondIdOf, BondSchema } from '../domains/ext/bonding.js';
import { RootConfig, type RootConfigInput } from '../index.js';
import { bondingValidators } from './bonding.js';
import { validateSemantics } from './index.js';

const run = (name: string, doc: RootConfigInput) =>
  bondingValidators.find((v) => v.name === name)!.validate(RootConfig.parse(doc));

const all = (doc: RootConfigInput) =>
  validateSemantics(RootConfig.parse(doc)).filter((i) => i.pointer.startsWith('/interfaces'));

/** A two-member LACP bond with an address, as in docs/user/interfaces/bonding.md. */
const LACP: RootConfigInput = {
  interfaces: {
    BondEthernet0: {
      enabled: true,
      ipv4: ['192.0.2.1/24'],
      bond: {
        mode: 'lacp',
        loadBalance: 'l34',
        members: { 'TenGigabitEthernet0/0/0': {}, 'TenGigabitEthernet0/0/1': { passive: true } },
      },
    },
    'TenGigabitEthernet0/0/0': { enabled: true },
    'TenGigabitEthernet0/0/1': { enabled: true },
  },
};

const B0 = '/interfaces/BondEthernet0/bond';
const T0 = 'TenGigabitEthernet0~10~10';
const T1 = 'TenGigabitEthernet0~10~11';

describe('interfaces.<name>.bond schema', () => {
  it('fills defaults and keeps the document idempotent', () => {
    const doc = RootConfig.parse(LACP);
    expect(doc.interfaces['BondEthernet0']!.bond).toEqual({
      mode: 'lacp',
      loadBalance: 'l34',
      members: {
        'TenGigabitEthernet0/0/0': { passive: false, longTimeout: false },
        'TenGigabitEthernet0/0/1': { passive: true, longTimeout: false },
      },
      numaOnly: false,
    });
    expect(RootConfig.parse(doc)).toEqual(doc);
    expect(all(LACP)).toEqual([]);
  });

  it('rejects unknown modes, hashes, sub-interface members, weights out of range and unknown keys', () => {
    for (const bad of [
      { mode: 'lag' },
      { mode: 'xor', loadBalance: 'l4' },
      { mode: 'lacp', members: { 'eth0.100': {} } },
      { mode: 'active-backup', members: { eth0: { weight: 0 } } },
      { mode: 'active-backup', members: { eth0: { weight: 256 } } },
      { mode: 'lacp', id: 4294967295 },
      { mode: 'lacp', mac: '02:00:00:00:00:01' },
      { members: {} },
      { mode: 'broadcast' }, // Q1 (manager): not offered
    ]) {
      expect(BondSchema.safeParse(bad).success, JSON.stringify(bad)).toBe(false);
    }
  });

  it('parses the bond id out of the interface name', () => {
    expect(bondIdOf('BondEthernet0')).toBe(0);
    expect(bondIdOf('BondEthernet6000')).toBe(6000);
    expect(bondIdOf('BondEthernet4294967294')).toBe(4294967294);
    expect(bondIdOf('BondEthernet4294967295')).toBeUndefined();
    expect(bondIdOf('BondEthernet01')).toBeUndefined();
    expect(bondIdOf('BondEthernet')).toBeUndefined();
    expect(bondIdOf('lag0')).toBeUndefined();
  });
});

describe('interfaces.bonding-name', () => {
  it('requires BondEthernet<id> and a matching id', () => {
    expect(
      run('interfaces.bonding-name', {
        interfaces: {
          lag0: { bond: { mode: 'xor' } },
          BondEthernet7: { bond: { mode: 'xor', id: 8 } },
          BondEthernet9: { bond: { mode: 'xor', id: 9 } },
          BondEthernet3: {}, // a bond that already exists: not created, no rule
        },
      }),
    ).toEqual([
      {
        pointer: '/interfaces/lag0/bond',
        message: "a bond interface must be named BondEthernet<id> (VPP's name), not 'lag0'",
      },
      {
        pointer: '/interfaces/BondEthernet7/bond/id',
        message: 'bond id 8 does not match the interface name BondEthernet7 (id 7)',
      },
    ]);
  });
});

describe('interfaces.bonding-member-exists / -kind', () => {
  it('reports members that are not configured, bonds, loopbacks and the bond itself', () => {
    const doc: RootConfigInput = {
      interfaces: {
        BondEthernet1: {
          bond: {
            mode: 'xor',
            members: { eth9: {}, loop0: {}, BondEthernet2: {}, BondEthernet1: {} },
          },
        },
        BondEthernet2: { bond: { mode: 'xor' } },
        loop0: {},
      },
    };
    expect(run('interfaces.bonding-member-exists', doc)).toEqual([
      {
        pointer: '/interfaces/BondEthernet1/bond/members/eth9',
        message:
          "member interface 'eth9' is not configured (add interfaces.eth9 with enabled: true)",
      },
    ]);
    expect(run('interfaces.bonding-member-kind', doc)).toEqual([
      {
        pointer: '/interfaces/BondEthernet1/bond/members/loop0',
        message: "'loop0' is a loopback; bond members must be physical interfaces",
      },
      {
        pointer: '/interfaces/BondEthernet1/bond/members/BondEthernet2',
        message: "'BondEthernet2' is a bond; bond members must be physical interfaces",
      },
      {
        pointer: '/interfaces/BondEthernet1/bond/members/BondEthernet1',
        message: 'a bond cannot be a member of itself',
      },
    ]);
  });
});

describe('interfaces.bonding-member-unique', () => {
  it('points at the second membership (higher bond id) of an interface in two bonds', () => {
    const issues = run('interfaces.bonding-member-unique', {
      interfaces: {
        BondEthernet6001: { bond: { mode: 'xor', members: { tap6000: {} } } },
        BondEthernet6000: { bond: { mode: 'lacp', members: { tap6000: {}, tap6001: {} } } },
        tap6000: { enabled: true },
        tap6001: { enabled: true },
      },
    });
    expect(issues).toEqual([
      {
        pointer: '/interfaces/BondEthernet6001/bond/members/tap6000',
        message:
          "'tap6000' is already a member of BondEthernet6000 (/interfaces/BondEthernet6000/bond/members/tap6000); an interface belongs to at most one bond",
      },
    ]);
  });
});

describe('interfaces.bonding-member-l3', () => {
  it('rejects addresses, DHCP, VRF and sub-interfaces on a member, once per member', () => {
    const issues = run('interfaces.bonding-member-l3', {
      vrfs: { red: { id: 10 } },
      interfaces: {
        BondEthernet0: { bond: { mode: 'xor', members: { a: {}, b: {}, c: {}, d: {}, e: {} } } },
        a: { ipv4: ['10.0.0.1/24'] },
        b: { ipv6: ['2001:db8::1/64'] },
        c: { dhcpClient: {} },
        d: { vrf: 'red' },
        e: { subinterfaces: { '10': { vlanId: 10 } } },
      },
    });
    expect(issues.map((i) => i.pointer)).toEqual([
      '/interfaces/a/ipv4',
      '/interfaces/b/ipv6',
      '/interfaces/c/dhcpClient',
      '/interfaces/d/vrf',
      '/interfaces/e/subinterfaces',
    ]);
    expect(issues[0]!.message).toBe(
      'a is a member of BondEthernet0: configure IPv4 addresses on the bond, not on a member',
    );
    expect(
      run('interfaces.bonding-member-l3', {
        interfaces: {
          BondEthernet0: { bond: { mode: 'xor', members: { a: {} } } },
          a: { enabled: true, mtu: 9000 },
        },
      }),
    ).toEqual([]);
  });
});

describe('mode-dependent options', () => {
  it('loadBalance only for xor/lacp', () => {
    expect(
      run('interfaces.bonding-load-balance', {
        interfaces: {
          BondEthernet0: { bond: { mode: 'lacp', loadBalance: 'l23' } },
          BondEthernet1: { bond: { mode: 'active-backup', loadBalance: 'l2' } },
          BondEthernet2: { bond: { mode: 'round-robin' } },
        },
      }),
    ).toEqual([
      {
        pointer: '/interfaces/BondEthernet1/bond/loadBalance',
        message:
          'loadBalance applies to xor and lacp bonds only; VPP forces the algorithm of a active-backup bond',
      },
    ]);
  });

  it('passive / longTimeout only for lacp, weight only for active-backup', () => {
    const doc: RootConfigInput = {
      interfaces: {
        BondEthernet0: {
          bond: {
            mode: 'xor',
            members: { a: { passive: true }, b: { longTimeout: true }, c: { weight: 5 } },
          },
        },
        BondEthernet1: { bond: { mode: 'active-backup', members: { d: { weight: 200 } } } },
        BondEthernet2: {
          bond: { mode: 'lacp', members: { e: { passive: true, longTimeout: true } } },
        },
        a: {},
        b: {},
        c: {},
        d: {},
        e: {},
      },
    };
    expect(run('interfaces.bonding-lacp-options', doc).map((i) => i.pointer)).toEqual([
      `/interfaces/BondEthernet0/bond/members/a/passive`,
      `/interfaces/BondEthernet0/bond/members/b/longTimeout`,
    ]);
    expect(run('interfaces.bonding-weight', doc)).toEqual([
      {
        pointer: '/interfaces/BondEthernet0/bond/members/c/weight',
        message: 'weight applies to active-backup bonds only; BondEthernet0 is a xor bond',
      },
    ]);
  });
});

describe('full documents', () => {
  it('the LACP example and the proto fixture are clean', () => {
    expect(all(LACP)).toEqual([]);
    const fixture = JSON.parse(
      readFileSync(
        new URL('../../../proto/test/fixtures/bonding-lacp-ab.json', import.meta.url),
        'utf8',
      ),
    ) as RootConfigInput;
    expect(RootConfig.safeParse(fixture).error?.issues).toBeUndefined();
    expect(validateSemantics(RootConfig.parse(fixture))).toEqual([]);
  });

  it('every rule is registered once, under the interfaces.bonding- prefix', () => {
    const names = bondingValidators.map((v) => v.name);
    expect(new Set(names).size).toBe(names.length);
    expect(names.every((n) => n.startsWith('interfaces.bonding-'))).toBe(true);
  });

  it('pointers escape interface names with /', () => {
    const issues = all({
      interfaces: {
        BondEthernet0: {
          bond: { mode: 'round-robin', members: { 'TenGigabitEthernet0/0/0': { weight: 1 } } },
        },
        BondEthernet1: {
          bond: {
            mode: 'xor',
            members: { 'TenGigabitEthernet0/0/0': {}, 'TenGigabitEthernet0/0/1': {} },
          },
        },
        'TenGigabitEthernet0/0/0': {},
      },
    });
    expect(issues.map((i) => i.pointer)).toEqual([
      `${B0}/members/${T0}/weight`,
      `/interfaces/BondEthernet1/bond/members/${T0}`,
      `/interfaces/BondEthernet1/bond/members/${T1}`,
    ]);
  });
});

describe('fix round 1', () => {
  it('F4: tunnels and other virtual L3 interfaces are not members (a 400 with the membership pointer)', () => {
    const issues = run('interfaces.bonding-member-kind', {
      interfaces: {
        BondEthernet0: {
          bond: {
            mode: 'xor',
            members: { wg0: {}, ipip3: {}, gre1: {}, vxlan_tunnel2: {}, eth9: {} },
          },
        },
        wg0: {},
        ipip3: {},
        gre1: {},
        vxlan_tunnel2: {},
        eth9: {},
      },
    });
    expect(issues.map((i) => i.pointer)).toEqual([
      '/interfaces/BondEthernet0/bond/members/wg0',
      '/interfaces/BondEthernet0/bond/members/ipip3',
      '/interfaces/BondEthernet0/bond/members/gre1',
      '/interfaces/BondEthernet0/bond/members/vxlan_tunnel2',
    ]);
    expect(issues[0]!.message).toBe(
      "'wg0' is not an Ethernet interface (a tunnel or virtual L3 interface); bond members must be physical interfaces",
    );
  });

  it('F5: a member has no MAC of its own (the bond gives it one)', () => {
    const doc: RootConfigInput = {
      interfaces: {
        BondEthernet0: {
          mac: '02:00:00:00:00:01',
          bond: { mode: 'lacp', members: { a: {}, b: {} } },
        },
        a: { mac: '02:00:00:00:00:0a' },
        b: {},
      },
    };
    expect(run('interfaces.bonding-member-mac', doc)).toEqual([
      {
        pointer: '/interfaces/a/mac',
        message:
          "a is a member of BondEthernet0: members take the bond's MAC address; set mac on the bond instead",
      },
    ]);
    expect(all(doc).map((i) => i.pointer)).toEqual(['/interfaces/a/mac']);
  });

  it('Q1: broadcast is not a mode', () => {
    expect(
      RootConfig.safeParse({ interfaces: { BondEthernet0: { bond: { mode: 'broadcast' } } } })
        .success,
    ).toBe(false);
  });
});
