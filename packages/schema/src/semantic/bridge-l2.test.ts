import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { RootConfig, type RootConfigInput } from '../index.js';
import { bridgeL2Validators } from './bridge-l2.js';
import { validateSemantics } from './index.js';

/**
 * F-bridge-l2 semantic rules. The valid corpus document is `packages/proto/test/fixtures/bridge-l2-full.json`
 * (the schema examples directory only admits the P02 groups' prefixes — F-bridge-l2-questions.md Q4).
 */
const fixture = JSON.parse(
  readFileSync(
    new URL('../../../proto/test/fixtures/bridge-l2-full.json', import.meta.url),
    'utf8',
  ),
) as RootConfigInput;

const run = (name: string, doc: RootConfigInput) => {
  const v = bridgeL2Validators.find((x) => x.name === name);
  if (v === undefined) throw new Error(`no validator ${name}`);
  return v.validate(RootConfig.parse(doc));
};

const pointers = (name: string, doc: RootConfigInput) => run(name, doc).map((i) => i.pointer);

/** A small valid base: bridge domain `lan` (id 7001) with a loopback BVI and one physical member. */
function base(): RootConfigInput & {
  interfaces: Record<string, unknown>;
  routing: { l2: Record<string, unknown> };
} {
  return {
    interfaces: {
      loop7000: { ipv4: ['10.7.0.1/24'], l2: { bridgeDomain: 'lan', bvi: true } },
      'GigabitEthernet0/0/0': { l2: { bridgeDomain: 'lan' } },
      'GigabitEthernet0/0/1': { subinterfaces: { '10': { vlanId: 10 } } },
    },
    routing: { l2: { bridgeDomains: { lan: { id: 7001 } } } },
  };
}

describe('bridge-l2 corpus', () => {
  it('the full fixture is schema- and semantically valid', () => {
    expect(validateSemantics(RootConfig.parse(fixture))).toEqual([]);
  });
  it('the base document is valid for every rule', () => {
    for (const v of bridgeL2Validators) expect(run(v.name, base())).toEqual([]);
  });
  it('validator names carry the owning domain and the slug', () => {
    for (const v of bridgeL2Validators) expect(v.name).toMatch(/^(interfaces|routing)\.bridge-l2-/);
  });
});

describe('schema-level rules', () => {
  it('rejects shg / mac-age out of range, a reversed MAC-filter range and tags that do not match the op', () => {
    const bad = (doc: RootConfigInput) => RootConfig.safeParse(doc).success;
    expect(bad({ interfaces: { loop1: { l2: { bridgeDomain: 'x', shg: 256 } } } })).toBe(false);
    expect(bad({ routing: { l2: { bridgeDomains: { x: { id: 1, macAgeMin: 256 } } } } })).toBe(
      false,
    );
    expect(bad({ routing: { l2: { bridgeDomains: { x: { id: 0 } } } } })).toBe(false);
    expect(bad({ routing: { l2: { bridgeDomains: { x: { id: 16777216 } } } } })).toBe(false);
    expect(
      bad({
        routing: {
          l2: {
            macFilters: {
              d: {
                mac: '02:00:00:00:00:01',
                ranges: [{ days: ['mon'], start: '10:00', end: '09:00' }],
              },
            },
          },
        },
      }),
    ).toBe(false);
    expect(
      bad({
        routing: {
          l2: {
            macFilters: {
              d: {
                mac: '02:00:00:00:00:01',
                ranges: [{ days: ['mon'], start: '25:00', end: '26:00' }],
              },
            },
          },
        },
      }),
    ).toBe(false);
    expect(bad({ interfaces: { x: { l2: { tagRewrite: { op: 'push-1' } } } } })).toBe(false);
    expect(bad({ interfaces: { x: { l2: { tagRewrite: { op: 'pop-1', tag1: 5 } } } } })).toBe(
      false,
    );
    expect(bad({ interfaces: { x: { l2: { tagRewrite: { op: 'push-2', tag1: 5 } } } } })).toBe(
      false,
    );
    expect(bad({ interfaces: { x: { l2: { tagRewrite: { op: 'pop-2', dot1ad: true } } } } })).toBe(
      false,
    );
    expect(
      bad({ interfaces: { x: { l2: { tagRewrite: { op: 'translate-1-2', tag1: 5, tag2: 6 } } } } }),
    ).toBe(true);
    expect(
      bad({
        routing: {
          l2: {
            macFilters: {
              d: {
                mac: '02:00:00:00:00:01',
                ranges: [{ days: ['mon'], start: '22:00', end: '24:00' }],
              },
            },
          },
        },
      }),
    ).toBe(true);
  });
});

