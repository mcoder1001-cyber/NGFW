import { describe, expect, it } from 'vitest';
import { RootConfig, StaticRouteSchema, VrfSchema, type RootConfigInput } from '../index.js';
import { validateSemantics } from './index.js';
import { vrfStaticEcmpValidators } from './vrf-static-ecmp.js';

const run = (name: string, doc: RootConfigInput) =>
  vrfStaticEcmpValidators.find((v) => v.name === name)!.validate(RootConfig.parse(doc));

const IFACES: RootConfigInput['interfaces'] = {
  lan: { vrf: 'red', subinterfaces: { '100': { vlanId: 100 } } },
  wan: {},
};

describe('schema: the fields F-vrf-static-ecmp adds', () => {
  it('sourceSelect is optional; entries need a prefix (network address, not /0) and an interface', () => {
    expect(VrfSchema.parse({ id: 7 })).toEqual({ id: 7 });
    expect(
      VrfSchema.parse({ id: 7, sourceSelect: [{ prefix: '10.20.0.0/16', interface: 'lan' }] }),
    ).toEqual({ id: 7, sourceSelect: [{ prefix: '10.20.0.0/16', interface: 'lan' }] });
    for (const bad of [
      { prefix: '0.0.0.0/0', interface: 'lan' },
      { prefix: '::/0', interface: 'lan' },
      { prefix: '10.20.0.1/16', interface: 'lan' },
      { prefix: '10.20.0.0/16' },
      { prefix: '10.20.0.0/16', interface: 'lan', vrf: 'x' },
    ]) {
      expect(VrfSchema.safeParse({ id: 7, sourceSelect: [bad] }).success, JSON.stringify(bad)).toBe(false);
    }
  });

  it('nextHops[].vrf and viaFrr are optional and absent by default (existing documents parse unchanged)', () => {
    expect(
      StaticRouteSchema.parse({ prefix: '10.0.0.0/8', nextHops: [{ address: '10.1.1.1' }] }),
    ).toEqual({
      prefix: '10.0.0.0/8',
      vrf: 'default',
      nextHops: [{ address: '10.1.1.1', weight: 1 }],
      blackhole: false,
      distance: 1,
    });
    expect(
      StaticRouteSchema.parse({
        prefix: '10.0.0.0/8',
        viaFrr: true,
        nextHops: [{ address: '10.1.1.1', vrf: 'blue' }],
      }),
    ).toMatchObject({ viaFrr: true, nextHops: [{ address: '10.1.1.1', vrf: 'blue', weight: 1 }] });
    expect(StaticRouteSchema.safeParse({ prefix: '10.0.0.0/8', viaFrr: 'yes', blackhole: true }).success).toBe(false);
  });
});

describe('routing.vrf-static-ecmp-nexthop-vrf', () => {
  const doc = (hop: Record<string, unknown>, vrf = 'red'): RootConfigInput => ({
    interfaces: IFACES,
    vrfs: { red: { id: 2001 }, blue: { id: 2002 } },
    routing: { static: [{ prefix: '10.2.9.0/24', vrf, nextHops: [hop] }] },
  });

  it('accepts a next hop resolved in another declared VRF or in default', () => {
    expect(run('routing.vrf-static-ecmp-nexthop-vrf', doc({ address: '10.2.1.1', vrf: 'blue' }))).toEqual([]);
    expect(run('routing.vrf-static-ecmp-nexthop-vrf', doc({ address: '10.2.1.1', vrf: 'default' }))).toEqual([]);
    expect(run('routing.vrf-static-ecmp-nexthop-vrf', doc({ address: '10.2.1.1' }))).toEqual([]);
  });

  it('rejects an undeclared VRF, the route’s own VRF and a VRF on an interface next hop', () => {
    const at = '/routing/static/0/nextHops/0/vrf';
    expect(run('routing.vrf-static-ecmp-nexthop-vrf', doc({ address: '10.2.1.1', vrf: 'green' }))).toEqual([
      { pointer: at, message: "VRF 'green' does not exist" },
    ]);
    expect(run('routing.vrf-static-ecmp-nexthop-vrf', doc({ address: '10.2.1.1', vrf: 'red' }))[0]?.pointer).toBe(at);
    expect(
      run('routing.vrf-static-ecmp-nexthop-vrf', doc({ address: '10.2.1.1', vrf: 'default' }, 'default'))[0]
        ?.message,
    ).toContain("route's own VRF");
    expect(
      run('routing.vrf-static-ecmp-nexthop-vrf', doc({ address: '10.2.1.1', interface: 'lan', vrf: 'blue' }))[0]
        ?.message,
    ).toContain('egress interface');
  });
});

