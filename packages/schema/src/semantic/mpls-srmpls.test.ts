import { describe, expect, it } from 'vitest';
import { MplsSchema, RoutingSchema } from '../index.js';
import type { RootConfigInput } from '../index.js';
import { validateConfig } from '../validate.js';

/**
 * F-mpls-srmpls: `routing.mpls` schema and the `routing.mpls-srmpls-…` rules. Documents go through `validateConfig`
 * (schema → semantic), exactly what the API runs before a commit, so every expectation is a pointer the 400
 * problem+json carries.
 */

type Mpls = NonNullable<NonNullable<RootConfigInput['routing']>['mpls']>;

const base = (mpls: Mpls, extra: Partial<RootConfigInput> = {}): RootConfigInput => ({
  interfaces: { loop5001: {}, 'host-w5l0': {} },
  vrfs: { red: { id: 5010 } },
  routing: { mpls },
  ...extra,
});

/** Pointers of the issues validateConfig reports (schema or semantic tier). */
function pointers(doc: RootConfigInput): { tier?: string; pointers: string[] } {
  const r = validateConfig(doc);
  if (r.ok) return { pointers: [] };
  return { tier: r.tier, pointers: r.issues.map((i) => i.pointer) };
}

const route = (label: number, extra: Record<string, unknown> = {}) => ({
  label,
  paths: [{ nextHop: '10.5.1.2', interface: 'loop5001', outLabels: [50017] }],
  ...extra,
});

const FULL: Mpls = {
  interfaces: ['loop5001', 'host-w5l0'],
  tables: { '5001': {} },
  labelRoutes: [
    route(50016, { table: 5001 }),
    { label: 50020, eos: false, paths: [{ interface: 't1', outLabels: [50021] }] },
    { label: 50030, payload: 'ip6', paths: [{ vrf: 'red' }] },
  ],
  ipBindings: [{ label: 50040, vrf: 'red', prefix: '10.5.40.0/24' }],
  tunnels: { t1: { paths: [{ nextHop: '10.5.1.2', interface: 'loop5001', outLabels: [50050] }] } },
  sr: {
    policies: {
      '50100': { segmentLists: [{ labels: [50101, 50102] }, { labels: [50103], weight: 2 }] },
    },
    steering: [{ prefix: '10.5.60.0/24', bsid: 50100, vpnLabel: 50061 }],
  },
};

