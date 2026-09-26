import { describe, expect, it } from 'vitest';
import i18n from '../../../i18n';
import en from '../../../locales/en/qos-flat.json';
import fa from '../../../locales/fa/qos-flat.json';
import {
  attachmentForm,
  attachmentRows,
  attachmentValue,
  burstFor,
  gridFromRow,
  gridsFromMap,
  localize,
  mapRows,
  nestPatch,
  parseCell,
  policerItemSchema,
  policerRows,
  rowFromGrid,
  rowsFromGrids,
  shaperBurstBytes,
  shaperItemSchema,
  shaperRows,
  type PolicerItem,
  type QosCfg,
} from './model';
import { QOS_POLL_MS } from './queries';

function keysOf(o: unknown, prefix = ''): string[] {
  if (typeof o !== 'object' || o === null) return [prefix];
  return Object.entries(o).flatMap(([k, v]) => keysOf(v, prefix ? `${prefix}.${k}` : k));
}

const zero = { packets: '0', bytes: '0' };
function item(p: Partial<PolicerItem> & { name: string }): PolicerItem {
  return {
    kind: 'policer',
    vppName: p.kind === 'shaper' ? `shaper:${p.name}` : p.name,
    configured: true,
    present: true,
    index: 0,
    type: '1r2c',
    rateUnit: 'kbps',
    cir: 1000,
    eir: 0,
    cb: 1250,
    eb: 0,
    bucket: { current: 1, limit: 1, extendedCurrent: 0, extendedLimit: 0 },
    conform: zero,
    exceed: zero,
    violate: zero,
    attachments: [],
    ...p,
  };
}

