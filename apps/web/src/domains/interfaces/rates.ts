import { useTopic } from '@ngfw/ui-kit/ws';
import { useRef, useState } from 'react';

/** One `iface.counters` entry as the API relays it (StreamStats; 64-bit counters are decimal strings, D-039). */
export interface WsCounter {
  name: string;
  rxPackets: string;
  rxBytes: string;
  txPackets: string;
  txBytes: string;
  errors?: string;
  drops?: string;
}
export interface WsCountersData {
  ts?: string;
  interfaces: WsCounter[];
}

export interface Rate {
  rxBps: number;
  txBps: number;
  rxPps: number;
  txPps: number;
  /** Recent rx+tx packet rates, oldest first (sparkline). */
  history: number[];
}

interface Sample {
  at: number;
  rxB: number;
  txB: number;
  rxP: number;
  txP: number;
}

export const HISTORY = 30;

/**
 * Rates from two consecutive absolute samples. A counter that went backwards (VPP restart, interface re-created)
 * yields no rate for that step instead of a negative or absurd one.
 */
export function stepRates(prev: Sample | undefined, cur: Sample): Omit<Rate, 'history'> | undefined {
  if (!prev) return undefined;
  const dt = (cur.at - prev.at) / 1000;
  if (dt <= 0) return undefined;
  const d = [cur.rxB - prev.rxB, cur.txB - prev.txB, cur.rxP - prev.rxP, cur.txP - prev.txP];
  if (d.some((x) => x < 0)) return undefined;
  return { rxBps: (d[0]! * 8) / dt, txBps: (d[1]! * 8) / dt, rxPps: d[2]! / dt, txPps: d[3]! / dt };
}

export function sampleOf(c: WsCounter, at: number): Sample {
  return { at, rxB: Number(c.rxBytes), txB: Number(c.txBytes), rxP: Number(c.rxPackets), txP: Number(c.txPackets) };
}

/** Folds one WS batch into the per-interface rate table (pure; keyed by VPP interface name). */
export function foldRates(
  last: Map<string, Sample>,
  rates: Map<string, Rate>,
  data: WsCountersData,
  now: number,
): Map<string, Rate> {
  const at = data.ts ? Date.parse(data.ts) : now;
  const next = new Map(rates);
  for (const c of data.interfaces ?? []) {
    const s = sampleOf(c, Number.isFinite(at) ? at : now);
    const r = stepRates(last.get(c.name), s);
    last.set(c.name, s);
    if (!r) continue;
    const history = [...(rates.get(c.name)?.history ?? []), r.rxPps + r.txPps].slice(-HISTORY);
    next.set(c.name, { ...r, history });
  }
  return next;
}

/** Live rates of every interface from the WS `iface.counters` topic (subscribed while the screen is mounted). */
export function useIfaceRates(): { rates: Map<string, Rate>; status: string } {
  const last = useRef(new Map<string, Sample>());
  const [rates, setRates] = useState(() => new Map<string, Rate>());
  const topic = useTopic<WsCountersData>('iface.counters', {
    onBatch: (batch) => {
      setRates((prev) => {
        let r = prev;
        for (const m of batch) r = foldRates(last.current, r, m.data, m.ts);
        return r;
      });
    },
  });
  return { rates, status: topic.status };
}
