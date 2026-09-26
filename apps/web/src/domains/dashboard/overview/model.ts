import type { PaletteMode } from '@mui/material/styles';
import type { Rate } from '../../interfaces/rates';

/**
 * Series colours of the dashboard charts (WEB-dashboard). Receive = slot 1 (blue), transmit = slot 2 (orange) of the
 * validated categorical order; checked against this theme's `background.paper` in both modes (lightness band, chroma,
 * CVD ΔE ≥ 24, normal-vision ΔE ≥ 31, ≥ 3:1 contrast). The theme's primary/secondary pair fails those checks, so charts
 * do not use it. Text never wears these colours (labels stay in text tokens).
 */
export const SERIES: Record<PaletteMode, { rx: string; tx: string }> = {
  light: { rx: '#2a78d6', tx: '#eb6834' },
  dark: { rx: '#3987e5', tx: '#d95926' },
};

/** One point of the aggregate traffic chart: total receive / transmit bit rate of every interface at `at` (ms). */
export interface TrafficPoint {
  at: number;
  rx: number;
  tx: number;
}

/** Points kept for the traffic chart (the WS topic flushes at most once per second → about two minutes). */
export const TRAFFIC_POINTS = 120;

/** Sum of the per-interface rates, in bit/s and packets/s. */
export function totals(rates: ReadonlyMap<string, Rate>): {
  rxBps: number;
  txBps: number;
  pps: number;
} {
  let rxBps = 0;
  let txBps = 0;
  let pps = 0;
  for (const r of rates.values()) {
    rxBps += r.rxBps;
    txBps += r.txBps;
    pps += r.rxPps + r.txPps;
  }
  return { rxBps, txBps, pps };
}

/** Append the current totals as one point (pure); keeps the last `max` points, never two points at one instant. */
export function pushTraffic(
  points: readonly TrafficPoint[],
  rates: ReadonlyMap<string, Rate>,
  at: number,
  max = TRAFFIC_POINTS,
): TrafficPoint[] {
  if (rates.size === 0) return points as TrafficPoint[];
  const { rxBps, txBps } = totals(rates);
  const kept =
    points.length > 0 && points[points.length - 1]!.at >= at ? points.slice(0, -1) : points;
  return [...kept, { at, rx: rxBps, tx: txBps }].slice(-max);
}

/** Interfaces ranked by total bit rate (rx + tx), busiest first. */
export function topInterfaces(
  rates: ReadonlyMap<string, Rate>,
  n = 5,
): { name: string; rate: Rate; bps: number }[] {
  return [...rates.entries()]
    .map(([name, rate]) => ({ name, rate, bps: rate.rxBps + rate.txBps }))
    .sort((a, b) => b.bps - a.bps || a.name.localeCompare(b.name))
    .slice(0, n);
}

/** The part of a `/state/interfaces` item the dashboard reads. */
export interface InterfaceLike {
  name: string;
  state: { adminUp?: boolean; linkUp?: boolean } | null;
}

export interface InterfaceSummary {
  total: number;
  up: number;
  down: number;
  adminDown: number;
  /** Configured but not (yet) present on the data plane. */
  missing: number;
}

/** Link states counted on the interfaces tile, in display order. */
export const LINK_STATES = ['up', 'down', 'adminDown'] as const;

/** Link-state counts: admin down wins over link state; no live state = missing. */
export function summarizeInterfaces(items: readonly InterfaceLike[]): InterfaceSummary {
  const s: InterfaceSummary = { total: items.length, up: 0, down: 0, adminDown: 0, missing: 0 };
  for (const i of items) {
    if (!i.state) s.missing += 1;
    else if (i.state.adminUp === false) s.adminDown += 1;
    else if (i.state.linkUp) s.up += 1;
    else s.down += 1;
  }
  return s;
}

/** Three to six round tick values from 0 up to at least `max` (1-2-2.5-5 steps), for a y axis. */
export function niceTicks(max: number, target = 4): number[] {
  if (!(max > 0) || !Number.isFinite(max)) return [0, 1];
  const raw = max / target;
  const mag = 10 ** Math.floor(Math.log10(raw));
  const step = ([1, 2, 2.5, 5, 10].find((m) => m * mag >= raw) ?? 10) * mag;
  const ticks: number[] = [];
  for (let v = 0; v < max + step * 1e-9; v += step) ticks.push(Number(v.toPrecision(12)));
  if (ticks[ticks.length - 1]! < max) ticks.push(Number((ticks.length * step).toPrecision(12)));
  return ticks;
}

