import { describe, expect, it } from 'vitest';
import { RootConfig } from '../index.js';
import { aclValidators, macToBigInt } from './acl.js';
import { sortIssues, type SemanticIssue } from './registry.js';

const base = {
  interfaces: {
    'Gig0/0/0': { vrf: 'default' },
    'Gig0/0/1': { vrf: 'cust' },
    'Gig0/0/2': {},
  },
  vrfs: { cust: {} },
  objects: {
    addresses: { srv: { type: 'host', address: '10.0.0.1' } },
    addressGroups: { grp: { members: ['srv'] } },
    services: { http: { protocol: 'tcp', destinationPorts: ['80'] } },
    serviceGroups: { web: { members: ['http'] } },
    schedules: { biz: { type: 'recurring', days: ['mon'], start: '08:00', end: '18:00' } },
    zones: { lan: { interfaces: ['Gig0/0/0'] }, wan: { interfaces: ['Gig0/0/1'] } },
    tags: { prod: {} },
  },
};

function run(name: string, acl: unknown): SemanticIssue[] {
  const v = aclValidators.find((x) => x.name === name);
  if (v === undefined) throw new Error(`no validator '${name}'`);
  return sortIssues(v.validate(RootConfig.parse({ ...base, acl })));
}

const pointers = (issues: SemanticIssue[]): string[] => issues.map((i) => i.pointer);
const rule = (sequence: number, extra: Record<string, unknown> = {}) => ({
  sequence,
  action: 'permit',
  ...extra,
});
const hostRule = (sequence: number, extra: Record<string, unknown> = {}) => ({
  sequence,
  action: 'accept',
  ...extra,
});

describe('macToBigInt', () => {
  it('parses both separators and cases', () => {
    expect(macToBigInt('aa:bb:cc:dd:ee:ff')).toBe(0xaabbccddeeffn);
    expect(macToBigInt('AA-BB-CC-DD-EE-FF')).toBe(0xaabbccddeeffn);
    expect(macToBigInt('00:00:00:00:00:01')).toBe(1n);
  });
});

describe('acl.rule-sequences-unique', () => {
  it('rejects repeated sequences in every list kind', () => {
    const issues = run('acl.rule-sequences-unique', {
      lists: { l: { rules: [rule(10), rule(20), rule(10)] } },
      macip: {
        m: {
          rules: [
            { sequence: 1, action: 'permit', sourceMac: '00:11:22:33:44:55' },
            { sequence: 1, action: 'deny', sourceMac: '00:11:22:33:44:66' },
          ],
        },
      },
      host: { h: { rules: [hostRule(5), hostRule(5)] } },
    });
    expect(issues).toEqual([
      { pointer: '/acl/host/h/rules/1/sequence', message: 'sequence 5 is already used by rule 0' },
      {
        pointer: '/acl/lists/l/rules/2/sequence',
        message: 'sequence 10 is already used by rule 0',
      },
      { pointer: '/acl/macip/m/rules/1/sequence', message: 'sequence 1 is already used by rule 0' },
    ]);
  });
});

describe('acl.rule-references', () => {
  it('rejects unknown address/service objects and schedules, accepts groups', () => {
    const issues = run('acl.rule-references', {
      lists: {
        l: {
          rules: [
            rule(1, {
              source: { kind: 'object', name: 'nope' },
              destination: { kind: 'object', name: 'grp' },
            }),
            rule(2, {
              destination: { kind: 'object', name: 'constructor' },
              service: { kind: 'object', name: 'nope' },
            }),
            rule(3, { service: { kind: 'object', name: 'web' }, schedule: 'nope' }),
            rule(4, {
              source: { kind: 'object', name: 'srv' },
              service: { kind: 'object', name: 'http' },
              schedule: 'biz',
            }),
          ],
        },
      },
      host: { h: { rules: [hostRule(1, { source: { kind: 'object', name: 'nope' } })] } },
    });
    expect(issues).toEqual([
      {
        pointer: '/acl/host/h/rules/0/source/name',
        message: expect.stringMatching(/'nope' is not an entry of objects.addresses/),
      },
      {
        pointer: '/acl/lists/l/rules/0/source/name',
        message: expect.stringMatching(/objects.addresses or objects.addressGroups/),
      },
      {
        pointer: '/acl/lists/l/rules/1/destination/name',
        message: expect.stringMatching(/'constructor' is not/),
      },
      {
        pointer: '/acl/lists/l/rules/1/service/name',
        message: expect.stringMatching(/objects.services or objects.serviceGroups/),
      },
      {
        pointer: '/acl/lists/l/rules/2/schedule',
        message: "schedule 'nope' does not exist in objects.schedules",
      },
    ]);
  });
});