describe('routing.mpls schema', () => {
  it('accepts the full model and fills the defaults', () => {
    const r = validateConfig(base(FULL));
    expect(r.ok ? [] : r.issues).toEqual([]);
    const m = r.ok ? r.config.routing.mpls : undefined;
    expect(m?.labelRoutes[0]).toMatchObject({
      table: 5001,
      eos: true,
      paths: [{ weight: 1, outLabels: [50017] }],
    });
    expect(m?.labelRoutes[2]?.table).toBe(0);
    expect(m?.ipBindings[0]?.vrf).toBe('red');
    expect(m?.sr.steering[0]?.vrf).toBe('default');
    expect(m?.sr.policies['50100']?.spray).toBe(false);
    expect(m?.tunnels['t1']?.l2Only).toBe(false);
  });

  it('is absent unless configured, and `{}` is an empty MPLS section', () => {
    expect(RoutingSchema.parse({}).mpls).toBeUndefined();
    expect(RoutingSchema.parse({ mpls: {} }).mpls).toEqual({
      interfaces: [],
      tables: {},
      labelRoutes: [],
      ipBindings: [],
      tunnels: {},
      sr: { policies: {}, steering: [] },
    });
  });

  it('stays a plain object that F-mpls-ldp extends by one key (no union, no record)', () => {
    expect(Object.keys(MplsSchema.shape)).toEqual([
      'interfaces',
      'tables',
      'labelRoutes',
      'ipBindings',
      'tunnels',
      'sr',
    ]);
  });

  it('label 5 (reserved) → pointer at the label', () => {
    expect(pointers(base({ labelRoutes: [route(5)] }))).toEqual({
      tier: 'schema',
      pointers: ['/routing/mpls/labelRoutes/0/label'],
    });
  });

  it('labels above 20 bits, reserved out labels, BSIDs and segments are rejected with their pointers', () => {
    const doc = base({
      labelRoutes: [
        route(1048576),
        { label: 50016, paths: [{ interface: 'loop5001', outLabels: [3] }] },
      ],
      sr: {
        policies: {
          '15': { segmentLists: [{ labels: [50101] }] },
          '50100': { segmentLists: [{ labels: [7] }] },
        },
      },
    });
    expect(pointers(doc)).toEqual({
      tier: 'schema',
      pointers: [
        '/routing/mpls/labelRoutes/0/label',
        '/routing/mpls/labelRoutes/1/paths/0/outLabels/0',
        '/routing/mpls/sr/policies/15',
        '/routing/mpls/sr/policies/50100/segmentLists/0/labels/0',
      ],
    });
  });

  it('an out-label stack has at most 16 labels', () => {
    const stack = Array.from({ length: 17 }, (_, i) => 50100 + i);
    expect(
      pointers(
        base({
          labelRoutes: [{ label: 50016, paths: [{ interface: 'loop5001', outLabels: stack }] }],
        }),
      ).pointers,
    ).toEqual(['/routing/mpls/labelRoutes/0/paths/0/outLabels']);
    expect(
      pointers(
        base({
          labelRoutes: [
            { label: 50016, paths: [{ interface: 'loop5001', outLabels: stack.slice(1) }] },
          ],
        }),
      ).pointers,
    ).toEqual([]);
  });

  it('payload and lookup-VRF paths only on end-of-stack routes; one next-hop family per route', () => {
    expect(
      pointers(base({ labelRoutes: [route(50016, { eos: false, payload: 'ip4' })] })).pointers,
    ).toEqual(['/routing/mpls/labelRoutes/0/payload']);
    expect(
      pointers(base({ labelRoutes: [{ label: 50016, eos: false, paths: [{ vrf: 'red' }] }] }))
        .pointers,
    ).toEqual(['/routing/mpls/labelRoutes/0/paths']);
    expect(
      pointers(
        base({ labelRoutes: [{ label: 50016, payload: 'ethernet', paths: [{ vrf: 'red' }] }] }),
      ).pointers,
    ).toEqual(['/routing/mpls/labelRoutes/0/payload']);
    expect(
      pointers(
        base({
          labelRoutes: [
            { label: 50016, paths: [{ nextHop: '10.5.1.2' }, { nextHop: '2001:db8::2' }] },
          ],
        }),
      ).pointers,
    ).toEqual(['/routing/mpls/labelRoutes/0/paths']);
    expect(
      pointers(
        base({ labelRoutes: [{ label: 50016, paths: [{ vrf: 'red', nextHop: '10.5.1.2' }] }] }),
      ).pointers,
    ).toEqual(['/routing/mpls/labelRoutes/0/paths/0/vrf']);
    expect(
      pointers(base({ labelRoutes: [{ label: 50016, paths: [{ outLabels: [50017] }] }] })).pointers,
    ).toEqual(['/routing/mpls/labelRoutes/0/paths/0/nextHop']);
  });

  it('table ids and tunnel names are validated as record keys', () => {
    expect(pointers(base({ tables: { '0': {} } })).pointers).toEqual(['/routing/mpls/tables/0']);
    expect(pointers(base({ tables: { '4294967296': {} } })).pointers).toEqual([
      '/routing/mpls/tables/4294967296',
    ]);
    expect(
      pointers(base({ tunnels: { 'a/b': { paths: [{ interface: 'loop5001' }] } } })).pointers,
    ).toEqual(['/routing/mpls/tunnels/a~1b']);
  });
});

