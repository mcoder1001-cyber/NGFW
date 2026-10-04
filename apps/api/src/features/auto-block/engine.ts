import type { AutoBlockSourceKind } from '@ngfw/schema';

/**
 * F-bruteforce-block: the pure detection and decision logic, with no Nest, DB or clock of its own — the time is
 * always passed in, so it is fully deterministic and unit-tested. The Nest service (auto-block.service.ts) owns the
 * config, the persisted block set and the wiring to login/agent events.
 *
 *  - `SlidingWindows` counts failures per (detector, source) inside a moving window and says when a threshold trips.
 *  - `blockDurationSec` turns an offence count into a block time (doubling per offence, capped) — the escalation.
 *  - the IP helpers below match a source against the allow-list (a source inside any allow-list prefix, or loopback,
 *    is never blocked) and canonicalise a bare address to a /32 or /128 so the block set keys on one form.
 */

export type Family = 4 | 6;

export interface ParsedPrefix {
  family: Family;
  first: bigint;
  last: bigint;
}

function parseIpv4(addr: string): bigint | undefined {
  const parts = addr.split('.');
  if (parts.length !== 4) return undefined;
  let v = 0n;
  for (const p of parts) {
    if (!/^\d{1,3}$/.test(p)) return undefined;
    const n = Number(p);
    if (n > 255) return undefined;
    v = (v << 8n) | BigInt(n);
  }
  return v;
}

function parseIpv6(addr: string): bigint | undefined {
  let text = addr;
  // an embedded IPv4 tail (e.g. ::ffff:192.0.2.1) → two hex groups
  const lastColon = text.lastIndexOf(':');
  if (lastColon >= 0 && text.slice(lastColon + 1).includes('.')) {
    const v4 = parseIpv4(text.slice(lastColon + 1));
    if (v4 === undefined) return undefined;
    const hi = (v4 >> 16n) & 0xffffn;
    const lo = v4 & 0xffffn;
    text = `${text.slice(0, lastColon + 1)}${hi.toString(16)}:${lo.toString(16)}`;
  }
  const halves = text.split('::');
  if (halves.length > 2) return undefined;
  const head = halves[0] === '' ? [] : halves[0]!.split(':');
  const tail = halves.length === 2 ? (halves[1] === '' ? [] : halves[1]!.split(':')) : null;
  const groups =
    tail === null ? head : [...head, ...Array(8 - head.length - tail.length).fill('0'), ...tail];
  if (tail === null && groups.length !== 8) return undefined;
  if (tail !== null && head.length + tail.length > 7) return undefined;
  if (groups.length !== 8) return undefined;
  let v = 0n;
  for (const g of groups) {
    if (!/^[0-9a-fA-F]{1,4}$/.test(g)) return undefined;
    v = (v << 16n) | BigInt(parseInt(g, 16));
  }
  return v;
}

/** Parse an address to its family and numeric value, or undefined when it is not an IP. */
export function parseIp(addr: string): { family: Family; value: bigint } | undefined {
  if (addr.includes(':')) {
    const v = parseIpv6(addr);
    return v === undefined ? undefined : { family: 6, value: v };
  }
  const v = parseIpv4(addr);
  return v === undefined ? undefined : { family: 4, value: v };
}

/** Parse `addr` or `addr/len` into a prefix range. A bare address becomes a /32 or /128. */
export function parsePrefix(entry: string): ParsedPrefix | undefined {
  const slash = entry.lastIndexOf('/');
  const addrText = slash < 0 ? entry : entry.slice(0, slash);
  const ip = parseIp(addrText);
  if (ip === undefined) return undefined;
  const bits = ip.family === 4 ? 32 : 128;
  const len = slash < 0 ? bits : Number(entry.slice(slash + 1));
  if (!Number.isInteger(len) || len < 0 || len > bits) return undefined;
  const hostBits = BigInt(bits - len);
  const first = (ip.value >> hostBits) << hostBits;
  const last = first | ((1n << hostBits) - 1n);
  return { family: ip.family, first, last };
}

