import { isPlainObject } from '../json.js';

/**
 * Shared helpers for the group-(c) validators (`vpn`, `tunnels`, `services`, `ha`) and the intra-object refinements
 * in `domains/services.ts`: pure IPv4/IPv6 math and duck-typed lookups into the `interfaces`, `vrfs`, `tunnels` and
 * `vpn` subtrees. The lookups read only the documented field names (docs/04, P02 prompt §2) through `unknown`, so
 * they work against the P02s placeholders today and against P02a's full `interfaces`/`vrfs` models unchanged.
 *
 * Not re-exported by `index.ts` (semantic/index.ts exports only the registry). The IP math is a candidate for
 * P02a's shared `ip.ts` — see docs/status/tasks/P02c-questions.md. Owner: P02c.
 */

export type IpFamily = 4 | 6;

/** Address family by syntax: anything containing `:` is IPv6. Inputs are already schema-validated. */
export function ipFamily(address: string): IpFamily {
  return address.includes(':') ? 6 : 4;
}

function parseIpv4(text: string): bigint | undefined {
  const parts = text.split('.');
  if (parts.length !== 4) return undefined;
  let value = 0n;
  for (const part of parts) {
    if (!/^\d{1,3}$/.test(part)) return undefined;
    const n = Number(part);
    if (n > 255) return undefined;
    value = (value << 8n) | BigInt(n);
  }
  return value;
}

function parseIpv6(text: string): bigint | undefined {
  let body = text;
  const zoneAt = body.indexOf('%');
  if (zoneAt !== -1) return undefined;
  // Embedded IPv4 tail (`::ffff:192.0.2.1`) → two hextets.
  const lastColon = body.lastIndexOf(':');
  if (body.includes('.')) {
    const v4 = parseIpv4(body.slice(lastColon + 1));
    if (v4 === undefined) return undefined;
    const hi = (v4 >> 16n).toString(16);
    const lo = (v4 & 0xffffn).toString(16);
    body = `${body.slice(0, lastColon)}:${hi}:${lo}`;
  }
  const halves = body.split('::');
  if (halves.length > 2) return undefined;
  const head = halves[0] === '' ? [] : (halves[0] as string).split(':');
  const tail =
    halves.length === 2 ? (halves[1] === '' ? [] : (halves[1] as string).split(':')) : [];
  const missing = 8 - head.length - tail.length;
  if (halves.length === 2 ? missing < 1 : missing !== 0) return undefined;
  const groups = [...head, ...Array<string>(missing).fill('0'), ...tail];
  let value = 0n;
  for (const g of groups) {
    if (!/^[0-9a-fA-F]{1,4}$/.test(g)) return undefined;
    value = (value << 16n) | BigInt(parseInt(g, 16));
  }
  return value;
}

/** Numeric value of an IPv4 (32-bit) or IPv6 (128-bit) address, or `undefined` when unparsable. */
export function parseIp(address: string): bigint | undefined {
  return ipFamily(address) === 4 ? parseIpv4(address) : parseIpv6(address);
}

export interface Cidr {
  family: IpFamily;
  /** The address as written (host bits kept), numeric. */
  address: bigint;
  prefixLength: number;
  /** First and last address of the prefix. */
  first: bigint;
  last: bigint;
}

/** Parse `a.b.c.d/len` or `x::y/len`. Host bits are kept in `address` and masked in `first`/`last`. */
export function parseCidr(cidr: string): Cidr | undefined {
  const slash = cidr.lastIndexOf('/');
  if (slash === -1) return undefined;
  const addressText = cidr.slice(0, slash);
  const lengthText = cidr.slice(slash + 1);
  if (!/^\d{1,3}$/.test(lengthText)) return undefined;
  const family = ipFamily(addressText);
  const bits = family === 4 ? 32 : 128;
  const prefixLength = Number(lengthText);
  if (prefixLength > bits) return undefined;
  const address = parseIp(addressText);
  if (address === undefined) return undefined;
  const hostBits = BigInt(bits - prefixLength);
  const first = (address >> hostBits) << hostBits;
  const last = first | ((1n << hostBits) - 1n);
  return { family, address, prefixLength, first, last };
}