describe('interfaces.bridge-l2-single-membership', () => {
  it('reports the second membership (the cross-connect) of a bridge member', () => {
    const doc = base();
    doc.routing.l2['xconnects'] = { 'GigabitEthernet0/0/0': { tx: 'GigabitEthernet0/0/1.10' } };
    const issues = run('interfaces.bridge-l2-single-membership', doc);
    expect(issues.map((i) => i.pointer)).toEqual(['/routing/l2/xconnects/GigabitEthernet0~10~10']);
    expect(issues[0]?.message).toContain("already a member of bridge domain 'lan'");
  });
  it('reports an L3 cross-connect on an L2 port', () => {
    const doc = base();
    doc.routing.l2['l3xc'] = { 'GigabitEthernet0/0/0': { ipv4Paths: [{ nextHop: '10.0.0.1' }] } };
    expect(pointers('interfaces.bridge-l2-single-membership', doc)).toEqual([
      '/routing/l2/l3xc/GigabitEthernet0~10~10',
    ]);
  });
  it('the full check reports it through validateSemantics as well (the API 400 path)', () => {
    const doc = base();
    doc.routing.l2['xconnects'] = { 'GigabitEthernet0/0/0': { tx: 'GigabitEthernet0/0/1.10' } };
    expect(validateSemantics(RootConfig.parse(doc)).map((i) => i.pointer)).toContain(
      '/routing/l2/xconnects/GigabitEthernet0~10~10',
    );
  });
});

describe('interfaces.bridge-l2-domain-exists', () => {
  it('rejects a membership in a bridge domain that is not declared', () => {
    const doc = base();
    (doc.interfaces['GigabitEthernet0/0/0'] as { l2: { bridgeDomain: string } }).l2.bridgeDomain =
      'nope';
    expect(pointers('interfaces.bridge-l2-domain-exists', doc)).toEqual([
      '/interfaces/GigabitEthernet0~10~10/l2/bridgeDomain',
    ]);
  });
});

describe('interfaces.bridge-l2-no-l3-when-bridged', () => {
  it('lets the BVI be routed and rejects addresses / VRF / DHCP on other L2 ports', () => {
    const doc = base();
    doc.interfaces['GigabitEthernet0/0/0'] = {
      ipv4: ['10.9.0.1/24'],
      vrf: 'blue',
      dhcpClient: {},
      l2: { bridgeDomain: 'lan' },
    };
    (doc as { vrfs?: unknown }).vrfs = { blue: { id: 7100 } };
    expect(pointers('interfaces.bridge-l2-no-l3-when-bridged', doc)).toEqual([
      '/interfaces/GigabitEthernet0~10~10/ipv4/0',
      '/interfaces/GigabitEthernet0~10~10/vrf',
      '/interfaces/GigabitEthernet0~10~10/dhcpClient',
    ]);
  });
  it('applies to an L2 cross-connect rx too', () => {
    const doc = base();
    doc.interfaces['GigabitEthernet0/0/2'] = { ipv6: ['2001:db8::1/64'] };
    doc.routing.l2['xconnects'] = { 'GigabitEthernet0/0/2': { tx: 'GigabitEthernet0/0/1.10' } };
    expect(pointers('interfaces.bridge-l2-no-l3-when-bridged', doc)).toEqual([
      '/interfaces/GigabitEthernet0~10~12/ipv6/0',
    ]);
  });
});

describe('interfaces.bridge-l2-port-role / one-bvi', () => {
  it('needs a bridge domain for shg / bvi / uuFwd', () => {
    expect(
      pointers('interfaces.bridge-l2-port-role', {
        interfaces: { x: { l2: { shg: 2, uuFwd: true } } },
      }),
    ).toEqual(['/interfaces/x/l2/shg', '/interfaces/x/l2/uuFwd']);
  });
  it('allows one BVI per bridge domain, on a loopback only', () => {
    const doc = base();
    doc.interfaces['loop7001'] = { l2: { bridgeDomain: 'lan', bvi: true } };
    doc.interfaces['GigabitEthernet0/0/1'] = {
      subinterfaces: {
        '10': { vlanId: 10, l2: { bridgeDomain: 'lan', uuFwd: true } },
        '11': { vlanId: 11, l2: { bridgeDomain: 'lan', uuFwd: true } },
      },
    };
    expect(pointers('interfaces.bridge-l2-one-bvi', doc)).toEqual([
      '/interfaces/loop7001/l2/bvi',
      '/interfaces/GigabitEthernet0~10~11/subinterfaces/11/l2/uuFwd',
    ]);
    const nonLoop = base();
    nonLoop.interfaces['loop7000'] = {};
    nonLoop.interfaces['GigabitEthernet0/0/0'] = { l2: { bridgeDomain: 'lan', bvi: true } };
    expect(pointers('interfaces.bridge-l2-one-bvi', nonLoop)).toEqual([
      '/interfaces/GigabitEthernet0~10~10/l2/bvi',
    ]);
  });
});