/** The canonical block key for a source address: `addr/32` or `addr/128` (the source is always a single host). */
export function canonicalSource(addr: string): string | undefined {
  let ip = parseIp(addr);
  if (ip === undefined) return undefined;
  if (ip.family === 6 && ip.value >> 32n === 0xffffn)
    ip = { family: 4, value: ip.value & 0xffffffffn };
  return `${formatIp(ip.family, ip.value)}/${ip.family === 4 ? 32 : 128}`;
}

function formatIp(family: Family, value: bigint): string {
  if (family === 4) {
    return [24n, 16n, 8n, 0n].map((s) => String((value >> s) & 255n)).join('.');
  }
  const groups: number[] = [];
  for (let i = 7; i >= 0; i--) groups.push(Number((value >> BigInt(i * 16)) & 0xffffn));
  // RFC 5952: compress the longest run (>= 2) of zero groups
  let best = -1;
  let bestLen = 0;
  for (let i = 0; i < 8;) {
    if (groups[i] !== 0) {
      i++;
      continue;
    }
    let j = i;
    while (j < 8 && groups[j] === 0) j++;
    if (j - i > bestLen && j - i >= 2) {
      best = i;
      bestLen = j - i;
    }
    i = j;
  }
  const hex = groups.map((g) => g.toString(16));
  if (best < 0) return hex.join(':');
  return `${hex.slice(0, best).join(':')}::${hex.slice(best + bestLen).join(':')}`;
}

/** Loopback ranges that are always allow-listed regardless of config (127.0.0.0/8 and ::1). */
const IMPLICIT_ALLOW: readonly ParsedPrefix[] = [
  parsePrefix('127.0.0.0/8')!,
  parsePrefix('::1/128')!,
];

/** Compile an allow-list of address/prefix strings into ranges, dropping any that will not parse. */
export function compileAllowlist(entries: readonly string[]): ParsedPrefix[] {
  const out: ParsedPrefix[] = [...IMPLICIT_ALLOW];
  for (const e of entries) {
    const p = parsePrefix(e);
    if (p !== undefined) {
      out.push(p);
      if (p.family === 6 && p.first >> 32n === 0xffffn && p.last >> 32n === 0xffffn)
        out.push({ family: 4, first: p.first & 0xffffffffn, last: p.last & 0xffffffffn });
    }
  }
  return out;
}

/** True when the source address falls inside any allow-list prefix (of the same family). */
export function isAllowlisted(allow: readonly ParsedPrefix[], source: string): boolean {
  let ip = parseIp(source);
  if (ip === undefined) return false;
  if (ip.family === 6 && ip.value >> 32n === 0xffffn)
    ip = { family: 4, value: ip.value & 0xffffffffn };
  return allow.some((p) => p.family === ip.family && ip.value >= p.first && ip.value <= p.last);
}

/** The block time for an offence: `blockSec` for the first, doubling each repeat, capped at `maxBlockSec`. */
export function blockDurationSec(
  offence: number,
  rule: { blockSec: number; escalate: boolean; maxBlockSec: number },
): number {
  if (!rule.escalate || offence <= 1) return Math.min(rule.blockSec, rule.maxBlockSec);
  // 2^(offence-1) without overflow: cap the shift, then clamp to maxBlockSec
  const factor = offence - 1 >= 30 ? 2 ** 30 : 2 ** (offence - 1);
  const secs = rule.blockSec * factor;
  return Number.isFinite(secs) ? Math.min(secs, rule.maxBlockSec) : rule.maxBlockSec;
}

/**
 * Per-(detector, source) sliding window of failure timestamps (ms). `observe` records one failure and returns the
 * number of failures still inside the window; `tripped` is true once that reaches the threshold. Memory is bounded:
 * a window is dropped once empty, and each window keeps at most `threshold` timestamps.
 */
export class SlidingWindows {
  private readonly hits = new Map<string, number[]>();

  private key(kind: AutoBlockSourceKind, source: string): string {
    return `${kind}\u0000${source}`;
  }

