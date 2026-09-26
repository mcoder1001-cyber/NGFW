import { describe, expect, it } from 'vitest';
import type { Rate } from '../../interfaces/rates';
import {
  byteParts,
  cpuLevel,
  engineThread,
  engineWording,
  hostSeries,
  levelOf,
  niceTicks,
  pushTraffic,
  severityColor,
  summarizeInterfaces,
  topInterfaces,
  totals,
  uptimeParts,
} from './model';

const rate = (rxBps: number, txBps: number, pps = 0): Rate => ({
  rxBps,
  txBps,
  rxPps: pps,
  txPps: 0,
  history: [],
});

describe('dashboard model', () => {
  it('sums per-interface rates', () => {
    const r = new Map([
      ['a', rate(1e6, 2e6, 10)],
      ['b', rate(3e6, 0, 5)],
    ]);
    expect(totals(r)).toEqual({ rxBps: 4e6, txBps: 2e6, pps: 15 });
  });

  it('appends traffic points, caps the window and never keeps two points at one instant', () => {
    const r = new Map([['a', rate(8, 16)]]);
    let pts = pushTraffic([], r, 1000, 3);
    pts = pushTraffic(pts, r, 2000, 3);
    pts = pushTraffic(pts, new Map([['a', rate(1, 1)]]), 2000, 3);
    expect(pts).toEqual([
      { at: 1000, rx: 8, tx: 16 },
      { at: 2000, rx: 1, tx: 1 },
    ]);
    pts = pushTraffic(pts, r, 3000, 3);
    pts = pushTraffic(pts, r, 4000, 3);
    expect(pts.map((p) => p.at)).toEqual([2000, 3000, 4000]);
    expect(pushTraffic(pts, new Map(), 5000, 3)).toBe(pts);
  });

  it('ranks the busiest interfaces by rx + tx', () => {
    const r = new Map([
      ['slow', rate(1, 1)],
      ['fast', rate(100, 50)],
      ['mid', rate(10, 10)],
    ]);
    expect(topInterfaces(r, 2).map((x) => x.name)).toEqual(['fast', 'mid']);
  });

  it('counts link states: admin down wins, no live state = missing', () => {
    expect(
      summarizeInterfaces([
        { name: 'a', state: { adminUp: true, linkUp: true } },
        { name: 'b', state: { adminUp: true, linkUp: false } },
        { name: 'c', state: { adminUp: false, linkUp: true } },
        { name: 'd', state: null },
      ]),
    ).toEqual({ total: 4, up: 1, down: 1, adminDown: 1, missing: 1 });
  });

  it('builds round axis ticks that cover the maximum', () => {
    expect(niceTicks(0)).toEqual([0, 1]);
    expect(niceTicks(95)).toEqual([0, 25, 50, 75, 100]);
    expect(niceTicks(1.2e9)).toEqual([0, 5e8, 1e9, 1.5e9]);
    const t = niceTicks(7_300);
    expect(t[0]).toBe(0);
    expect(t[t.length - 1]).toBeGreaterThanOrEqual(7_300);
  });

  it('maps severities and CPU levels', () => {
    expect(severityColor('ERROR')).toBe('error');
    expect(severityColor('warn')).toBe('warning');
    expect(severityColor('info')).toBe('info');
    expect(cpuLevel(20)).toBe('normal');
    expect(cpuLevel(75)).toBe('warning');
    expect(cpuLevel(95)).toBe('critical');
  });

  it('names engine threads without the engine product name', () => {
    expect(engineThread('vpp_main', 0)).toEqual({ kind: 'main' });
    expect(engineThread('vpp_wk_3', 4)).toEqual({ kind: 'worker', n: 3 });
    expect(engineThread('', 2)).toEqual({ kind: 'worker', n: 2 });
    expect(engineThread('custom', 5)).toEqual({ kind: 'other', name: 'custom' });
  });

  it('formats bytes, uptime, levels and host history', () => {
    expect(byteParts(512)).toEqual({ value: 512, unit: 'B' });
    expect(byteParts(1536)).toEqual({ value: 1.5, unit: 'KiB' });
    expect(byteParts(16 * 2 ** 30)).toEqual({ value: 16, unit: 'GiB' });
    expect(uptimeParts(90_061)).toEqual({ d: 1, h: 1, m: 1 });
    expect(levelOf(null)).toBe('normal');
    expect(levelOf(85, 80, 92)).toBe('warning');
    expect(levelOf(93, 80, 92)).toBe('critical');
    expect(hostSeries([{ at: 1, cpuPct: null, memUsedPct: 30 }])).toEqual([
      { at: 1, values: { cpu: null, mem: 30 } },
    ]);
    expect(engineWording('VPP connection lost (VPPX kept)', 'engine')).toBe(
      'engine connection lost (VPPX kept)',
    );
  });
});