describe('routing.vrf-static-ecmp-single-path-weight', () => {
  it('accepts weights on ECMP routes and the default weight on single-path routes', () => {
    expect(
      run('routing.vrf-static-ecmp-single-path-weight', {
        routing: {
          static: [
            {
              prefix: '0.0.0.0/0',
              nextHops: [
                { address: '10.2.1.2', weight: 3 },
                { address: '10.2.1.3', weight: 1 },
              ],
            },
            { prefix: '10.2.9.0/24', nextHops: [{ address: '10.2.1.2' }] },
            { prefix: '10.2.10.0/24', blackhole: true },
          ],
        },
      }),
    ).toEqual([]);
  });

  it('rejects a weight other than 1 on a route with one next hop', () => {
    expect(
      run('routing.vrf-static-ecmp-single-path-weight', {
        routing: { static: [{ prefix: '10.2.9.0/24', nextHops: [{ address: '10.2.1.2', weight: 5 }] }] },
      }),
    ).toEqual([
      {
        pointer: '/routing/static/0/nextHops/0/weight',
        message:
          'weight 5 has no effect on a route with one next hop; weights split traffic between two or more next hops (ECMP)',
      },
    ]);
  });
});

describe('vrfs.vrf-static-ecmp-source-select-*', () => {
  const doc = (red: unknown[], blue: unknown[] = []): RootConfigInput =>
    ({
      interfaces: IFACES,
      vrfs: { red: { id: 2001, sourceSelect: red }, blue: { id: 2002, sourceSelect: blue } },
    }) as RootConfigInput;

  it('accepts entries on existing interfaces and sub-interfaces', () => {
    const d = doc([{ prefix: '10.20.0.0/16', interface: 'wan' }], [{ prefix: '10.20.0.0/16', interface: 'lan.100' }]);
    expect(run('vrfs.vrf-static-ecmp-source-select-interface-exists', d)).toEqual([]);
    expect(run('vrfs.vrf-static-ecmp-source-select-unique', d)).toEqual([]);
  });

  it('rejects an unknown interface', () => {
    expect(
      run('vrfs.vrf-static-ecmp-source-select-interface-exists', doc([{ prefix: '10.20.0.0/16', interface: 'dmz' }])),
    ).toEqual([{ pointer: '/vrfs/red/sourceSelect/0/interface', message: "interface 'dmz' does not exist" }]);
  });

  it('rejects the same (interface, source prefix) selecting two VRFs, whatever the spelling', () => {
    const issues = run(
      'vrfs.vrf-static-ecmp-source-select-unique',
      doc([{ prefix: '2001:db8:2::/48', interface: 'wan' }], [{ prefix: '2001:DB8:2:0::/48', interface: 'wan' }]),
    );
    expect(issues).toHaveLength(1);
    expect(issues[0]?.pointer).toBe('/vrfs/blue/sourceSelect/0/prefix');
    expect(issues[0]?.message).toContain('/vrfs/red/sourceSelect/0/prefix');
  });
});

describe('existing rules this feature relies on (reused, not re-added)', () => {
  it('vrfs.id-unique: two VRFs cannot share a table id', () => {
    const pointers = validateSemantics(RootConfig.parse({ vrfs: { red: { id: 2001 }, blue: { id: 2001 } } })).map(
      (i) => i.pointer,
    );
    expect(pointers).toEqual(['/vrfs/blue/id']);
  });

  it('routing.static-unique: one (vrf, prefix) is one route — ECMP paths go into its nextHops', () => {
    const issues = validateSemantics(
      RootConfig.parse({
        vrfs: { red: { id: 2001 } },
        routing: {
          static: [
            { prefix: '10.2.9.0/24', vrf: 'red', nextHops: [{ address: '10.2.1.2' }] },
            { prefix: '10.2.9.0/24', vrf: 'red', nextHops: [{ address: '10.2.1.3' }] },
            { prefix: '10.2.9.0/24', vrf: 'default', nextHops: [{ address: '10.2.1.3' }] },
          ],
        },
      }),
    );
    expect(issues.map((i) => i.pointer)).toEqual(['/routing/static/1/prefix']);
  });

  it('routing.vrf-exists: a route in an undeclared VRF is rejected', () => {
    const issues = validateSemantics(
      RootConfig.parse({ routing: { static: [{ prefix: '10.2.9.0/24', vrf: 'nope', blackhole: true }] } }),
    );
    expect(issues).toEqual([{ pointer: '/routing/static/0/vrf', message: "VRF 'nope' does not exist" }]);
  });
});