describe('acl.rule-consistency', () => {
  it('rejects family mismatches between ipVersion, prefixes and ICMP flavour', () => {
    const issues = run('acl.rule-consistency', {
      lists: {
        l: {
          rules: [
            rule(1, { ipVersion: 'ipv4', source: { kind: 'prefix', prefix: '2001:db8::/32' } }),
            rule(2, {
              source: { kind: 'prefix', prefix: '10.0.0.0/8' },
              destination: { kind: 'prefix', prefix: '2001:db8::/32' },
            }),
            rule(3, { ipVersion: 'ipv6', service: { kind: 'inline', spec: { protocol: 'icmp' } } }),
            rule(4, {
              destination: { kind: 'prefix', prefix: '10.0.0.0/8' },
              service: { kind: 'inline', spec: { protocol: 'icmp6' } },
            }),
            rule(5, {
              source: { kind: 'object', name: 'srv' },
              service: { kind: 'inline', spec: { protocol: 'icmp', code: 3 } },
            }),
            rule(6, {
              service: {
                kind: 'inline',
                spec: { protocol: 'tcp', destinationPorts: ['80', '70-90'] },
              },
            }),
            rule(7, {
              ipVersion: 'ipv6',
              source: { kind: 'prefix', prefix: '2001:db8::/32' },
              service: { kind: 'inline', spec: { protocol: 'icmp6', type: 128 } },
            }),
            rule(8, { ipVersion: 'ipv4', service: { kind: 'object', name: 'http' } }),
          ],
        },
      },
      host: {
        h: {
          rules: [
            hostRule(1, {
              ipVersion: 'ipv6',
              destination: { kind: 'prefix', prefix: '10.0.0.0/8' },
            }),
          ],
        },
      },
    });
    expect(issues).toEqual([
      { pointer: '/acl/host/h/rules/0/destination/prefix', message: 'IPv4 prefix in an IPv6 rule' },
      { pointer: '/acl/lists/l/rules/0/source/prefix', message: 'IPv6 prefix in an IPv4 rule' },
      {
        pointer: '/acl/lists/l/rules/1/destination/prefix',
        message: 'IPv6 prefix in an IPv4 rule',
      },
      {
        pointer: '/acl/lists/l/rules/2/service/spec/protocol',
        message: 'icmp cannot be matched in an IPv6 rule',
      },
      {
        pointer: '/acl/lists/l/rules/3/service/spec/protocol',
        message: 'icmp6 cannot be matched in an IPv4 rule',
      },
      {
        pointer: '/acl/lists/l/rules/4/service/spec/code',
        message: 'ICMP code requires an ICMP type',
      },
      {
        pointer: '/acl/lists/l/rules/5/service/spec/destinationPorts/1',
        message: "port range '70-90' overlaps entry 0",
      },
    ]);
  });
});

describe('acl.tags-exist', () => {
  it('rejects unknown tags on every list kind', () => {
    expect(
      pointers(
        run('acl.tags-exist', {
          lists: { l: { tags: ['prod', 'nope'] } },
          macip: { m: { tags: ['nope'] } },
          host: { h: { tags: ['constructor'] } },
        }),
      ),
    ).toEqual(['/acl/host/h/tags/0', '/acl/lists/l/tags/1', '/acl/macip/m/tags/0']);
  });
});

describe('acl.macip-rules', () => {
  it('rejects a source MAC with bits outside its mask', () => {
    expect(
      run('acl.macip-rules', {
        macip: {
          m: {
            rules: [
              {
                sequence: 1,
                action: 'permit',
                sourceMac: 'aa:bb:cc:dd:ee:ff',
                sourceMacMask: 'ff:ff:ff:00:00:00',
              },
              {
                sequence: 2,
                action: 'permit',
                sourceMac: 'aa:bb:cc:00:00:00',
                sourceMacMask: 'ff:ff:ff:00:00:00',
              },
              { sequence: 3, action: 'deny', sourceMac: 'aa:bb:cc:dd:ee:ff' },
            ],
          },
        },
      }),
    ).toEqual([
      {
        pointer: '/acl/macip/m/rules/0/sourceMac',
        message: 'source MAC has bits set outside sourceMacMask',
      },
    ]);
  });
});

