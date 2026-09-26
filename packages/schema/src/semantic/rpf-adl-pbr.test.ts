import { describe, expect, it } from 'vitest';
import { RootConfig, type RootConfigInput } from '../index.js';
import { validateSemantics } from './index.js';
import { rpfAdlPbrValidators } from './rpf-adl-pbr.js';

const run = (name: string, doc: RootConfigInput) =>
  rpfAdlPbrValidators.find((v) => v.name === name)!.validate(RootConfig.parse(doc));

/** A valid document using every field of the feature (also the base of the negative cases). */
const base = (): RootConfigInput => ({
  vrfs: { wan2: { id: 10 }, allow: { id: 11 } },
  interfaces: {
    'GigabitEthernet0/8/0': {
      enabled: true,
      ipv4: ['198.51.100.2/30'],
      urpf: { ipv4: 'strict', ipv6: 'loose' },
      adl: { ipv4: true, allowVrf: 'allow' },
    },
    'GigabitEthernet0/9/0': { enabled: true, vrf: 'wan2', ipv4: ['203.0.113.2/30'] },
    loop0: { enabled: true, ipv4: ['10.0.0.1/24'] },
  },
  routing: {
    pbr: {
      policies: {
        'via-wan2': {
          acl: 'from-lan-b',
          priority: 10,
          paths: [{ address: '203.0.113.1', interface: 'GigabitEthernet0/9/0' }],
        },
        'lookup-wan2': { acl: 'from-lan-b', paths: [{ vrf: 'wan2' }] },
      },
      attachments: [
        { policy: 'via-wan2', interface: 'loop0' },
        { policy: 'lookup-wan2', interface: 'loop0', family: 'ipv6' },
      ],
    },
  },
  acl: {
    lists: {
      'from-lan-b': {
        rules: [
          {
            sequence: 10,
            action: 'permit',
            ipVersion: 'ipv4',
            source: { kind: 'prefix', prefix: '10.0.1.0/24' },
          },
        ],
      },
    },
  },
  services: { autoSdl: { enabled: true, threshold: 10, removeTimeoutSec: 600 } },
});

describe('F-rpf-adl-pbr schema', () => {
  it('parses the full example with defaults filled and nothing flagged', () => {
    const parsed = RootConfig.parse(base());
    expect(parsed.interfaces['GigabitEthernet0/8/0']?.urpf).toEqual({
      ipv4: 'strict',
      ipv6: 'loose',
      direction: 'rx',
    });
    expect(parsed.interfaces['GigabitEthernet0/8/0']?.adl).toEqual({
      ipv4: true,
      ipv6: false,
      allowVrf: 'allow',
      defaultAllow: true,
    });
    expect(parsed.routing.pbr?.policies['lookup-wan2']).toEqual({
      acl: 'from-lan-b',
      priority: 100,
      paths: [{ vrf: 'wan2', weight: 1 }],
    });
    expect(parsed.routing.pbr?.attachments[0]?.family).toBe('ipv4');
    expect(parsed.services.autoSdl).toEqual({
      enabled: true,
      threshold: 10,
      removeTimeoutSec: 600,
    });
    expect(validateSemantics(parsed)).toEqual([]);
  });

  it('absent means off: no key is invented by the defaults', () => {
    const parsed = RootConfig.parse({ interfaces: { loop0: {} } });
    expect(parsed.interfaces['loop0']?.urpf).toBeUndefined();
    expect(parsed.interfaces['loop0']?.adl).toBeUndefined();
    expect(parsed.routing.pbr).toBeUndefined();
    expect(parsed.services.autoSdl).toBeUndefined();
    // an ADL object holding only its defaults checks nothing and needs no allow-list VRF
    expect(RootConfig.safeParse({ interfaces: { loop0: { adl: {} } } }).success).toBe(true);
  });

  it('rejects malformed values at the offending field', () => {
    const bad = (doc: RootConfigInput) =>
      RootConfig.safeParse(doc).error?.issues.map((i) => i.path.join('/'));
    expect(bad({ interfaces: { loop0: { urpf: { ipv4: 'feasible' as 'loose' } } } })).toEqual([
      'interfaces/loop0/urpf/ipv4',
    ]);
    expect(bad({ interfaces: { loop0: { adl: { ipv4: true } } } })).toEqual([
      'interfaces/loop0/adl/allowVrf',
    ]);
    expect(bad({ routing: { pbr: { policies: { 'a#1': { acl: 'x', paths: [{}] } } } } })).toEqual([
      'routing/pbr/policies/a#1',
    ]);
    expect(bad({ routing: { pbr: { policies: { p: { acl: 'x', paths: [] } } } } })).toEqual([
      'routing/pbr/policies/p/paths',
    ]);
    expect(
      bad({
        routing: {
          pbr: { policies: { p: { acl: 'x', paths: [{ interface: 'loop0', vrf: 'red' }] } } },
        },
      }),
    ).toEqual(['routing/pbr/policies/p/paths/0/vrf']);
    expect(bad({ services: { autoSdl: { threshold: 0 } } })).toEqual([
      'services/autoSdl/threshold',
    ]);
  });
});

