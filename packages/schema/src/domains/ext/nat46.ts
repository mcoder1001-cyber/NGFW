import { z } from 'zod';
import { ipv4Address, ipv6Address, objectName, vppInterfaceName } from '../../primitives.js';
import { withUi } from '../../ui.js';

/**
 * `nat.nat46` — stateless NAT46 (RFC 7915 SIIT, 1:1): an IPv4 service address in front of an IPv6-only server
 * (F-nat46, WBS D4.5). VPP 26.06 has no NAT46 plugin; the agent projects every mapping onto a MAP-T domain in 1:1
 * mode (`nat46-<name>`: IPv4 /32 ↔ IPv6 /128, ea_bits_len 0, ip6_src = `clientPrefix`) plus MAP-T translation on
 * `interfaces` (docs/agent/descriptors/nat46.md). Stateful NAT46 is out of scope (LOG.md, docs/vpp-code-track.md).
 *
 * Intra-`nat46` rules are refinements here, mirroring `descriptors/nat46.Validate`: clientPrefix a /96 with host bits
 * zero, interfaces required with mappings and unique, mapping names unique and short enough for the VPP tag, IPv4
 * service addresses and IPv6 servers unique, a server never inside the client prefix. Cross-domain rules (interface
 * existence, overlap with `nat.map`) are the semantic rules in `semantic/nat46.ts`.
 */

/** Domain-name prefix of NAT46 mappings (`descriptors/nat46.DomainPrefix`). */
export const NAT46_PREFIX = 'nat46-';
/** RFC 6052 well-known prefix. */
export const NAT46_WELL_KNOWN_PREFIX = '64:ff9b::/96';

export const Nat46MappingSchema = withUi(
  z.strictObject({
    name: withUi(objectName.max(54), {
      title: 'Name',
      help: `The VPP domain is '${NAT46_PREFIX}<name>' (the tag holds 64 bytes with the owner)`,
    }),
    ipv4: withUi(ipv4Address, { title: 'IPv4 service address', help: 'What IPv4 clients connect to' }),
    ipv6: withUi(ipv6Address, { title: 'IPv6 server', help: 'The IPv6-only server behind the service address' }),
    mtu: withUi(z.number().int().min(1280).max(65535), {
      title: 'IPv6 MTU',
      help: 'Unset = VPP default; IPv4 packets above MTU-20 are fragmented',
    }).optional(),
  }),
  { title: 'NAT46 mapping' },
);

/** Parse an IPv6 address into a 128-bit bigint (z.ipv6() already validated it). */
export function ipv6ToBigInt(a: string): bigint | undefined {
  let s = a.toLowerCase();
  const pct = s.indexOf('%');
  if (pct >= 0) s = s.slice(0, pct);
  const v4 = /(\d+)\.(\d+)\.(\d+)\.(\d+)$/.exec(s);
  if (v4) {
    const n = v4.slice(1).map(Number);
    if (n.some((x) => x > 255)) return undefined;
    s = s.slice(0, v4.index) + `${((n[0]! << 8) | n[1]!).toString(16)}:${((n[2]! << 8) | n[3]!).toString(16)}`;
  }
  const halves = s.split('::');
  if (halves.length > 2) return undefined;
  const left = halves[0] ? halves[0].split(':') : [];
  const right = halves.length === 2 && halves[1] ? halves[1].split(':') : [];
  const fill = halves.length === 2 ? 8 - left.length - right.length : 0;
  if (fill < 0 || (halves.length === 1 && left.length !== 8)) return undefined;
  const groups = [...left, ...Array<string>(fill).fill('0'), ...right];
  let out = 0n;
  for (const g of groups) {
    if (!/^[0-9a-f]{1,4}$/.test(g)) return undefined;
    out = (out << 16n) | BigInt(parseInt(g, 16));
  }
  return out;
}