describe('qos-flat model', () => {
  it('en and fa have identical key sets', () => {
    expect(keysOf(fa).sort()).toEqual(keysOf(en).sort());
  });

  it('never polls the policer state faster than every 30 s (D-132)', () => {
    expect(QOS_POLL_MS).toBeGreaterThanOrEqual(30_000);
  });

  it('derives the shaper burst like the agent: ≈ 10 ms of the rate, at least 3000 bytes', () => {
    expect(shaperBurstBytes(1000)).toBe(3000); // 1250 B would drop every full-size frame
    expect(shaperBurstBytes(50_000)).toBe(62_500);
    expect(shaperBurstBytes(2401)).toBe(3002); // rounded up
  });

  it('rate/burst helper: bytes for kbit/s, packets for packets/s, rounded up', () => {
    expect(burstFor(10_000, 'kbps', 10)).toBe(12_500);
    expect(burstFor(1000, 'pps', 50)).toBe(50);
    expect(burstFor(3, 'kbps', 1)).toBe(1);
    expect(burstFor(0, 'kbps', 10)).toBe(0);
    expect(burstFor(Number.NaN, 'pps', 10)).toBe(0);
  });

  it('map grids: rows ↔ one cell per recorded value of the source, sorted, empty cells unlisted', () => {
    const grid = gridFromRow('ip', [
      { from: 46, to: 34 },
      { from: 8, to: 0 },
      { from: 99, to: 1 }, // outside DSCP: dropped
    ]);
    expect(grid).toHaveLength(64);
    expect(grid[46]).toBe(34);
    expect(grid[8]).toBe(0);
    expect(grid[0]).toBeNull();
    expect(rowFromGrid(grid)).toEqual([
      { from: 8, to: 0 },
      { from: 46, to: 34 },
    ]);
    expect(gridFromRow('vlan', undefined)).toHaveLength(8);
    expect(gridFromRow('ext', [])).toHaveLength(256);
    const grids = gridsFromMap({ rows: { ip: [{ from: 46, to: 34 }], vlan: [] } });
    grids.mpls[5] = 3;
    expect(rowsFromGrids(grids)).toEqual({ ip: [{ from: 46, to: 34 }], mpls: [{ from: 5, to: 3 }] });
  });

  it('parses grid cells: empty = unlisted, 0–255, anything else invalid', () => {
    expect(parseCell('')).toBeNull();
    expect(parseCell(' 34 ')).toBe(34);
    expect(parseCell('255')).toBe(255);
    expect(parseCell('256')).toBeUndefined();
    expect(parseCell('-1')).toBeUndefined();
    expect(parseCell('x')).toBeUndefined();
  });

  const qos: QosCfg = {
    policers: { gold: { type: '2r3c-rfc2698', cir: 20_000, eir: 40_000, cb: 25_000, eb: 50_000 }, fresh: { cir: 5, cb: 1 } },
    shapers: { uplink: { rateKbps: 50_000 }, backup: { rateKbps: 2000, burstBytes: 4000 } },
    maps: { remark: { id: 1001, rows: { ip: [{ from: 46, to: 34 }], vlan: [{ from: 5, to: 3 }] } }, pcp: { rows: { ip: [] } } },
    interfaces: {
      loop1002: { policer: { output: 'gold' }, mark: { map: 'remark', output: 'ip' } },
      loop1001: { policer: { input: 'gold' }, shaper: 'uplink', record: 'vlan', store: { source: 'ip', value: 0 } },
    },
  };

  it('policer rows join the candidate with the live state (applied, not committed, not in VPP, unmanaged)', () => {
    const rows = policerRows(qos, [
      item({ name: 'gold', type: '2r3c-rfc2698', cir: 20_000, eir: 40_000, conform: { packets: '12', bytes: '900' } }),
      item({ name: 'ghost', configured: false }),
      item({ name: 'uplink', kind: 'shaper' }),
    ]);
    expect(rows.map((r) => [r.name, r.status])).toEqual([
      ['fresh', 'pending'],
      ['ghost', 'unmanaged'],
      ['gold', 'applied'],
    ]);
    expect(rows[2]).toMatchObject({ cir: 20_000, eir: 40_000, cb: 25_000, eb: 50_000 });
    expect(rows[1]?.cfg).toBeUndefined();
    expect(policerRows(qos, [item({ name: 'gold', present: false, index: null })])[1]?.status).toBe('missing');
  });

  it('rate-limit rows show the configured or the derived burst', () => {
    const rows = shaperRows(qos, [item({ name: 'uplink', kind: 'shaper', cir: 50_000, cb: 62_500 })]);
    expect(rows.map((r) => [r.name, r.status, r.burstBytes, r.derivedBurst])).toEqual([
      ['backup', 'pending', 4000, 3000],
      ['uplink', 'applied', undefined, 62_500],
    ]);
  });

  it('map rows count entries per source and list the interfaces marking with them', () => {
    expect(mapRows(qos).map((r) => [r.name, r.id, r.entries, r.usedBy])).toEqual([
      ['pcp', undefined, {}, []],
      ['remark', 1001, { ip: 1, vlan: 1 }, ['loop1002']],
    ]);
  });

  it('attachment rows are sorted; the form round-trips the document and drops what is off', () => {
    expect(attachmentRows(qos).map((r) => r.name)).toEqual(['loop1001', 'loop1002']);
    for (const a of Object.values(qos.interfaces ?? {})) {
      expect(attachmentValue(attachmentForm(a))).toEqual(a);
    }
    const f = attachmentForm(undefined);
    expect(f).toMatchObject({ egress: 'none', storeOn: false, storeSource: 'ip', markOutput: 'ip' });
    expect(attachmentValue({ ...f, input: 'gold', egress: 'shaper', shaper: 'uplink', output: 'gold' })).toEqual({
      policer: { input: 'gold' },
      shaper: 'uplink',
    });
    // a store value that is not an integer goes to the server as typed (it answers with a pointer)
    expect(attachmentValue({ ...f, storeOn: true, storeValue: 'x' })).toEqual({ store: { source: 'ip', value: 'x' } });
  });

  it('merge patches nest below services', () => {
    expect(nestPatch(['qos', 'policers', 'gold'], null)).toEqual({ qos: { policers: { gold: null } } });
  });

  it('forms come from the one schema and are localized', () => {
    const p = localize(policerItemSchema(), (k, o) => i18n.t(k, { ...o, ns: 'qos-flat' }), 'policer');
    const props = p.properties as Record<string, { title?: string }>;
    expect(Object.keys(props)).toEqual(
      expect.arrayContaining(['type', 'rateUnit', 'cir', 'eir', 'cb', 'eb', 'round', 'colorAware', 'conformAction']),
    );
    expect(props['cir']?.title).toBe('Committed information rate (CIR)');
    const s = localize(shaperItemSchema(), (k, o) => i18n.t(k, { ...o, ns: 'qos-flat', lng: 'fa' }), 'shaper');
    expect((s.properties as Record<string, { title?: string }>)['rateKbps']?.title).toBe('نرخ (kbit/s)');
  });
});
