import { ipFamily, parseCidr, parseIp } from '../../semantic/tunnels-common.js';

/**
 * F-global-blocking: the one parser of block-list files (uploads and server downloads) and the canonical form the
 * schema checks. Plain text, one entry per line: an IPv4/IPv6 address or prefix; `#` starts a comment; blank lines
 * are skipped. Entries are normalised (an address → /32 or /128, host bits masked, IPv6 lower-case RFC 5952),
 * deduplicated, and prefixes covered by another entry are collapsed into it.
 */

export interface InvalidLine {
  /** 1-based line number in the file. */
  line: number;
  text: string;
  reason: string;
}

export interface ParsedList {
  /** Canonical entries, IPv4 first, each family in address order, overlaps collapsed. */
  entries: string[];
  invalid: InvalidLine[];
  /** Entries whose host bits were masked (`10.0.0.1/24` → `10.0.0.0/24`). */
  normalised: number;
  /** Duplicates and entries covered by a wider one. */
  collapsed: number;
}

interface Range {
  v6: boolean;
  first: bigint;
  last: bigint;
  len: number;
}

function formatV4(n: bigint): string {
  return [24n, 16n, 8n, 0n].map((s) => String((n >> s) & 255n)).join('.');
}

/** RFC 5952: lower-case, leading zeros dropped, the longest run (≥ 2) of zero groups as `::`. */
function formatV6(n: bigint): string {
  const g: number[] = [];
  for (let i = 7; i >= 0; i--) g.push(Number((n >> BigInt(i * 16)) & 0xffffn));
  let best = -1;
  let bestLen = 0;
  for (let i = 0; i < 8; ) {
    if (g[i] !== 0) {
      i++;
      continue;
    }
    let j = i;
    while (j < 8 && g[j] === 0) j++;
    if (j - i > bestLen && j - i >= 2) {
      best = i;
      bestLen = j - i;
    }
    i = j;
  }
  const hex = g.map((x) => x.toString(16));
  if (best < 0) return hex.join(':');
  return `${hex.slice(0, best).join(':')}::${hex.slice(best + bestLen).join(':')}`;
}

/** Canonical text of a range. */
function text(r: Range): string {
  return `${r.v6 ? formatV6(r.first) : formatV4(r.first)}/${r.len}`;
}

/** One entry → its range and whether host bits were masked; or why it is not an entry. */
export function parseEntry(raw: string): { range: Range; masked: boolean } | { error: string } {
  const s = raw.trim();
  const withLen = s.includes('/') ? s : `${s}/${s.includes(':') ? 128 : 32}`;
  const slash = withLen.lastIndexOf('/');
  const addr = withLen.slice(0, slash);
  if (addr.includes('%')) return { error: 'zone indexes are not allowed' };
  if (parseIp(addr) === undefined) return { error: 'not an IPv4 or IPv6 address' };
  const c = parseCidr(withLen);
  if (c === undefined) return { error: 'bad prefix length' };
  return {
    range: { v6: ipFamily(addr) === 6, first: c.first, last: c.last, len: c.prefixLength },
    masked: c.address !== c.first,
  };
}

/** The canonical form of one entry (undefined when invalid). */
export function canonicalEntry(raw: string): string | undefined {
  const p = parseEntry(raw);
  return 'error' in p ? undefined : text(p.range);
}

/** Sort (IPv4 first, by first address, wider first) and drop duplicates and covered entries. */
function collapse(ranges: Range[]): { kept: Range[]; dropped: number } {
  ranges.sort((a, b) =>
    a.v6 !== b.v6 ? (a.v6 ? 1 : -1) : a.first < b.first ? -1 : a.first > b.first ? 1 : a.len - b.len,
  );
  const kept: Range[] = [];
  let cur: Range | undefined;
  for (const r of ranges) {
    if (cur && cur.v6 === r.v6 && r.last <= cur.last) continue; // inside the last kept (same start → wider first)
    kept.push(r);
    cur = r;
  }
  return { kept, dropped: ranges.length - kept.length };
}

/** Parse a block-list file (see the module comment). */
export function parseBlockList(file: string): ParsedList {
  const ranges: Range[] = [];
  const invalid: InvalidLine[] = [];
  let normalised = 0;
  file.split(/\r?\n/).forEach((rawLine, i) => {
    const line = rawLine.replace(/#.*$/, '').trim();
    if (line === '') return;
    const p = parseEntry(line);
    if ('error' in p) {
      invalid.push({ line: i + 1, text: rawLine.slice(0, 200), reason: p.error });
      return;
    }
    if (p.masked) normalised++;
    ranges.push(p.range);
  });
  const { kept, dropped } = collapse(ranges);
  return { entries: kept.map(text), invalid, normalised, collapsed: dropped };
}

/** Canonicalise and collapse an entry list (for the schema check and the diff of two lists). */
export function canonicalEntries(entries: readonly string[]): string[] {
  const ranges: Range[] = [];
  for (const e of entries) {
    const p = parseEntry(e);
    if (!('error' in p)) ranges.push(p.range);
  }
  return collapse(ranges).kept.map(text);
}

/** What changes from `before` to `after` (both canonical lists). */
export function diffEntries(before: readonly string[], after: readonly string[]): { added: string[]; removed: string[] } {
  const b = new Set(before);
  const a = new Set(after);
  return { added: after.filter((e) => !b.has(e)), removed: before.filter((e) => !a.has(e)) };
}
