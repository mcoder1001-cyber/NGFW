import type { QosPolicerCounter, QosPolicerStatus } from '@ngfw/proto';

type Json = Record<string, unknown>;

/**
 * Burst the agent derives for a shaper without `burstBytes` (desired/qos.go, V3): ≈ 10 ms of traffic at the rate
 * (rateKbps × 1000 / 8 × 0.01 = rateKbps × 1.25 bytes), but at least two full-size (1500-byte) Ethernet frames — a
 * bucket smaller than one frame would drop every full-size packet. Must stay equal to the agent's value.
 */
export function shaperBurstBytes(rateKbps: number): number {
  return Math.max(Math.ceil((rateKbps * 5) / 4), 3000);
}

/** The VPP-side name of a shaper (the agent's egress policer `shaper:<name>`). */
export const SHAPER_PREFIX = 'shaper:';

export interface QosCounterOut {
  packets: string;
  bytes: string;
}

export interface QosAttachmentOut {
  interface: string;
  direction: 'input' | 'output';
}

export interface QosPolicerItem {
  name: string;
  kind: 'policer' | 'shaper';
  vppName: string;
  configured: boolean;
  present: boolean;
  index: number | null;
  type: string;
  rateUnit: string;
  cir: number;
  eir: number;
  cb: number;
  eb: number;
  bucket: { current: number; limit: number; extendedCurrent: number; extendedLimit: number } | null;
  conform: QosCounterOut;
  exceed: QosCounterOut;
  violate: QosCounterOut;
  attachments: QosAttachmentOut[];
}

const isObj = (v: unknown): v is Json => typeof v === 'object' && v !== null && !Array.isArray(v);
const rec = (v: unknown): Record<string, Json> =>
  isObj(v)
    ? (Object.fromEntries(Object.entries(v).filter(([, x]) => isObj(x))) as Record<string, Json>)
    : {};
const num = (v: unknown, d = 0): number => (typeof v === 'number' && Number.isFinite(v) ? v : d);
const str = (v: unknown, d: string): string => (typeof v === 'string' && v !== '' ? v : d);

function counter(c: QosPolicerCounter | undefined): QosCounterOut {
  return { packets: String(c?.packets ?? '0'), bytes: String(c?.bytes ?? '0') };
}

const ZERO: QosCounterOut = { packets: '0', bytes: '0' };

/** `services.qos` of a configuration document ({} when absent). */
export function qosOf(doc: Json): Json {
  const services = doc['services'];
  if (!isObj(services)) return {};
  const qos = services['qos'];
  return isObj(qos) ? qos : {};
}

/** Attachments of the running configuration, by VPP-side policer name. */
function attachmentsOf(qos: Json): Map<string, QosAttachmentOut[]> {
  const out = new Map<string, QosAttachmentOut[]>();
  const add = (vppName: string, a: QosAttachmentOut) =>
    out.set(vppName, [...(out.get(vppName) ?? []), a]);
  for (const [ifName, a] of Object.entries(rec(qos['interfaces'])).sort(([x], [y]) =>
    x.localeCompare(y),
  )) {
    const p = isObj(a['policer']) ? a['policer'] : {};
    if (typeof p['input'] === 'string') add(p['input'], { interface: ifName, direction: 'input' });
    if (typeof p['output'] === 'string')
      add(p['output'], { interface: ifName, direction: 'output' });
    if (typeof a['shaper'] === 'string') {
      add(SHAPER_PREFIX + a['shaper'], { interface: ifName, direction: 'output' });
    }
  }
  return out;
}

/**
 * `/state/services/qos/policers`: the agent's QosPolicerState joined with the running configuration — every running
 * policer and shaper (present or not) and every policer VPP reports for this owner (configured or not), sorted by kind
 * (policers first) then name. A configured item VPP lacks carries its configured parameters, `present: false`.
 */
export function joinPolicers(running: Json, live: readonly QosPolicerStatus[]): QosPolicerItem[] {
  const qos = qosOf(running);
  const attachments = attachmentsOf(qos);
  const liveBy = new Map(live.map((p) => [p.name, p]));
  const items = new Map<string, QosPolicerItem>();
  const fromLive = (p: QosPolicerStatus, configured: boolean): QosPolicerItem => {
    const shaper = p.kind === 'shaper' || p.name.startsWith(SHAPER_PREFIX);
    return {
      name: shaper ? p.name.slice(SHAPER_PREFIX.length) : p.name,
      kind: shaper ? 'shaper' : 'policer',
      vppName: p.name,
      configured,
      present: true,
      index: p.index,
      type: p.type,
      rateUnit: p.rateUnit,
      cir: p.cir,
      eir: p.eir,
      cb: Number(p.cb),
      eb: Number(p.eb),
      bucket: {
        current: p.currentBucket,
        limit: p.currentLimit,
        extendedCurrent: p.extendedBucket,
        extendedLimit: p.extendedLimit,
      },
      conform: counter(p.conform),
      exceed: counter(p.exceed),
      violate: counter(p.violate),
      attachments: attachments.get(p.name) ?? [],
    };
  };
  for (const [name, p] of Object.entries(rec(qos['policers']))) {
    const l = liveBy.get(name);
    items.set(
      name,
      l
        ? fromLive(l, true)
        : {
            name,
            kind: 'policer',
            vppName: name,
            configured: true,
            present: false,
            index: null,
            type: str(p['type'], '1r2c'),
            rateUnit: str(p['rateUnit'], 'kbps'),
            cir: num(p['cir']),
            eir: num(p['eir']),
            cb: num(p['cb']),
            eb: num(p['eb']),
            bucket: null,
            conform: ZERO,
            exceed: ZERO,
            violate: ZERO,
            attachments: attachments.get(name) ?? [],
          },
    );
  }
  for (const [name, s] of Object.entries(rec(qos['shapers']))) {
    const vppName = SHAPER_PREFIX + name;
    const l = liveBy.get(vppName);
    const rate = num(s['rateKbps']);
    items.set(
      vppName,
      l
        ? fromLive(l, true)
        : {
            name,
            kind: 'shaper',
            vppName,
            configured: true,
            present: false,
            index: null,
            type: '1r2c',
            rateUnit: 'kbps',
            cir: rate,
            eir: 0,
            cb: typeof s['burstBytes'] === 'number' ? s['burstBytes'] : shaperBurstBytes(rate),
            eb: 0,
            bucket: null,
            conform: ZERO,
            exceed: ZERO,
            violate: ZERO,
            attachments: attachments.get(vppName) ?? [],
          },
    );
  }
  for (const p of live) if (!items.has(p.name)) items.set(p.name, fromLive(p, false));
  return [...items.values()].sort((a, b) =>
    a.kind === b.kind ? a.name.localeCompare(b.name) : a.kind === 'policer' ? -1 : 1,
  );
}