/** True when every address of `inner` lies inside `outer` (same family). */
export function cidrContains(outer: Cidr, inner: Cidr): boolean {
  return outer.family === inner.family && inner.first >= outer.first && inner.last <= outer.last;
}

/** True when the address (numeric, of `family`) lies inside the prefix. */
export function cidrContainsIp(prefix: Cidr, family: IpFamily, ip: bigint): boolean {
  return prefix.family === family && ip >= prefix.first && ip <= prefix.last;
}

/** True when the two prefixes share at least one address. */
export function cidrsOverlap(a: Cidr, b: Cidr): boolean {
  return a.family === b.family && a.first <= b.last && b.first <= a.last;
}

/** Same prefix (network and length), regardless of host bits. */
export function sameCidr(a: Cidr, b: Cidr): boolean {
  return a.family === b.family && a.first === b.first && a.prefixLength === b.prefixLength;
}

/** 224.0.0.0/4 or ff00::/8. */
export function isMulticast(address: string): boolean {
  const ip = parseIp(address);
  if (ip === undefined) return false;
  return ipFamily(address) === 4 ? ip >> 28n === 0xen : ip >> 120n === 0xffn;
}

/** `0.0.0.0` or `::`. */
export function isUnspecified(address: string): boolean {
  return parseIp(address) === 0n;
}

/** Numeric ordering of two addresses of the same family (`undefined` when either is unparsable). */
export function compareIp(a: string, b: string): number | undefined {
  const x = parseIp(a);
  const y = parseIp(b);
  if (x === undefined || y === undefined || ipFamily(a) !== ipFamily(b)) return undefined;
  return x < y ? -1 : x > y ? 1 : 0;
}

// ---------------------------------------------------------------------------------------------------------------
// Duck-typed lookups into the other domains
// ---------------------------------------------------------------------------------------------------------------

/** Name of the VRF every object falls back to (VPP FIB table 0). It always exists (vdom.md #1 keeps it explicit). */
export const DEFAULT_VRF = 'default';

function asRecord(value: unknown): Record<string, unknown> {
  return isPlainObject(value) ? value : {};
}

function stringField(entry: unknown, key: string): string | undefined {
  const v = asRecord(entry)[key];
  return typeof v === 'string' ? v : undefined;
}

function stringArray(entry: unknown, key: string): string[] {
  const v = asRecord(entry)[key];
  return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : [];
}

function numberField(entry: unknown, key: string): number | undefined {
  const v = asRecord(entry)[key];
  return typeof v === 'number' ? v : undefined;
}

/** VRF names that exist: `default` plus every key of `vrfs`. */
export function knownVrfs(config: unknown): Set<string> {
  return new Set([DEFAULT_VRF, ...Object.keys(asRecord(asRecord(config).vrfs))]);
}

export interface InterfaceInfo {
  /** VPP interface name (`TenGigabitEthernet0/0/0`, `TenGigabitEthernet0/0/1.100`, `gre0`, `wg0`). */
  name: string;
  vrf: string;
  /** Configured prefixes (both families), host bits kept. */
  addresses: Cidr[];
}

function collectAddresses(entry: unknown): Cidr[] {
  const out: Cidr[] = [];
  for (const key of ['ipv4', 'ipv6', 'address']) {
    for (const text of stringArray(entry, key)) {
      const cidr = parseCidr(text);
      if (cidr !== undefined) out.push(cidr);
    }
  }
  return out;
}