describe('interfaces.bridge-l2-tag-rewrite-l2-only', () => {
  it('accepts pop-1 on a bridged VLAN sub-interface and rejects a rewrite on an L3 port or too many pops', () => {
    const ok = base();
    ok.interfaces['GigabitEthernet0/0/1'] = {
      subinterfaces: {
        '10': { vlanId: 10, l2: { bridgeDomain: 'lan', tagRewrite: { op: 'pop-1' } } },
      },
    };
    expect(run('interfaces.bridge-l2-tag-rewrite-l2-only', ok)).toEqual([]);
    const l3 = base();
    l3.interfaces['GigabitEthernet0/0/1'] = {
      subinterfaces: { '10': { vlanId: 10, l2: { tagRewrite: { op: 'pop-1' } } } },
    };
    expect(pointers('interfaces.bridge-l2-tag-rewrite-l2-only', l3)).toEqual([
      '/interfaces/GigabitEthernet0~10~11/subinterfaces/10/l2/tagRewrite',
    ]);
    const pops = base();
    pops.interfaces['GigabitEthernet0/0/0'] = {
      l2: { bridgeDomain: 'lan', tagRewrite: { op: 'pop-1' } },
    };
    expect(pointers('interfaces.bridge-l2-tag-rewrite-l2-only', pops)).toEqual([
      '/interfaces/GigabitEthernet0~10~10/l2/tagRewrite/op',
    ]);
  });
});

describe('interfaces.bridge-l2-mac-filter-parent', () => {
  it('rejects the MAC filter on a sub-interface', () => {
    const doc = base();
    doc.interfaces['GigabitEthernet0/0/1'] = {
      subinterfaces: { '10': { vlanId: 10, l2: { macFilter: true } } },
    };
    expect(pointers('interfaces.bridge-l2-mac-filter-parent', doc)).toEqual([
      '/interfaces/GigabitEthernet0~10~11/subinterfaces/10/l2/macFilter',
    ]);
  });
});

describe('routing rules', () => {
  it('bridge-domain ids are unique', () => {
    const doc = base();
    doc.routing.l2['bridgeDomains'] = { lan: { id: 7001 }, dmz: { id: 7001 } };
    expect(pointers('routing.bridge-l2-domain-id-unique', doc)).toEqual([
      '/routing/l2/bridgeDomains/dmz/id',
    ]);
  });
  it('static MACs are unique and reached through a member', () => {
    const doc = base();
    doc.routing.l2['bridgeDomains'] = {
      lan: {
        id: 7001,
        staticMacs: [
          { mac: '02:00:00:00:00:01', interface: 'GigabitEthernet0/0/0' },
          { mac: '02:00:00:00:00:01', interface: 'GigabitEthernet0/0/0' },
          { mac: '02:00:00:00:00:02', interface: 'GigabitEthernet0/0/1' },
        ],
      },
    };
    expect(pointers('routing.bridge-l2-static-mac', doc)).toEqual([
      '/routing/l2/bridgeDomains/lan/staticMacs/1/mac',
      '/routing/l2/bridgeDomains/lan/staticMacs/2/interface',
    ]);
  });
  it('xconnect rx ≠ tx, both exist', () => {
    const doc = base();
    doc.routing.l2['xconnects'] = {
      'GigabitEthernet0/0/1.10': { tx: 'GigabitEthernet0/0/1.10' },
      ghost0: { tx: 'ghost1' },
    };
    expect(pointers('routing.bridge-l2-xconnect', doc)).toEqual([
      '/routing/l2/xconnects/GigabitEthernet0~10~11.10/tx',
      '/routing/l2/xconnects/ghost0',
      '/routing/l2/xconnects/ghost0/tx',
    ]);
  });
  it('l3xc paths: at least one, family, interface and VRF exist', () => {
    const doc = base();
    doc.routing.l2['l3xc'] = {
      'GigabitEthernet0/0/1.10': {
        ipv4Paths: [{ nextHop: '2001:db8::1' }, { interface: 'ghost', vrf: 'nope' }],
      },
      'GigabitEthernet0/0/1': {},
    };
    expect(pointers('routing.bridge-l2-l3xc', doc)).toEqual([
      '/routing/l2/l3xc/GigabitEthernet0~10~11.10/ipv4Paths/0/nextHop',
      '/routing/l2/l3xc/GigabitEthernet0~10~11.10/ipv4Paths/1/interface',
      '/routing/l2/l3xc/GigabitEthernet0~10~11.10/ipv4Paths/1/vrf',
      '/routing/l2/l3xc/GigabitEthernet0~10~11',
    ]);
  });
  it('MAC-filter MACs are unique and days are not repeated', () => {
    const doc = base();
    doc.routing.l2['macFilters'] = {
      a: { mac: '02:00:00:00:00:0a' },
      b: {
        mac: '02:00:00:00:00:0A',
        ranges: [{ days: ['mon', 'mon'], start: '08:00', end: '09:00' }],
      },
    };
    expect(pointers('routing.bridge-l2-mac-filter', doc)).toEqual([
      '/routing/l2/macFilters/b/mac',
      '/routing/l2/macFilters/b/ranges/0/days',
    ]);
  });
});
