import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { RootConfig, type RootConfigInput } from '../index.js';
import { semanticRegistry, validateSemantics } from './index.js';
import { srv6Validators } from './srv6.js';

/** The F-srv6 proto fixture is the valid corpus document of this feature. */
const fixture = (): RootConfigInput =>
  JSON.parse(
    readFileSync(new URL('../../../proto/test/fixtures/srv6-full.json', import.meta.url), 'utf8'),
  ) as RootConfigInput;

type Doc = ReturnType<typeof fixture> & { routing: { srv6: Record<string, unknown> } };

/** Parses `edit(fixture)` with the schema and returns the F-srv6 findings as `pointer → message`. */
function issues(edit: (d: Doc) => void = () => undefined): Record<string, string> {
  const doc = fixture() as Doc;
  edit(doc);
  const parsed = RootConfig.safeParse(doc);
  expect(parsed.error?.issues).toBeUndefined();
  const names = new Set(srv6Validators.map((v) => v.name));
  const out: Record<string, string> = {};
  for (const v of semanticRegistry.list()) {
    if (!names.has(v.name)) continue;
    for (const i of v.validate(parsed.data!)) out[`${v.name} ${i.pointer}`] = i.message;
  }
  return out;
}

const srv6 = (d: Doc) => d.routing.srv6 as {
  encapSource?: string;
  localSids: Record<string, Record<string, unknown>>;
  policies: Record<string, Record<string, unknown> & { sidLists: { sids: string[]; weight?: number }[] }>;
  steering: Record<string, unknown>[];
};

describe('routing.srv6 schema', () => {
  it('the fixture is schema-valid and semantically clean (whole registry)', () => {
    const parsed = RootConfig.parse(fixture());
    expect(validateSemantics(parsed)).toEqual([]);
    expect(parsed.routing.srv6?.localSids['fd00:4:ff::1']).toEqual({ behavior: 'end', psp: true, vrf: 'default' });
  });

  it('fills the defaults (psp off, default VRF, encap on, weight 1, l3 VRF default)', () => {
    const parsed = RootConfig.parse({
      routing: {
        srv6: {
          encapSource: 'fd00:4::1',
          localSids: { 'fd00:4:ff::1': { behavior: 'end' } },
          policies: { 'fd00:4:bb::1': { sidLists: [{ sids: ['fd00:4:ee::1'] }] } },
          steering: [{ type: 'l3', prefix: '10.4.0.0/16', bsid: 'fd00:4:bb::1' }],
        },
      },
    });
    expect(parsed.routing.srv6).toEqual({
      encapSource: 'fd00:4::1',
      localSids: { 'fd00:4:ff::1': { behavior: 'end', psp: false, vrf: 'default' } },
      policies: {
        'fd00:4:bb::1': {
          type: 'default',
          encap: true,
          vrf: 'default',
          sidLists: [{ sids: ['fd00:4:ee::1'], weight: 1 }],
        },
      },
      steering: [{ type: 'l3', prefix: '10.4.0.0/16', vrf: 'default', bsid: 'fd00:4:bb::1' }],
    });
    expect(validateSemantics(parsed)).toEqual([]);
  });

  it('is optional: {} has no srv6 and no findings', () => {
    const parsed = RootConfig.parse({});
    expect(parsed.routing.srv6).toBeUndefined();
    expect(validateSemantics(parsed)).toEqual([]);
  });

  it.each([
    ['more than 16 SIDs', { sids: Array.from({ length: 17 }, (_, i) => `fd00:4:ee::${i + 1}`) }],
    ['no SID', { sids: [] }],
    ['an IPv4 segment', { sids: ['10.0.0.1'] }],
    ['weight 0', { sids: ['fd00:4:ee::1'], weight: 0 }],
  ])('rejects a segment list with %s', (_, list) => {
    const doc = fixture() as Doc;
    srv6(doc).policies['fd00:4:bb::1']!.sidLists = [list as { sids: string[] }];
    expect(RootConfig.safeParse(doc).success).toBe(false);
  });

  it.each([
    ['a policy without segment lists', (d: Doc) => void (srv6(d).policies['fd00:4:bb::1']!.sidLists = [])],
    ['an IPv4 local SID key', (d: Doc) => void (srv6(d).localSids['10.0.0.1'] = { behavior: 'end' })],
    ['an unknown behaviour (end.ad proxy)', (d: Doc) => void (srv6(d).localSids['fd00:4:ff::9'] = { behavior: 'end.ad' })],
    ['a steering entry without a type', (d: Doc) => void srv6(d).steering.push({ prefix: '10.0.0.0/8', bsid: 'fd00:4:bb::1' })],
    ['an L2 entry with a prefix', (d: Doc) => void srv6(d).steering.push({ type: 'l2', interface: 'host-w4l0', prefix: '10.0.0.0/8', bsid: 'fd00:4:bb::1' })],
    ['an L3 prefix with host bits', (d: Doc) => void srv6(d).steering.push({ type: 'l3', prefix: '10.4.0.1/16', bsid: 'fd00:4:bb::1' })],
    ['hop limit 0', (d: Doc) => void (d.routing.srv6['encapHopLimit'] = 0)],
  ])('rejects %s', (_, edit) => {
    const doc = fixture() as Doc;
    edit(doc);
    expect(RootConfig.safeParse(doc).success).toBe(false);
  });
});