describe('routing.mpls-srmpls rules', () => {
  it('(table, label, eos) is unique; eos and non-eos of one label may coexist', () => {
    const doc = base({
      labelRoutes: [
        route(50016),
        route(50016, { eos: false }),
        route(50016),
        route(50016, { table: 5001 }),
      ],
      tables: { '5001': {} },
    });
    expect(pointers(doc)).toEqual({
      tier: 'semantic',
      pointers: ['/routing/mpls/labelRoutes/2/label'],
    });
    const r = validateConfig(doc);
    expect(r.ok ? '' : r.issues[0]?.message).toMatch(
      /label 50016 \(end of stack\) already has a route in MPLS table 0 \(first defined at \/routing\/mpls\/labelRoutes\/0\/label\)/,
    );
  });

  it('a label route table must be 0 or declared', () => {
    expect(pointers(base({ labelRoutes: [route(50016, { table: 5002 })] })).pointers).toEqual([
      '/routing/mpls/labelRoutes/0/table',
    ]);
  });

  it('MPLS interfaces exist and are listed once', () => {
    expect(pointers(base({ interfaces: ['loop5001', 'nope', 'loop5001'] })).pointers).toEqual([
      '/routing/mpls/interfaces/1',
      '/routing/mpls/interfaces/2',
    ]);
  });

  it('path interfaces name an interface or another MPLS tunnel', () => {
    const doc = base({
      labelRoutes: [
        { label: 50016, paths: [{ interface: 'ghost' }] },
        { label: 50017, paths: [{ interface: 't1' }] },
      ],
      tunnels: {
        t1: { paths: [{ interface: 't1' }] },
        t2: { paths: [{ interface: 't1', outLabels: [50018] }] },
      },
    });
    expect(pointers(doc).pointers).toEqual([
      '/routing/mpls/labelRoutes/0/paths/0/interface',
      '/routing/mpls/tunnels/t1/paths/0/interface',
    ]);
  });

  it('VRFs of lookup paths, bindings and steering exist', () => {
    const doc = base({
      labelRoutes: [{ label: 50016, paths: [{ vrf: 'blue' }] }],
      ipBindings: [{ label: 50040, vrf: 'blue', prefix: '10.5.40.0/24' }],
      sr: {
        policies: { '50100': { segmentLists: [{ labels: [50101] }] } },
        steering: [{ vrf: 'blue', prefix: '10.5.60.0/24', bsid: 50100 }],
      },
    });
    expect(pointers(doc).pointers).toEqual([
      '/routing/mpls/ipBindings/0/vrf',
      '/routing/mpls/labelRoutes/0/paths/0/vrf',
      '/routing/mpls/sr/steering/0/vrf',
    ]);
  });

  it('a tunnel name is not an interface name', () => {
    expect(
      pointers(base({ tunnels: { loop5001: { paths: [{ nextHop: '10.5.1.2' }] } } })).pointers,
    ).toEqual(['/routing/mpls/tunnels/loop5001']);
  });

  it('a BSID is not used as a static label route of table 0 (another table is fine)', () => {
    const sr = { policies: { '50100': { segmentLists: [{ labels: [50101] }] } } };
    expect(pointers(base({ labelRoutes: [route(50100)], sr })).pointers).toEqual([
      '/routing/mpls/labelRoutes/0/label',
    ]);
    expect(pointers(base({ labelRoutes: [route(50100, { eos: false })], sr })).pointers).toEqual([
      '/routing/mpls/labelRoutes/0/label',
    ]);
    expect(
      pointers(base({ labelRoutes: [route(50100, { table: 5001 })], tables: { '5001': {} }, sr }))
        .pointers,
    ).toEqual([]);
  });

  it('a bound label is not a table-0 route, a BSID or another binding; one label per prefix', () => {
    const doc = base({
      labelRoutes: [route(50016)],
      ipBindings: [
        { label: 50016, prefix: '10.5.1.0/24' },
        { label: 50100, prefix: '10.5.2.0/24' },
        { label: 50041, prefix: '10.5.3.0/24' },
        { label: 50041, prefix: '10.5.4.0/24' },
        { label: 50042, prefix: '10.5.3.0/24' },
      ],
      sr: { policies: { '50100': { segmentLists: [{ labels: [50101] }] } } },
    });
    expect(pointers(doc).pointers).toEqual([
      '/routing/mpls/ipBindings/0/label',
      '/routing/mpls/ipBindings/1/label',
      '/routing/mpls/ipBindings/3/label',
      '/routing/mpls/ipBindings/4/prefix',
    ]);
  });

  it('steering names an existing policy and a (vrf, prefix) once', () => {
    const doc = base({
      sr: {
        policies: { '50100': { segmentLists: [{ labels: [50101] }] } },
        steering: [
          { prefix: '10.5.60.0/24', bsid: 50100 },
          { prefix: '10.5.61.0/24', bsid: 50200 },
          { vrf: 'default', prefix: '10.5.60.0/24', bsid: 50100 },
        ],
      },
    });
    expect(pointers(doc).pointers).toEqual([
      '/routing/mpls/sr/steering/1/bsid',
      '/routing/mpls/sr/steering/2/prefix',
    ]);
  });

  it('segment lists of one policy are distinct', () => {
    const doc = base({
      sr: {
        policies: {
          '50100': {
            segmentLists: [{ labels: [50101, 50102] }, { labels: [50101, 50102], weight: 3 }],
          },
        },
      },
    });
    expect(pointers(doc).pointers).toEqual([
      '/routing/mpls/sr/policies/50100/segmentLists/1/labels',
    ]);
  });
});
