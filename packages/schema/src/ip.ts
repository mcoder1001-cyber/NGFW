/**
 * Pure IP address / prefix arithmetic shared by the primitives (host-bits checks) and the semantic validators
 * (subnet overlap, address-family consistency). Inputs normally passed the Zod primitives first; every function
 * still returns `undefined` / `false` on malformed input instead of throwing, so callers never crash on a
 * document that slipped past structural validation.
 */

export type IpFamily = 4 | 6;

/** A parsed prefix: `address` is the full 32/128-bit value (host bits kept), `length` the prefix length. */
export interface IpPrefix {
  family: IpFamily;
  address: bigint;
  length: number;
}

const BITS: Record<IpFamily, number> = { 4: 32, 6: 128 };

/** `10.0.0.1` → 167772161n. Rejects leading zeros, missing or out-of-range octets. */
export function parseIpv4(text: string): bigint | undefined {
  const octets = text.split('.');
  if (octets.length !== 4) return undefined;
  let value = 0n;
  for (const octet of octets) {
    if (!/^(?:0|[1-9][0-9]{0,2})$/.test(octet) || Number(octet) > 255) return undefined;
    value = (value << 8n) | BigInt(Number(octet));
  }
  return value;
}

/**
 * `2001:db8::1` → bigint. Handles `::` compression and an embedded IPv4 tail (`::ffff:10.0.0.1`).
 * Zone identifiers (`fe80::1%eth0`) are not addresses and are rejected.
 */
export function parseIpv6(text: string): bigint | undefined {
  const gap = text.indexOf('::');
  const compressed = gap >= 0;
  if (compressed && text.indexOf('::', gap + 1) >= 0) return undefined; // a second `::` (also catches `:::`)
  const headText = compressed ? text.slice(0, gap) : text;
  const tailText = compressed ? text.slice(gap + 2) : '';
  const head = headText === '' ? [] : headText.split(':');
  const tail = tailText === '' ? [] : tailText.split(':');
  const headValues = expandGroups(head, tail.length === 0);
  const tailValues = expandGroups(tail, true);
  if (headValues === undefined || tailValues === undefined) return undefined;
  const count = headValues.length + tailValues.length;
  if (compressed ? count > 7 : count !== 8) return undefined;
  const full = [...headValues, ...Array<bigint>(8 - count).fill(0n), ...tailValues];
  return full.reduce((acc, group) => (acc << 16n) | group, 0n);
}

/** Textual groups → 16-bit values; an embedded IPv4 is allowed only as the very last group. */
function expandGroups(groups: readonly string[], allowEmbeddedIpv4: boolean): bigint[] | undefined {
  const out: bigint[] = [];
  for (const [i, group] of groups.entries()) {
    if (group.includes('.')) {
      if (!allowEmbeddedIpv4 || i !== groups.length - 1) return undefined;
      const v4 = parseIpv4(group);
      if (v4 === undefined) return undefined;
      out.push(v4 >> 16n, v4 & 0xffffn);
    } else {
      if (!/^[0-9a-fA-F]{1,4}$/.test(group)) return undefined;
      out.push(BigInt(Number.parseInt(group, 16)));
    }
  }
  return out;
}

/** Address family of a bare address, or `undefined` if it is neither IPv4 nor IPv6. */
export function ipFamily(text: string): IpFamily | undefined {
  if (parseIpv4(text) !== undefined) return 4;
  if (parseIpv6(text) !== undefined) return 6;
  return undefined;
}

/** Parse `a.b.c.d/len` or `x::y/len`. Host bits are kept — use {@link isNetworkAddress} to check them. */
export function parseCidr(text: string): IpPrefix | undefined {
  const slash = text.lastIndexOf('/');
  if (slash < 0) return undefined;
  const lengthText = text.slice(slash + 1);
  if (!/^(?:0|[1-9][0-9]?|1[01][0-9]|12[0-8])$/.test(lengthText)) return undefined;
  const length = Number(lengthText);
  const addressText = text.slice(0, slash);
  const v4 = parseIpv4(addressText);
  if (v4 !== undefined) return length <= BITS[4] ? { family: 4, address: v4, length } : undefined;
  const v6 = parseIpv6(addressText);
  return v6 === undefined ? undefined : { family: 6, address: v6, length };
}