describe('routing.srv6 semantic rules', () => {
  it('fixture: no findings', () => {
    expect(issues()).toEqual({});
  });

  it('routing.srv6-canonical: keys, segments, next hops and prefixes in canonical form', () => {
    const got = issues((d) => {
      srv6(d).localSids['FD00:4:ff:0::7'] = { behavior: 'end' };
      srv6(d).policies['fd00:4:bb::1']!.sidLists[0]!.sids[0] = 'fd00:4:ee:0:0::1';
      srv6(d).localSids['fd00:4:ff::2']!['nextHop'] = 'FD00:4:1::2';
    });
    expect(got).toEqual({
      'routing.srv6-canonical /routing/srv6/localSids/FD00:4:ff:0::7': 'write FD00:4:ff:0::7 in canonical form: fd00:4:ff::7',
      'routing.srv6-canonical /routing/srv6/policies/fd00:4:bb::1/sidLists/0/sids/0':
        'write fd00:4:ee:0:0::1 in canonical form: fd00:4:ee::1',
      'routing.srv6-canonical /routing/srv6/localSids/fd00:4:ff::2/nextHop': 'write FD00:4:1::2 in canonical form: fd00:4:1::2',
    });
  });

  it('routing.srv6-sid-address: no ::, ::1 or multicast SIDs and sources', () => {
    const got = issues((d) => {
      srv6(d).localSids['::'] = { behavior: 'end' };
      srv6(d).localSids['ff02::1'] = { behavior: 'end' };
      srv6(d).policies['fd00:4:bb::2']!['encapSource'] = '::1';
    });
    expect(Object.keys(got).sort()).toEqual([
      'routing.srv6-sid-address /routing/srv6/localSids/::',
      'routing.srv6-sid-address /routing/srv6/localSids/ff02::1',
      'routing.srv6-sid-address /routing/srv6/policies/fd00:4:bb::2/encapSource',
    ]);
  });

  it('routing.srv6-sid-unique: a local SID is not a binding SID', () => {
    expect(issues((d) => void (srv6(d).localSids['fd00:4:bb::1'] = { behavior: 'end' }))).toEqual({
      'routing.srv6-sid-unique /routing/srv6/localSids/fd00:4:bb::1':
        'fd00:4:bb::1 is also the binding SID of policy fd00:4:bb::1; a SID is either a local SID or a binding SID',
    });
  });

  it('routing.srv6-behavior-fields: required and forbidden fields per behaviour', () => {
    const got = issues((d) => {
      const l = srv6(d).localSids;
      l['fd00:4:ff::10'] = { behavior: 'end.x' }; // no interface, no next hop
      l['fd00:4:ff::11'] = { behavior: 'end.dx4', interface: 'host-w4l0', nextHop: 'fd00:4:1::2' }; // v6 next hop
      l['fd00:4:ff::12'] = { behavior: 'end.dt6' }; // no lookup VRF
      l['fd00:4:ff::13'] = { behavior: 'end', interface: 'host-w4l0', nextHop: 'fd00:4:1::2', lookupVrf: 'cust-a' };
      l['fd00:4:ff::14'] = { behavior: 'end.dt4', lookupVrf: 'cust-a', psp: true };
      l['fd00:4:ff::15'] = { behavior: 'end.dx2', interface: 'host-w4l1', nextHop: 'fd00:4:1::2' };
    });
    expect(got).toEqual({
      'routing.srv6-behavior-fields /routing/srv6/localSids/fd00:4:ff::10/interface': 'end.x needs an interface to cross-connect to',
      'routing.srv6-behavior-fields /routing/srv6/localSids/fd00:4:ff::10/nextHop': 'end.x needs a next hop',
      'routing.srv6-behavior-fields /routing/srv6/localSids/fd00:4:ff::11/nextHop': 'end.dx4 needs an IPv4 next hop',
      'routing.srv6-behavior-fields /routing/srv6/localSids/fd00:4:ff::12/lookupVrf': 'end.dt6 needs a lookup VRF',
      'routing.srv6-behavior-fields /routing/srv6/localSids/fd00:4:ff::13/interface':
        'end takes no interface (only end.x, end.dx2, end.dx4, end.dx6)',
      'routing.srv6-behavior-fields /routing/srv6/localSids/fd00:4:ff::13/nextHop': 'end takes no next hop (only end.x, end.dx4, end.dx6)',
      'routing.srv6-behavior-fields /routing/srv6/localSids/fd00:4:ff::13/lookupVrf':
        'end takes no lookup VRF (only end.t, end.dt4, end.dt6)',
      'routing.srv6-behavior-fields /routing/srv6/localSids/fd00:4:ff::14/psp': 'end.dt4 does not support PSP (only end, end.x, end.t)',
      'routing.srv6-behavior-fields /routing/srv6/localSids/fd00:4:ff::15/nextHop':
        'end.dx2 takes no next hop (only end.x, end.dx4, end.dx6)',
    });
  });

  it('routing.srv6-vrf-exists and routing.srv6-interface-exists', () => {
    const got = issues((d) => {
      srv6(d).localSids['fd00:4:ff::a']!['lookupVrf'] = 'nope';
      srv6(d).localSids['fd00:4:ff::1']!['vrf'] = 'nope';
      srv6(d).policies['fd00:4:bb::1']!['vrf'] = 'nope';
      srv6(d).steering[0]!['vrf'] = 'nope';
      srv6(d).localSids['fd00:4:ff::2']!['interface'] = 'loop9';
      srv6(d).steering[2]!['interface'] = 'loop8';
    });
    expect(Object.keys(got).sort()).toEqual([
      'routing.srv6-interface-exists /routing/srv6/localSids/fd00:4:ff::2/interface',
      'routing.srv6-interface-exists /routing/srv6/steering/2/interface',
      'routing.srv6-vrf-exists /routing/srv6/localSids/fd00:4:ff::1/vrf',
      'routing.srv6-vrf-exists /routing/srv6/localSids/fd00:4:ff::a/lookupVrf',
      'routing.srv6-vrf-exists /routing/srv6/policies/fd00:4:bb::1/vrf',
      'routing.srv6-vrf-exists /routing/srv6/steering/0/vrf',
    ]);
  });

  it('routing.srv6-encap-source (D-074): encap needs a source, insert takes none', () => {
    expect(issues((d) => void delete srv6(d).encapSource)).toEqual({
      'routing.srv6-encap-source /routing/srv6/policies/fd00:4:bb::1/encapSource':
        'an encapsulating policy needs an outer source address: set encapSource here or routing.srv6.encapSource (VPP’s global default cannot be read back, D-074)',
    });
    expect(issues((d) => void (srv6(d).policies['fd00:4:bb::3']!['encapSource'] = 'fd00:4::3'))).toEqual({
      'routing.srv6-encap-source /routing/srv6/policies/fd00:4:bb::3/encapSource':
        'an insert policy (encap off) has no outer header: remove encapSource',
    });
  });

  it('routing.srv6-steering-bsid, -steering-encap, -steering-unique', () => {
    const got = issues((d) => {
      const st = srv6(d).steering;
      st.push({ type: 'l3', prefix: '10.4.200.0/24', vrf: 'default', bsid: 'fd00:4:bb::99' }); // 3: no policy
      st.push({ type: 'l3', prefix: '10.4.201.0/24', vrf: 'default', bsid: 'fd00:4:bb::3' }); // 4: v4 into insert
      st.push({ type: 'l2', interface: 'host-w4l0', bsid: 'fd00:4:bb::3' }); // 5: l2 into insert (+ addresses)
      st.push({ type: 'l3', prefix: '10.4.100.0/24', vrf: 'cust-a', bsid: 'fd00:4:bb::2' }); // 6: duplicate of 0
      st.push({ type: 'l2', interface: 'host-w4l1', bsid: 'fd00:4:bb::1' }); // 7: duplicate of 2
    });
    expect(got).toEqual({
      'routing.srv6-steering-bsid /routing/srv6/steering/3/bsid': 'no policy with binding SID fd00:4:bb::99',
      'routing.srv6-steering-encap /routing/srv6/steering/4/bsid':
        'IPv4 steering needs an encapsulating policy; fd00:4:bb::3 inserts (encap off)',
      'routing.srv6-steering-encap /routing/srv6/steering/5/bsid':
        'L2 steering needs an encapsulating policy; fd00:4:bb::3 inserts (encap off)',
      'routing.srv6-l2-interface /routing/srv6/steering/5/interface':
        'interface host-w4l0 has IP addresses; L2 steering switches it to L2 mode — remove them first',
      'routing.srv6-steering-unique /routing/srv6/steering/6':
        "prefix 10.4.100.0/24 of VRF 'cust-a' is steered twice (first defined at /routing/srv6/steering/0)",
      'routing.srv6-steering-unique /routing/srv6/steering/7':
        'interface host-w4l1 is steered twice (first defined at /routing/srv6/steering/2)',
    });
  });

  it('registers every rule id once, under routing', () => {
    for (const v of srv6Validators) {
      expect(v.name).toMatch(/^routing\.srv6-/);
      expect(semanticRegistry.has(v.name)).toBe(true);
      expect(v.domains).toContain('routing');
    }
  });
});
