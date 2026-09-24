import { describe, expect, it } from 'vitest';
import { localizeSchema, pathsText, routeRows, routeStatus, staticRouteSchema, vrfItemSchema, vrfRows } from './model';

describe('vrf-static-ecmp model', () => {
  it('reads the VRF and static-route item schemas from the one schema (sourceSelect, nextHops[].vrf, viaFrr)', () => {
    expect(Object.keys((vrfItemSchema().properties ?? {}) as object)).toEqual(expect.arrayContaining(['id', 'description', 'sourceSelect']));
    const route = staticRouteSchema();
    expect(Object.keys((route.properties ?? {}) as object)).toEqual(expect.arrayContaining(['prefix', 'vrf', 'nextHops', 'blackhole', 'distance', 'viaFrr']));
    const hop = ((route.properties as Record<string, { items?: { properties?: object } }>)['nextHops']?.items?.properties ?? {}) as object;
    expect(Object.keys(hop)).toEqual(expect.arrayContaining(['address', 'interface', 'weight', 'vrf']));
  });

  it('shows ECMP weights, next-hop VRFs and blackholes', () => {
    const rows = routeRows([
      { prefix: '0.0.0.0/0', vrf: 'red', blackhole: false, distance: 1, nextHops: [{ address: '10.2.1.2', weight: 3 }, { address: '10.2.1.3', weight: 1 }] },
      { prefix: '10.2.60.0/24', vrf: 'red', blackhole: false, distance: 1, nextHops: [{ address: '10.2.2.2', weight: 1, vrf: 'default' }] },
      { prefix: '10.2.70.0/24', vrf: 'blue', blackhole: true, distance: 1, nextHops: [] },
      { prefix: '10.2.80.0/24', vrf: 'default', blackhole: false, distance: 1, nextHops: [{ interface: 'lan', weight: 1 }], viaFrr: true },
    ]);
    expect(rows.map((r) => r.paths)).toEqual(['10.2.1.2 ×3 · 10.2.1.3 ×1', '10.2.2.2 in default', '', 'via lan']);
    expect(rows.map((r) => r.ecmp)).toEqual([true, false, false, false]);
    const installed = new Map([['red', new Set(['0.0.0.0/0'])], ['blue', new Set<string>()]]);
    expect(rows.map((r) => routeStatus(r, installed))).toEqual(['installed', 'missing', 'missing', 'frr']);
    expect(pathsText({ nextHops: [] })).toBe('');
  });

  it('lists default even when it is not declared and sorts by table id', () => {
    expect(vrfRows({ red: { id: 2001, sourceSelect: [{ prefix: '10.2.50.0/24', interface: 'wan' }] } }).map((r) => [r.name, r.tableId, r.sourceSelect])).toEqual([
      ['default', 0, 0],
      ['red', 2001, 1],
    ]);
  });

  it('localizes nested titles (array items included) and keeps the English fallback', () => {
    const t = (k: string, o?: Record<string, unknown>) => (k === 'field.sourceSelect.prefix.title' ? 'پیشوند مبدأ' : String(o?.['defaultValue'] ?? ''));
    const s = localizeSchema(vrfItemSchema(), t) as { properties: Record<string, { title?: string; items?: { properties: Record<string, { title?: string }> } }> };
    expect(s.properties['sourceSelect']?.items?.properties['prefix']?.title).toBe('پیشوند مبدأ');
    expect(s.properties['id']?.title).toBe('Table ID');
  });
});