  observe(
    kind: AutoBlockSourceKind,
    source: string,
    nowMs: number,
    windowSec: number,
    threshold: number,
  ): { count: number; tripped: boolean } {
    const key = this.key(kind, source);
    const cutoff = nowMs - windowSec * 1000;
    const arr = (this.hits.get(key) ?? []).filter((t) => t > cutoff);
    arr.push(nowMs);
    // keep at most `threshold` most-recent samples: older ones can never change the decision
    if (arr.length > threshold) arr.splice(0, arr.length - threshold);
    this.hits.set(key, arr);
    return { count: arr.length, tripped: arr.length >= threshold };
  }

  /** Forget a source after it has been blocked, so the count starts fresh when the block lifts. */
  clear(kind: AutoBlockSourceKind, source: string): void {
    this.hits.delete(this.key(kind, source));
  }

  /** Drop windows whose newest sample is older than `nowMs - maxAgeMs` (called on the expiry sweep). */
  prune(nowMs: number, maxAgeMs: number): void {
    for (const [key, arr] of this.hits) {
      const last = arr[arr.length - 1];
      if (last === undefined || last <= nowMs - maxAgeMs) this.hits.delete(key);
    }
  }

  get size(): number {
    return this.hits.size;
  }
}

/** Fixed detector budgets independent of the configurable blocked-entry cap. */
export const MAX_SCAN_PORTS_PER_SOURCE = 4096;
const MAX_SCAN_PORT_OBSERVATIONS = 100_000;
const MAX_SCAN_SOURCES = 10_000;

/** Repeated ports refresh rather than adding hits; exhausted budgets discard evidence. */
export class DistinctPortWindows {
  private readonly sources = new Map<string, Map<number, number>>();
  private entries = 0;
  private lastCapacityPrune = -Infinity;

  observe(
    source: string,
    port: number,
    nowMs: number,
    windowSec: number,
    threshold: number,
    maxSources: number,
  ): { count: number; tripped: boolean } {
    if (threshold > MAX_SCAN_PORTS_PER_SOURCE) return { count: 0, tripped: false };
    const cutoff = nowMs - windowSec * 1000;
    let ports = this.sources.get(source);
    if (ports !== undefined) {
      for (const [seenPort, at] of ports)
        if (at <= cutoff) {
          ports.delete(seenPort);
          this.entries--;
        }
    }
    // Reclaim globally at most once per second when capacity is exhausted.
    // Ordinary traffic scans only its own bounded window.
    if (
      (ports === undefined && this.sources.size >= Math.min(maxSources, MAX_SCAN_SOURCES)) ||
      this.entries >= MAX_SCAN_PORT_OBSERVATIONS
    ) {
      if (nowMs >= this.lastCapacityPrune + 1000) {
        this.prune(cutoff);
        this.lastCapacityPrune = nowMs;
        ports = this.sources.get(source);
      }
    }
    if (ports === undefined) {
      if (
        this.sources.size >= Math.min(maxSources, MAX_SCAN_SOURCES) ||
        this.entries >= MAX_SCAN_PORT_OBSERVATIONS
      )
        return { count: 0, tripped: false };
      ports = new Map<number, number>();
      this.sources.set(source, ports);
    }
    if (!ports.has(port)) {
      if (
        ports.size >= Math.min(threshold, MAX_SCAN_PORTS_PER_SOURCE) ||
        this.entries >= MAX_SCAN_PORT_OBSERVATIONS
      )
        return { count: ports.size, tripped: false };
      this.entries++;
    }
    ports.set(port, nowMs);
    return { count: ports.size, tripped: ports.size >= threshold };
  }

  clear(source: string): void {
    this.entries -= this.sources.get(source)?.size ?? 0;
    this.sources.delete(source);
  }

  prune(cutoffMs: number): void {
    for (const [source, ports] of this.sources) {
      for (const [port, at] of ports)
        if (at <= cutoffMs) {
          ports.delete(port);
          this.entries--;
        }
      if (ports.size === 0) this.sources.delete(source);
    }
  }

  get size(): number {
    return this.sources.size;
  }
  get observationCount(): number {
    return this.entries;
  }
}