describe('acl.attachments', () => {
  const lists = { fw: { rules: [rule(1)] } };
  const att = (extra: Record<string, unknown>) => ({
    list: 'fw',
    target: { kind: 'interface', interface: 'Gig0/0/0' },
    sequence: 1,
    ...extra,
  });

  it('rejects unknown list, interface, zone and VRF', () => {
    expect(
      run('acl.attachments', {
        lists,
        attachments: [
          att({ list: 'nope', sequence: 1 }),
          att({ target: { kind: 'interface', interface: 'nope' }, sequence: 2 }),
          att({ target: { kind: 'zone', zone: 'nope' }, sequence: 3 }),
          att({ vrf: 'nope', sequence: 4 }),
        ],
      }),
    ).toEqual([
      { pointer: '/acl/attachments/0/list', message: "access list 'nope' does not exist" },
      {
        pointer: '/acl/attachments/1/target/interface',
        message: "interface 'nope' does not exist",
      },
      { pointer: '/acl/attachments/2/target/zone', message: "zone 'nope' does not exist" },
      { pointer: '/acl/attachments/3/vrf', message: "VRF 'nope' does not exist" },
    ]);
  });

  it('requires vrf to match the target interface(s) when they declare one', () => {
    // distinct lists per attachment: zone 'wan' contains Gig0/0/1 and 'lan' contains Gig0/0/0 (see L7 below)
    expect(
      run('acl.attachments', {
        lists: { ...lists, viaZoneWan: { rules: [] }, viaZoneLan: { rules: [] } },
        attachments: [
          att({ vrf: 'default', sequence: 1 }),
          att({
            target: { kind: 'interface', interface: 'Gig0/0/1' },
            vrf: 'default',
            sequence: 2,
          }),
          att({
            list: 'viaZoneWan',
            target: { kind: 'zone', zone: 'wan' },
            vrf: 'default',
            sequence: 3,
          }),
          att({ target: { kind: 'interface', interface: 'Gig0/0/2' }, vrf: 'cust', sequence: 4 }),
          att({ list: 'viaZoneLan', target: { kind: 'zone', zone: 'lan' }, sequence: 5 }),
        ],
      }),
    ).toEqual([
      {
        pointer: '/acl/attachments/1/vrf',
        message: "interface 'Gig0/0/1' is in VRF 'cust', not 'default'",
      },
      {
        pointer: '/acl/attachments/2/vrf',
        message: "interface 'Gig0/0/1' is in VRF 'cust', not 'default'",
      },
    ]);
  });

  it('rejects the same list twice on one target+direction and shared sequences', () => {
    expect(
      run('acl.attachments', {
        lists: { ...lists, other: { rules: [] } },
        attachments: [
          att({ sequence: 1 }),
          att({ sequence: 2 }),
          att({ list: 'other', sequence: 1 }),
          att({ direction: 'out', sequence: 1 }),
          // zone 'lan' = [Gig0/0/0]: the same list + sequence reach Gig0/0/0 a second time through the zone
          att({ target: { kind: 'zone', zone: 'lan' }, sequence: 1 }),
        ],
      }),
    ).toEqual([
      {
        pointer: '/acl/attachments/1/list',
        message: "access list 'fw' is already attached to this target in direction 'in'",
      },
      {
        pointer: '/acl/attachments/2/sequence',
        message: 'sequence 1 is already used by attachment 0 on the same target and direction',
      },
      {
        pointer: '/acl/attachments/4/list',
        message:
          "access list 'fw' is already attached to interface 'Gig0/0/0' (attachment 0) in direction 'in'",
      },
      {
        pointer: '/acl/attachments/4/sequence',
        message:
          "sequence 1 is already used by attachment 0 on interface 'Gig0/0/0' in direction 'in'",
      },
    ]);
  });

  it('expands zones before the duplicate checks, in either order and only for the same direction (L7)', () => {
    // zone first, member interface second → reported at the interface attachment
    expect(
      run('acl.attachments', {
        lists: { ...lists, other: { rules: [] } },
        attachments: [
          att({ target: { kind: 'zone', zone: 'lan' }, sequence: 10 }),
          att({ sequence: 20 }),
          att({ list: 'other', sequence: 10 }),
          att({
            list: 'other',
            target: { kind: 'zone', zone: 'lan' },
            direction: 'out',
            sequence: 10,
          }),
          att({ target: { kind: 'zone', zone: 'wan' }, sequence: 10 }),
          att({ target: { kind: 'zone', zone: 'nope' }, sequence: 10 }),
          att({ target: { kind: 'zone', zone: 'nope' }, sequence: 10 }),
        ],
      }),
    ).toEqual([
      {
        pointer: '/acl/attachments/1/list',
        message:
          "access list 'fw' is already attached to interface 'Gig0/0/0' (attachment 0) in direction 'in'",
      },
      {
        pointer: '/acl/attachments/2/sequence',
        message:
          "sequence 10 is already used by attachment 0 on interface 'Gig0/0/0' in direction 'in'",
      },
      { pointer: '/acl/attachments/5/target/zone', message: "zone 'nope' does not exist" },
      {
        pointer: '/acl/attachments/6/list',
        message: "access list 'fw' is already attached to this target in direction 'in'",
      },
      {
        pointer: '/acl/attachments/6/sequence',
        message: 'sequence 10 is already used by attachment 5 on the same target and direction',
      },
      { pointer: '/acl/attachments/6/target/zone', message: "zone 'nope' does not exist" },
    ]);
    // one issue per attachment and check, even when the literal target and a member both collide
    expect(
      pointers(
        run('acl.attachments', {
          lists,
          attachments: [
            att({ target: { kind: 'zone', zone: 'lan' }, sequence: 1 }),
            att({ sequence: 1 }),
            att({ target: { kind: 'zone', zone: 'lan' }, sequence: 1 }),
          ],
        }),
      ),
    ).toEqual([
      '/acl/attachments/1/list',
      '/acl/attachments/1/sequence',
      '/acl/attachments/2/list',
      '/acl/attachments/2/sequence',
    ]);
  });
});