/** The prefix with its host bits cleared. */
export function networkAddress(prefix: IpPrefix): bigint {
  const hostBits = BigInt(BITS[prefix.family] - prefix.length);
  return (prefix.address >> hostBits) << hostBits;
}

/** True when the CIDR text has no host bits set (`10.0.0.0/24`, not `10.0.0.1/24`). Malformed → false. */
export function isNetworkAddress(text: string): boolean {
  const prefix = parseCidr(text);
  return prefix !== undefined && networkAddress(prefix) === prefix.address;
}

/** True when two prefixes of the same family share at least one address (one contains the other). */
export function prefixesOverlap(a: IpPrefix, b: IpPrefix): boolean {
  if (a.family !== b.family) return false;
  const length = Math.min(a.length, b.length);
  return networkAddress({ ...a, length }) === networkAddress({ ...b, length });
}

/** True when the bare `address` of `family` lies inside `prefix`. */
export function prefixContains(prefix: IpPrefix, family: IpFamily, address: bigint): boolean {
  return prefixesOverlap(prefix, { family, address, length: BITS[family] });
}

/** `167772161n` → `10.0.0.1`. */
export function formatIpv4(value: bigint): string {
  return [24n, 16n, 8n, 0n].map((shift) => String((value >> shift) & 0xffn)).join('.');
}

/**
 * 128-bit value → RFC 5952 canonical text: lower case, no leading zeros, the longest run (≥ 2) of zero groups
 * compressed to `::` (the first one on a tie). Embedded-IPv4 notation is not produced.
 */
export function formatIpv6(value: bigint): string {
  const groups = Array.from({ length: 8 }, (_, i) => (value >> BigInt(112 - 16 * i)) & 0xffffn);
  let bestStart = -1;
  let bestLength = 1; // a single zero group is never compressed
  for (let i = 0; i < 8;) {
    if (groups[i] !== 0n) {
      i += 1;
      continue;
    }
    let j = i;
    while (j < 8 && groups[j] === 0n) j += 1;
    if (j - i > bestLength) {
      bestStart = i;
      bestLength = j - i;
    }
    i = j;
  }
  const hex = (gs: bigint[]) => gs.map((g) => g.toString(16)).join(':');
  if (bestStart < 0) return hex(groups);
  return `${hex(groups.slice(0, bestStart))}::${hex(groups.slice(bestStart + bestLength))}`;
}

/**
 * Canonical text of a bare address (`2001:DB8:0::1` → `2001:db8::1`, `10.0.0.1` unchanged) or `undefined` when
 * `text` is not an address. Use it as the key whenever two spellings of one address must compare equal (D-049).
 */
export function canonicalIp(text: string): string | undefined {
  const v4 = parseIpv4(text);
  if (v4 !== undefined) return formatIpv4(v4);
  const v6 = parseIpv6(text);
  return v6 === undefined ? undefined : formatIpv6(v6);
}

/**
 * Canonical text of a prefix with its host bits cleared (`2001:DB8:0::/32` → `2001:db8::/32`,
 * `10.0.0.1/24` → `10.0.0.0/24`) or `undefined` for malformed input — the uniqueness key of routes and networks.
 */
export function canonicalPrefix(text: string): string | undefined {
  const prefix = parseCidr(text);
  if (prefix === undefined) return undefined;
  const network = networkAddress(prefix);
  return `${prefix.family === 4 ? formatIpv4(network) : formatIpv6(network)}/${prefix.length}`;
}

/** Uniqueness key of an address: {@link canonicalIp}, or the text itself when it is not an address. */
export function ipKey(text: string): string {
  return canonicalIp(text) ?? text;
}

/** Uniqueness key of a prefix: {@link canonicalPrefix}, or the text itself when it is not a prefix. */
export function prefixKey(text: string): string {
  return canonicalPrefix(text) ?? text;
}