/**
 * Every interface a group-(c) object may refer to: `interfaces` (plus sub-interfaces as `<parent>.<key>` and
 * `<parent>.<vlanId>`), tunnels with an explicit `instance` (`gre<i>`, `ipip<i>`, `vxlan_tunnel<i>`) and WireGuard
 * interfaces (`wg<instance>`). A tunnel without `instance` gets its VPP name at runtime and cannot be referenced.
 */
export function interfaceIndex(config: unknown): Map<string, InterfaceInfo> {
  const index = new Map<string, InterfaceInfo>();
  const put = (name: string, vrf: string | undefined, addresses: Cidr[]): void => {
    if (!index.has(name)) index.set(name, { name, vrf: vrf ?? DEFAULT_VRF, addresses });
  };
  const root = asRecord(config);
  for (const [name, entry] of Object.entries(asRecord(root.interfaces))) {
    put(name, stringField(entry, 'vrf'), collectAddresses(entry));
    for (const [subKey, sub] of Object.entries(asRecord(asRecord(entry).subinterfaces))) {
      const vrf = stringField(sub, 'vrf') ?? stringField(entry, 'vrf');
      const addresses = collectAddresses(sub);
      put(`${name}.${subKey}`, vrf, addresses);
      const vlanId = numberField(sub, 'vlanId');
      if (vlanId !== undefined) put(`${name}.${vlanId}`, vrf, addresses);
    }
  }
  const tunnels = asRecord(root.tunnels);
  const kinds: [string, string][] = [
    ['gre', 'gre'],
    ['ipip', 'ipip'],
    ['vxlan', 'vxlan_tunnel'],
  ];
  for (const [kind, prefix] of kinds) {
    for (const entry of Object.values(asRecord(tunnels[kind]))) {
      const instance = numberField(entry, 'instance');
      if (instance !== undefined) {
        put(`${prefix}${instance}`, stringField(entry, 'vrf'), collectAddresses(entry));
      }
    }
  }
  const wg = asRecord(asRecord(asRecord(root.vpn).wireguard).interfaces);
  for (const entry of Object.values(wg)) {
    const instance = numberField(entry, 'instance');
    if (instance !== undefined) {
      put(`wg${instance}`, stringField(entry, 'vrf'), collectAddresses(entry));
    }
  }
  return index;
}

/** True when `address` is configured (as the address part of a prefix) on some interface in `vrf`. */
export function addressConfiguredInVrf(
  index: ReadonlyMap<string, InterfaceInfo>,
  address: string,
  vrf: string,
): boolean {
  const ip = parseIp(address);
  if (ip === undefined) return false;
  const family = ipFamily(address);
  for (const info of index.values()) {
    if (info.vrf !== vrf) continue;
    if (info.addresses.some((c) => c.family === family && c.address === ip)) return true;
  }
  return false;
}

/** Prefixes configured on the named interfaces (missing names are skipped). */
export function prefixesOfInterfaces(
  index: ReadonlyMap<string, InterfaceInfo>,
  names: readonly string[],
): Cidr[] {
  return names.flatMap((n) => index.get(n)?.addresses ?? []);
}

/** Names present in a record-valued node of the document (`vpn.ipsec.proposals`, `tunnels.ipip`, …). */
export function recordKeys(node: unknown): Set<string> {
  return new Set(Object.keys(asRecord(node)));
}

/**
 * Walk every string value under `node` and call `visit(path, value)`. Used by the inline-secret guard: the document
 * is strict, so unknown keys are already rejected — this catches key material pasted into free-text fields.
 */
export function walkStrings(
  node: unknown,
  visit: (path: readonly (string | number)[], value: string) => void,
  path: (string | number)[] = [],
): void {
  if (typeof node === 'string') {
    visit(path, node);
  } else if (Array.isArray(node)) {
    node.forEach((item, i) => walkStrings(item, visit, [...path, i]));
  } else if (isPlainObject(node)) {
    for (const [k, v] of Object.entries(node)) walkStrings(v, visit, [...path, k]);
  }
}
