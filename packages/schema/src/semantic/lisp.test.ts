import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { RootConfig } from '../index.js';
import { validateSemantics } from './index.js';
import { canonicalEid, lispValidators } from './lisp.js';

/** The full example (also the proto drift corpus): packages/proto/test/fixtures/lisp-full.json. */
const FULL = JSON.parse(
  readFileSync(new URL('../../../proto/test/fixtures/lisp-full.json', import.meta.url), 'utf8'),
) as { tunnels: { lisp: Record<string, unknown> } } & Record<string, unknown>;

const withLisp = (patch: Record<string, unknown>) =>
  structuredClone({ ...FULL, tunnels: { lisp: { ...FULL.tunnels.lisp, ...patch } } });

const run = (doc: unknown) => {
  const parsed = RootConfig.safeParse(doc);
  expect(parsed.error?.issues).toBeUndefined();
  return validateSemantics(parsed.data!, ['tunnels']);
};

const pointers = (doc: unknown) => run(doc).map((i) => i.pointer);

describe('tunnels.lisp', () => {
  it('the full example is accepted and semantically clean', () => {
    expect(run(FULL)).toEqual([]);
  });

  it('absent lisp is valid; an empty lisp object is valid', () => {
    expect(run({})).toEqual([]);
    expect(run({ tunnels: { lisp: {} } })).toEqual([]);
  });

  it('validator names are prefixed tunnels.lisp- and unique', () => {
    for (const v of lispValidators) expect(v.name.startsWith('tunnels.lisp-')).toBe(true);
    expect(new Set(lispValidators.map((v) => v.name)).size).toBe(lispValidators.length);
  });

  it('canonical EIDs', () => {
    expect(canonicalEid('10.1.2.3/24')).toBe('10.1.2.0/24');
    expect(canonicalEid('02:0B:00:00:00:01')).toBe('02:0b:00:00:00:01');
    expect(canonicalEid('fd11:0::/64')).toBe('fd11::/64');
    expect(
      pointers(
        withLisp({
          localEids: [{ vni: 1100, eid: '10.11.100.1/24', locatorSet: 'w11-rloc' }],
          adjacencies: [],
        }),
      ),
    ).toContain('/tunnels/lisp/localEids/0/eid');
  });

  it('duplicate EID in one VNI → pointer at the second', () => {
    const doc = withLisp({
      localEids: [
        { vni: 1100, eid: '10.11.100.0/24', locatorSet: 'w11-rloc' },
        { vni: 1100, eid: '10.11.100.0/24', locatorSet: 'w11-rloc' },
      ],
    });
    expect(run(doc)).toContainEqual({
      pointer: '/tunnels/lisp/localEids/1/eid',
      message: 'EID 10.11.100.0/24 in VNI 1100 is already configured at /tunnels/lisp/localEids/0',
    });
  });

  it('the same EID in two VNIs is fine; local vs remote clash in one VNI is not', () => {
    expect(
      pointers(
        withLisp({
          remoteMappings: [{ vni: 1100, eid: '10.11.100.0/24', rlocs: [] }],
          adjacencies: [],
        }),
      ),
    ).toContain('/tunnels/lisp/remoteMappings/0/eid');
  });

  it('locator interface and locator set must exist', () => {
    const doc = withLisp({
      locatorSets: { 'w11-rloc': { locators: [{ interface: 'nope0' }] } },
      localEids: [{ vni: 1100, eid: '10.11.100.0/24', locatorSet: 'other' }],
      adjacencies: [],
    });
    expect(pointers(doc)).toEqual(
      expect.arrayContaining([
        '/tunnels/lisp/locatorSets/w11-rloc/locators/0/interface',
        '/tunnels/lisp/localEids/0/locatorSet',
      ]),
    );
  });

  it('RLOC family consistent', () => {
    const doc = withLisp({
      remoteMappings: [
        {
          vni: 1100,
          eid: '10.11.200.0/24',
          rlocs: [{ address: '10.11.1.2' }, { address: 'fd00::2' }],
        },
      ],
    });
    expect(pointers(doc)).toContain('/tunnels/lisp/remoteMappings/0/rlocs');
  });

  it('each VNI with local EIDs needs an eid-table mapping of the right kind', () => {
    expect(pointers(withLisp({ eidTables: {}, remoteMappings: [], adjacencies: [] }))).toContain(
      '/tunnels/lisp/localEids/0/vni',
    );
    const l2 = withLisp({
      localEids: [{ vni: 1100, eid: '02:0b:00:00:00:01', locatorSet: 'w11-rloc' }],
      remoteMappings: [],
      adjacencies: [],
    });
    expect(pointers(l2)).toContain('/tunnels/lisp/localEids/0/vni');
  });

  it('eidTables entry needs exactly one of vrf / bridgeDomain, and the VRF exists', () => {
    expect(
      pointers(withLisp({ eidTables: { '1100': { vrf: 'overlay', bridgeDomain: 5 } } })),
    ).toContain('/tunnels/lisp/eidTables/1100');
    expect(pointers(withLisp({ eidTables: { '1100': { vrf: 'ghost' } } }))).toContain(
      '/tunnels/lisp/eidTables/1100/vrf',
    );
  });

  it('gpe requires enabled; objects require enabled', () => {
    const p = pointers(withLisp({ enabled: false }));
    expect(p).toEqual(expect.arrayContaining(['/tunnels/lisp/gpe', '/tunnels/lisp/enabled']));
  });

  it('gpe entries require gpe', () => {
    expect(pointers(withLisp({ gpe: false }))).toContain('/tunnels/lisp/gpe');
  });

  it('adjacency endpoints must be configured', () => {
    const doc = withLisp({
      adjacencies: [{ vni: 1100, reid: '10.11.250.0/24', leid: '10.11.100.0/24' }],
    });
    expect(pointers(doc)).toContain('/tunnels/lisp/adjacencies/0/reid');
  });

  it('schema rejects a bad EID shape and an out-of-range VNI', () => {
    expect(
      RootConfig.safeParse(withLisp({ localEids: [{ vni: 1, eid: 'x', locatorSet: 'a' }] }))
        .success,
    ).toBe(false);
    expect(
      RootConfig.safeParse(
        withLisp({ localEids: [{ vni: 2 ** 24, eid: '10.0.0.0/8', locatorSet: 'a' }] }),
      ).success,
    ).toBe(false);
  });
});