/** Parse "addr/len" into [network, bits]; undefined when not IPv6. */
export function ipv6Prefix(p: string): { net: bigint; bits: number } | undefined {
  const [a, l] = p.split('/');
  const bits = Number(l);
  const net = a === undefined ? undefined : ipv6ToBigInt(a);
  if (net === undefined || !Number.isInteger(bits) || bits < 0 || bits > 128) return undefined;
  return { net, bits };
}

const hostMask = (bits: number) => (1n << BigInt(128 - bits)) - 1n;

/** Whether IPv6 address `a` lies inside prefix `p`. */
export function ipv6PrefixContains(p: { net: bigint; bits: number }, a: bigint): boolean {
  const m = hostMask(p.bits);
  return (a & ~m) === (p.net & ~m);
}

export const Nat46Schema = withUi(
  z
    .strictObject({
      clientPrefix: withUi(z.cidrv6().default(NAT46_WELL_KNOWN_PREFIX), {
        title: 'Client prefix',
        widget: 'cidr',
        help: 'RFC 6052 /96 that represents IPv4 clients on the IPv6 side (servers see <prefix>::<client IPv4>)',
      }),
      interfaces: withUi(z.array(vppInterfaceName).max(1024).default([]), {
        title: 'Interfaces',
        widget: 'interface-picker',
        help: 'MAP-T translation runs on these (both the IPv4-facing and the IPv6-facing interfaces)',
      }),
      mappings: withUi(z.array(Nat46MappingSchema).max(4096).default([]), { title: 'Mappings' }),
    })
    .superRefine((v, ctx) => {
      const add = (path: (string | number)[], message: string) => ctx.addIssue({ code: 'custom', path, message });
      const cp = ipv6Prefix(v.clientPrefix);
      if (cp !== undefined) {
        if (cp.bits !== 96) {
          add(['clientPrefix'], 'must be a /96 (RFC 6052; 1:1 SIIT embeds the whole IPv4 address in the last 32 bits)');
        } else if ((cp.net & hostMask(96)) !== 0n) {
          add(['clientPrefix'], 'host bits must be zero');
        }
      }
      if (v.mappings.length > 0 && v.interfaces.length === 0) {
        add(['interfaces'], 'at least one interface is required when mappings are configured');
      }
      const ifs = new Set<string>();
      v.interfaces.forEach((n, i) => {
        if (ifs.has(n)) add(['interfaces', i], `duplicate interface '${n}'`);
        ifs.add(n);
      });
      const names = new Set<string>();
      const v4s = new Set<string>();
      const v6s = new Set<bigint>();
      v.mappings.forEach((m, i) => {
        if (names.has(m.name)) add(['mappings', i, 'name'], `duplicate mapping name '${m.name}'`);
        names.add(m.name);
        const o = m.ipv4.split('.').map(Number);
        if (o[0] === 0 || o[0] === 127 || o[0]! >= 224 || (o[0] === 169 && o[1] === 254)) {
          add(['mappings', i, 'ipv4'], 'must be a unicast address');
        } else if (v4s.has(m.ipv4)) {
          add(['mappings', i, 'ipv4'], `IPv4 service address ${m.ipv4} used twice`);
        }
        v4s.add(m.ipv4);
        const a6 = ipv6ToBigInt(m.ipv6);
        if (a6 === undefined) return;
        if (a6 <= 1n || a6 >> 120n === 0xffn || a6 >> 118n === 0x3fan) {
          add(['mappings', i, 'ipv6'], 'must be a unicast address');
        } else if (v6s.has(a6)) {
          add(['mappings', i, 'ipv6'], `IPv6 server ${m.ipv6} used twice`);
        } else if (cp !== undefined && ipv6PrefixContains(cp, a6)) {
          add(['mappings', i, 'ipv6'], `server ${m.ipv6} is inside the client prefix ${v.clientPrefix}`);
        }
        v6s.add(a6);
      });
    }),
  { title: 'NAT46 (stateless SIIT 1:1)' },
);

export type Nat46Config = z.infer<typeof Nat46Schema>;
export type Nat46Mapping = z.infer<typeof Nat46MappingSchema>;