describe('acl.macip-attachments', () => {
  const macip = { m: { rules: [] } };

  it('rejects unknown list/interface/VRF, VRF mismatch and two lists per interface', () => {
    expect(
      run('acl.macip-attachments', {
        macip,
        macipAttachments: [
          { list: 'nope', interface: 'nope', vrf: 'nope' },
          { list: 'm', interface: 'Gig0/0/1', vrf: 'default' },
          { list: 'm', interface: 'Gig0/0/1' },
          { list: 'm', interface: 'Gig0/0/0', vrf: 'default' },
        ],
      }),
    ).toEqual([
      { pointer: '/acl/macipAttachments/0/interface', message: "interface 'nope' does not exist" },
      {
        pointer: '/acl/macipAttachments/0/list',
        message: "MACIP access list 'nope' does not exist",
      },
      { pointer: '/acl/macipAttachments/0/vrf', message: "VRF 'nope' does not exist" },
      {
        pointer: '/acl/macipAttachments/1/vrf',
        message: "interface 'Gig0/0/1' is in VRF 'cust', not 'default'",
      },
      {
        pointer: '/acl/macipAttachments/2/interface',
        message: expect.stringMatching(/already has a MACIP access list/),
      },
    ]);
  });
});

describe('acl.host-attachments', () => {
  it('rejects unknown lists and the same list twice on one chain', () => {
    expect(
      run('acl.host-attachments', {
        host: { h: { rules: [] } },
        hostAttachments: [
          { list: 'nope', chain: 'input' },
          { list: 'h', chain: 'input' },
          { list: 'h', chain: 'input', priority: 10 },
          { list: 'h', chain: 'output' },
        ],
      }),
    ).toEqual([
      { pointer: '/acl/hostAttachments/0/list', message: "host access list 'nope' does not exist" },
      {
        pointer: '/acl/hostAttachments/2/chain',
        message: "host access list 'h' is already attached to chain 'input'",
      },
    ]);
  });
});