describe('routing.rpf-adl-pbr-acl-exists', () => {
  it('reports a policy naming an unknown ACL at its acl field', () => {
    const doc = base();
    doc.acl = { lists: {} };
    expect(run('routing.rpf-adl-pbr-acl-exists', doc)).toEqual([
      { pointer: '/routing/pbr/policies/via-wan2/acl', message: "ACL 'from-lan-b' does not exist" },
      {
        pointer: '/routing/pbr/policies/lookup-wan2/acl',
        message: "ACL 'from-lan-b' does not exist",
      },
    ]);
    expect(run('routing.rpf-adl-pbr-acl-exists', base())).toEqual([]);
  });
});

describe('routing.rpf-adl-pbr-path-refs', () => {
  it('reports unknown path VRFs and interfaces', () => {
    const doc = base();
    doc.routing!.pbr!.policies!['lookup-wan2']!.paths = [
      { vrf: 'nope' },
      { address: '192.0.2.1', interface: 'GigabitEthernet0/7/0' },
    ];
    expect(run('routing.rpf-adl-pbr-path-refs', doc)).toEqual([
      {
        pointer: '/routing/pbr/policies/lookup-wan2/paths/0/vrf',
        message: "VRF 'nope' does not exist",
      },
      {
        pointer: '/routing/pbr/policies/lookup-wan2/paths/1/interface',
        message: "interface 'GigabitEthernet0/7/0' does not exist",
      },
    ]);
  });
});

describe('routing.rpf-adl-pbr-attachment-refs / -path-family / -attachment-unique', () => {
  it('reports an unknown policy and interface', () => {
    const doc = base();
    doc.routing!.pbr!.attachments = [
      { policy: 'nope', interface: 'loop0' },
      { policy: 'via-wan2', interface: 'loop9' },
    ];
    expect(run('routing.rpf-adl-pbr-attachment-refs', doc)).toEqual([
      { pointer: '/routing/pbr/attachments/0/policy', message: "PBR policy 'nope' does not exist" },
      {
        pointer: '/routing/pbr/attachments/1/interface',
        message: "interface 'loop9' does not exist",
      },
    ]);
  });
  it('accepts a sub-interface as attachment target', () => {
    const doc = base();
    doc.interfaces!['GigabitEthernet0/9/0']!.subinterfaces = { '100': { vlanId: 100 } };
    doc.routing!.pbr!.attachments = [
      { policy: 'lookup-wan2', interface: 'GigabitEthernet0/9/0.100' },
    ];
    expect(run('routing.rpf-adl-pbr-attachment-refs', doc)).toEqual([]);
  });
  it('refuses to attach a policy with IPv4 next hops for IPv6, and a policy mixing families', () => {
    const doc = base();
    doc.routing!.pbr!.attachments = [{ policy: 'via-wan2', interface: 'loop0', family: 'ipv6' }];
    expect(run('routing.rpf-adl-pbr-attachment-refs', doc)).toEqual([
      {
        pointer: '/routing/pbr/attachments/0/family',
        message: "policy 'via-wan2' forwards to IPv4 next hops; it cannot be attached for IPv6",
      },
    ]);
    doc.routing!.pbr!.policies!['via-wan2']!.paths.push({ address: '2001:db8::1' });
    expect(run('routing.rpf-adl-pbr-path-family', doc)).toEqual([
      {
        pointer: '/routing/pbr/policies/via-wan2/paths',
        message:
          "policy 'via-wan2' mixes IPv4 and IPv6 next hops; use one policy per address family",
      },
    ]);
  });
  it('reports a repeated attachment at the later item', () => {
    const doc = base();
    doc.routing!.pbr!.attachments = [
      { policy: 'via-wan2', interface: 'loop0' },
      { policy: 'via-wan2', interface: 'loop0', family: 'ipv4' },
    ];
    expect(run('routing.rpf-adl-pbr-attachment-unique', doc)).toEqual([
      {
        pointer: '/routing/pbr/attachments/1',
        message:
          "policy 'via-wan2' is attached to loop0 (ipv4) twice (first defined at /routing/pbr/attachments/0)",
      },
    ]);
  });
});

describe('interfaces.rpf-adl-pbr-adl-vrf-exists', () => {
  it('reports an unknown allow-list VRF with an escaped pointer', () => {
    const doc = base();
    doc.interfaces!['GigabitEthernet0/8/0']!.adl = { ipv4: true, allowVrf: 'nope' };
    expect(run('interfaces.rpf-adl-pbr-adl-vrf-exists', doc)).toEqual([
      {
        pointer: '/interfaces/GigabitEthernet0~18~10/adl/allowVrf',
        message: "VRF 'nope' does not exist",
      },
    ]);
  });
});