/** Event severity → the MUI colour the list uses (with an icon and the text; never colour alone). */
export function severityColor(severity: string): 'error' | 'warning' | 'info' | 'success' {
  switch (severity.toLowerCase()) {
    case 'critical':
    case 'error':
    case 'err':
      return 'error';
    case 'warning':
    case 'warn':
      return 'warning';
    case 'ok':
    case 'success':
      return 'success';
    default:
      return 'info';
  }
}

/** Health pill states of the banner. */
export const HEALTH = { ok: 'ok', warn: 'warn', down: 'down' } as const;
export type Health = (typeof HEALTH)[keyof typeof HEALTH];

/** Load level of a percentage: normal below `warn`, warning below `crit`, critical from `crit`. */
export function levelOf(
  pct: number | null | undefined,
  warn = 70,
  crit = 90,
): 'normal' | 'warning' | 'critical' {
  if (pct === null || pct === undefined) return 'normal';
  if (pct >= crit) return 'critical';
  if (pct >= warn) return 'warning';
  return 'normal';
}

/** Engine thread CPU level (same thresholds as the host). */
export function cpuLevel(pct: number): 'normal' | 'warning' | 'critical' {
  return levelOf(pct);
}

/**
 * Display identity of a packet-engine thread without the engine's product name: the main thread, or worker <n>
 * (`vpp_main` → main, `vpp_wk_0` → worker 0). Unknown names are kept as the engine reports them.
 */
export function engineThread(
  name: string,
  worker: number,
): { kind: 'main' } | { kind: 'worker'; n: number } | { kind: 'other'; name: string } {
  if (/(^|_)main$/.test(name) || (name === '' && worker === 0)) return { kind: 'main' };
  const m = /(?:^|_)wk_(\d+)$/.exec(name);
  if (m) return { kind: 'worker', n: Number(m[1]) };
  return name === '' ? { kind: 'worker', n: worker } : { kind: 'other', name };
}

export type ByteUnit = 'B' | 'KiB' | 'MiB' | 'GiB' | 'TiB';

/** 1536 → { value: 1.5, unit: 'KiB' } (binary units, as memory and disks are sized). */
export function byteParts(n: number): { value: number; unit: ByteUnit } {
  const units: ByteUnit[] = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let v = Math.max(0, n);
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i += 1;
  }
  return { value: v, unit: units[i]! };
}

/** Seconds → whole days, hours and minutes. */
export function uptimeParts(sec: number): { d: number; h: number; m: number } {
  const s = Math.max(0, Math.floor(sec));
  return {
    d: Math.floor(s / 86_400),
    h: Math.floor((s % 86_400) / 3_600),
    m: Math.floor((s % 3_600) / 60),
  };
}

/** Traffic points → time-chart points. */
export function trafficSeries(
  points: readonly TrafficPoint[],
): { at: number; values: Record<string, number | null> }[] {
  return points.map((p) => ({ at: p.at, values: { rx: p.rx, tx: p.tx } }));
}

/** Host history (`/state/host`) → time-chart points (CPU %, memory %). */
export function hostSeries(
  history: readonly { at: number; cpuPct: number | null; memUsedPct: number }[],
): { at: number; values: Record<string, number | null> }[] {
  return history.map((h) => ({ at: h.at, values: { cpu: h.cpuPct, mem: h.memUsedPct } }));
}

/** Host series colours: CPU = slot 1 (blue), memory = slot 3 (aqua) — validated with the traffic pair on both surfaces. */
export const HOST_SERIES: Record<PaletteMode, { cpu: string; mem: string; disk: string }> = {
  light: { cpu: '#2a78d6', mem: '#1baf7a', disk: '#eb6834' },
  dark: { cpu: '#3987e5', mem: '#199e70', disk: '#d95926' },
};

/** System event codes the dashboard shows as translated text (`events.codes.<code>`); others keep the server's message. */
export const EVENT_CODES = new Set([
  'VPP_CONNECTED',
  'VPP_DISCONNECTED',
  'DEGRADED',
  'COMMIT_APPLIED',
  'COMMIT_CONFIRMED',
  'CONFIRM_REVERTED',
  'RUNNING_IN_SYNC',
]);

/** A server message without the engine's product name (the product calls it the engine). */
export function engineWording(message: string, engine: string): string {
  return message.replace(/\bVPP\b/g, engine);
}
