import { LispSchema } from '@ngfw/schema';
import { describe, expect, it } from 'vitest';
import { sectionPatch, sectionSchema, sectionValue, SECTIONS, statusRows, type LispState } from './model';

const lisp = LispSchema.parse({
  enabled: true,
  gpe: true,
  locatorSets: { 'w11-rloc': { locators: [{ interface: 'host-w11-eth0' }] } },
  localEids: [{ vni: 1100, eid: '10.11.100.0/24', locatorSet: 'w11-rloc' }],
  eidTables: { '1100': { vrf: 'overlay' } },
  remoteMappings: [{ vni: 1100, eid: '10.11.200.0/24', rlocs: [{ address: '10.11.1.2' }] }],
  adjacencies: [{ vni: 1100, reid: '10.11.200.0/24', leid: '10.11.100.0/24' }],
  gpeEntries: [{ vni: 1101, vrf: 'overlay', reid: '10.11.201.0/24', leid: '10.11.101.0/24' }],
  mapResolvers: ['10.11.1.254'],
  pitr: 'w11-rloc',
});

const state: LispState = {
  enabled: true,
  gpeEnabled: true,
  pitr: 'w11-rloc',
  locatorSets: [{ name: 'w11-rloc', locators: [{ interface: 'host-w11-eth0', swIfIndex: 1, priority: 1, weight: 1 }] }],
  mappings: [
    { vni: 1100, eid: '10.11.100.0/24', local: true, locatorSet: 'w11-rloc', rlocs: [], action: 'no-action', authoritative: false, ttl: 0 },
  ],
  adjacencies: [],
  eidTables: [{ vni: 1100, dpTable: 1100, isL2: false }],
  mapResolvers: ['10.11.1.254'],
  mapServers: [],
  gpeVnis: [1101],
  retrievedAt: null,
};

describe('LISP tab model', () => {
  it('splits tunnels.lisp into the four sub-tab forms, every key exactly once', () => {
    const all = SECTIONS.flatMap((s) => Object.keys(sectionSchema(s.id).properties ?? {}));
    expect(all.sort()).toEqual(Object.keys(LispSchema.shape).sort());
    expect(sectionValue('resolvers', lisp)).toEqual({ mapResolvers: ['10.11.1.254'], mapServers: [], pitr: 'w11-rloc' });
  });

  it('a section save patches only its keys; cleared keys are removed', () => {
    expect(sectionPatch('resolvers', { mapResolvers: [] })).toEqual({ lisp: { mapResolvers: [], mapServers: null, pitr: null } });
  });

  it('status: in VPP / not in VPP / write-only', () => {
    const s = (id: Parameters<typeof statusRows>[0]) => statusRows(id, lisp, state).map((r) => [r.kind, r.id, r.status]);
    expect(s('locators')).toEqual([
      ['switch', 'LISP', 'up'],
      ['switch', 'LISP-GPE', 'up'],
      ['locatorSet', 'w11-rloc', 'up'],
    ]);
    expect(s('eids')).toEqual([
      ['localEid', '1100/10.11.100.0/24', 'up'],
      ['eidTable', '1100', 'up'],
    ]);
    expect(s('mappings')).toEqual([
      ['remoteMapping', '1100/10.11.200.0/24', 'down'],
      ['adjacency', '1100/10.11.200.0/24', 'down'],
      ['gpeEntry', '1101/10.11.201.0/24', 'degraded'],
    ]);
    expect(s('resolvers')).toEqual([
      ['mapResolver', '10.11.1.254', 'up'],
      ['pitr', 'w11-rloc', 'up'],
    ]);
    expect(statusRows('eids', lisp, null).every((r) => r.status === 'down')).toBe(true);
  });
});
