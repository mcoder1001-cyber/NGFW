import { describe, expect, it } from 'vitest';
import { RootConfig, type RootConfigInput } from '../index.js';
import { validateSemantics } from './index.js';

/**
 * F-vlan-qinq: stacked VLANs (802.1ad outer + 802.1Q inner, and 802.1Q-in-802.1Q) on sub-interfaces. The schema and
 * the rules already carry QinQ (`SubinterfaceSchema{vlanId, innerVlanId?, dot1ad}`, `interfaces.vlan-unique`,
 * `interfaces.subinterface-mtu`); these cases prove it. Every case runs the whole `interfaces` rule set (tier b) on a
 * schema-parsed document, as the commit engine does. Names follow the slot rig (`host-w5w0`, 10.5.0.0/16).
 */

const W = 'host-w5w0';
const SUBS = `/interfaces/${W}/subinterfaces`;

/** Schema (tier a) then every validator that reads `interfaces` (tier b). */
function check(doc: RootConfigInput) {
  const parsed = RootConfig.safeParse(doc);
  if (!parsed.success) return { schema: parsed.error.issues, semantic: [] };
  return { schema: [], semantic: validateSemantics(parsed.data, ['interfaces']) };
}

const qinqDoc = (subs: Record<string, unknown>, parentMtu?: number): RootConfigInput =>
  ({
    interfaces: {
      [W]: {
        enabled: true,
        ...(parentMtu === undefined ? {} : { mtu: parentMtu }),
        subinterfaces: subs,
      },
    },
  }) as RootConfigInput;

describe('QinQ sub-interfaces (F-vlan-qinq)', () => {
  it('accepts an 802.1ad outer tag with an 802.1Q inner tag and keeps the tag stack as written', () => {
    const doc = qinqDoc({
      '100': { vlanId: 100, enabled: true, ipv4: ['10.5.100.1/24'] },
      '200': {
        vlanId: 200,
        innerVlanId: 100,
        dot1ad: true,
        enabled: true,
        ipv4: ['10.5.200.1/24'],
      },
    });
    expect(check(doc)).toEqual({ schema: [], semantic: [] });
    const subs = RootConfig.parse(doc).interfaces[W]!.subinterfaces;
    expect(subs['200']).toMatchObject({ vlanId: 200, innerVlanId: 100, dot1ad: true });
    // dot1ad defaults to false (plain 802.1Q), and an absent inner tag stays absent (single-tag)
    expect(subs['100']).toMatchObject({ vlanId: 100, dot1ad: false });
    expect(subs['100']).not.toHaveProperty('innerVlanId');
  });

  it('accepts 802.1Q-in-802.1Q (two dot1q tags) next to a single-tag sub-interface on the same outer VLAN', () => {
    expect(
      check(
        qinqDoc({
          '300': { vlanId: 300 },
          '301': { vlanId: 300, innerVlanId: 1 },
          '302': { vlanId: 300, innerVlanId: 4094 },
        }),
      ),
    ).toEqual({ schema: [], semantic: [] });
  });

  it('accepts the same outer tag once as dot1q and once as dot1ad on one parent (distinct tag stacks)', () => {
    expect(
      check(
        qinqDoc({
          '200': { vlanId: 200, innerVlanId: 100 },
          '1200': { vlanId: 200, innerVlanId: 100, dot1ad: true },
          '201': { vlanId: 201 },
          '1201': { vlanId: 201, dot1ad: true },
        }),
      ),
    ).toEqual({ schema: [], semantic: [] });
  });

  it('accepts the same tag stack on two different parents', () => {
    const doc = {
      interfaces: {
        [W]: { subinterfaces: { '200': { vlanId: 200, innerVlanId: 100, dot1ad: true } } },
        'host-w5l0': { subinterfaces: { '200': { vlanId: 200, innerVlanId: 100, dot1ad: true } } },
      },
    } as RootConfigInput;
    expect(check(doc)).toEqual({ schema: [], semantic: [] });
  });

  it('rejects a duplicate (dot1ad, vlanId, innerVlanId) with the pointer to the second entry', () => {
    const { schema, semantic } = check(
      qinqDoc({
        '200': { vlanId: 200, innerVlanId: 100, dot1ad: true, ipv4: ['10.5.200.1/24'] },
        '201': { vlanId: 200, innerVlanId: 100, dot1ad: true, ipv4: ['10.5.201.1/24'] },
      }),
    );
    expect(schema).toEqual([]);
    expect(semantic).toEqual([
      {
        pointer: `${SUBS}/201/vlanId`,
        message: `VLAN dot1ad 200.100 is already used by sub-interface ${W}.200`,
      },
    ]);
  });

  it('rejects a duplicate dot1q-in-dot1q stack, and "second" follows the numeric order of the ids', () => {
    // JSON object keys that are integers iterate in ascending numeric order, whatever order they were written in
    const { semantic } = check(
      qinqDoc({ '310': { vlanId: 300, innerVlanId: 7 }, '30': { vlanId: 300, innerVlanId: 7 } }),
    );
    expect(semantic).toEqual([
      {
        pointer: `${SUBS}/310/vlanId`,
        message: `VLAN dot1q 300.7 is already used by sub-interface ${W}.30`,
      },
    ]);
  });

  it('rejects innerVlanId without vlanId (schema: the outer tag is required)', () => {
    const { schema } = check(qinqDoc({ '200': { innerVlanId: 100, dot1ad: true } }));
    expect(schema.map((i) => i.path.join('/'))).toEqual([
      `interfaces/${W}/subinterfaces/200/vlanId`,
    ]);
  });

  it('rejects out-of-range inner tags (1–4094, like the outer tag)', () => {
    for (const inner of [0, 4095, 1.5]) {
      const { schema } = check(qinqDoc({ '200': { vlanId: 200, innerVlanId: inner } }));
      expect(schema.map((i) => i.path.join('/'))).toEqual([
        `interfaces/${W}/subinterfaces/200/innerVlanId`,
      ]);
    }
  });

  it('rejects a QinQ sub-interface MTU above the parent MTU, with the pointer to the sub-interface MTU', () => {
    const { schema, semantic } = check(
      qinqDoc(
        {
          '200': { vlanId: 200, innerVlanId: 100, dot1ad: true, mtu: 1500 },
          '100': { vlanId: 100, mtu: 1400 },
        },
        1400,
      ),
    );
    expect(schema).toEqual([]);
    expect(semantic).toEqual([
      {
        pointer: `${SUBS}/200/mtu`,
        message: `sub-interface MTU 1500 exceeds the MTU 1400 of ${W}`,
      },
    ]);
  });
});
