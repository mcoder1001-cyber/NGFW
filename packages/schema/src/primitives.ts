import { z } from 'zod';
import { isNetworkAddress } from './ip.js';
import { withUi } from './ui.js';

/**
 * Reusable field primitives shared by every domain. Use these instead of re-declaring string patterns so the
 * generated JSON Schema (format / pattern / bounds) is identical everywhere and the UI can pick matching
 * widgets. Refinements (`.refine`) are enforced at parse time only — JSON Schema carries the pattern/bounds.
 */

/* ------------------------------------------------------------------------------------------------ addresses */

/** A single IPv4 address, dotted quad without leading zeros. */
export const ipv4Address = withUi(z.ipv4(), { title: 'IPv4 address', widget: 'ip' });

/** A single IPv6 address (RFC 4291 text form; no zone identifier, no prefix length). */
export const ipv6Address = withUi(z.ipv6(), { title: 'IPv6 address', widget: 'ip' });

/** A single IPv4 or IPv6 address (no prefix length). */
export const ipAddress = withUi(z.union([z.ipv4(), z.ipv6()]), {
  title: 'IP address',
  widget: 'ip',
});

/** IPv4 prefix in CIDR notation, e.g. `10.0.0.1/24` (host bits allowed — interface addresses keep them). */
export const ipv4Cidr = withUi(z.cidrv4(), {
  title: 'IPv4 CIDR',
  widget: 'cidr',
  help: 'e.g. 10.0.0.1/24',
});

/** IPv6 prefix in CIDR notation, e.g. `2001:db8::1/64` (host bits allowed). */
export const ipv6Cidr = withUi(z.cidrv6(), {
  title: 'IPv6 CIDR',
  widget: 'cidr',
  help: 'e.g. 2001:db8::1/64',
});

/** IPv4 or IPv6 CIDR with host bits allowed. */
export const ipCidr = withUi(z.union([z.cidrv4(), z.cidrv6()]), { title: 'CIDR', widget: 'cidr' });

const NO_HOST_BITS = 'host bits must be zero (e.g. 10.0.0.0/24, not 10.0.0.1/24)';

/** IPv4 network prefix — CIDR whose host bits are zero (`10.0.0.0/24`). Routes and prefix-lists use this. */
export const ipv4Network = withUi(z.cidrv4().refine(isNetworkAddress, NO_HOST_BITS), {
  title: 'IPv4 prefix',
  widget: 'cidr',
  help: 'network address with a prefix length, e.g. 10.0.0.0/24 or 0.0.0.0/0',
});

/** IPv6 network prefix — CIDR whose host bits are zero (`2001:db8::/64`). */
export const ipv6Network = withUi(z.cidrv6().refine(isNetworkAddress, NO_HOST_BITS), {
  title: 'IPv6 prefix',
  widget: 'cidr',
  help: 'network address with a prefix length, e.g. 2001:db8::/64 or ::/0',
});

/** IPv4 or IPv6 network prefix (host bits zero). */
export const ipNetwork = withUi(z.union([ipv4Network, ipv6Network]), {
  title: 'Prefix',
  widget: 'cidr',
});

/* ------------------------------------------------------------------------------------------------ link layer */

/**
 * Unicast MAC address `aa:bb:cc:dd:ee:ff`. `-` separators and upper case are accepted as written (consumers
 * lower-case when talking to VPP); multicast (odd first octet) and all-zero addresses are rejected.
 */
export const macAddress = withUi(
  z
    .string()
    .regex(
      /^[0-9a-fA-F]{2}([:-])(?:[0-9a-fA-F]{2}\1){4}[0-9a-fA-F]{2}$/,
      'expected a MAC address like aa:bb:cc:dd:ee:ff',
    )
    .refine((mac) => (Number.parseInt(mac.slice(0, 2), 16) & 1) === 0, 'multicast MAC addresses are not allowed')
    .refine((mac) => !/^00([:-]00){5}$/.test(mac), 'the all-zero MAC address is not allowed'),
  { title: 'MAC address', widget: 'mac' },
);

/**
 * VPP interface name as shown by `show interface`: `TenGigabitEthernet0/0/0`, `GigabitEthernet0/8/0.100`,
 * `vmxnet3-0/b/0/0` (PCI parts are hex), `loop0`, `BondEthernet0`, `host-w1-eth0`, `memif0/0`, `vxlan_tunnel0`,
 * `ipsec0`. VPP limits names to 63 bytes. Names contain `/` — always escape them with `jsonPointer()`.
 */
export const vppInterfaceName = withUi(
  z
    .string()
    .min(1)
    .max(63)
    .regex(
      /^[A-Za-z][A-Za-z0-9_-]*(?:\/[0-9a-fA-F]+)*(?:\.[0-9]+)?$/,
      'expected a VPP interface name like TenGigabitEthernet0/0/0',
    ),
  { title: 'Interface', widget: 'interface-picker' },
);

/** 802.1Q VLAN identifier (1–4094; 0 and 4095 are reserved). */
export const vlanId = withUi(z.number().int().min(1).max(4094), { title: 'VLAN ID', widget: 'number' });

/** Interface MTU in bytes (68 = IPv4 minimum, 9216 = jumbo). */
export const mtu = withUi(z.number().int().min(68).max(9216), {
  title: 'MTU',
  widget: 'number',
  help: 'bytes, 68–9216',
});

/** PCI address `DDDD:BB:DD.F` as used by `dpdk { dev 0000:0b:00.0 }`. */
export const pciAddress = withUi(
  z.string().regex(/^[0-9a-fA-F]{4}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}\.[0-7]$/, 'expected a PCI address like 0000:0b:00.0'),
  { title: 'PCI address', help: 'domain:bus:device.function, e.g. 0000:0b:00.0' },
);

/* -------------------------------------------------------------------------------------------------- naming */

/**
 * RFC 1123 hostname: labels of 1–63 alphanumerics/hyphens, not starting or ending with `-`, ≤ 253 total,
 * ASCII only (IDNs must be given in A-label / punycode form), last label not all-numeric (RFC 1123 §2.1).
 */
export const hostname = withUi(
  z
    .string()
    .min(1)
    .max(253)
    .regex(
      /^(?!-)[A-Za-z0-9-]{1,63}(?<!-)(?:\.(?!-)[A-Za-z0-9-]{1,63}(?<!-))*$/,
      'expected an RFC 1123 hostname',
    )
    .refine((name) => !/^[0-9]+$/.test(name.split('.').at(-1) ?? ''), 'the last label must not be all digits'),
  { title: 'Hostname' },
);

/** A hostname or an IP address — NTP servers, syslog collectors, RADIUS servers. */
export const hostOrIp = withUi(z.union([z.ipv4(), z.ipv6(), hostname]), {
  title: 'Host',
  help: 'IP address or hostname',
});

/**
 * Name of an object inside its domain (VRF, address object, ACL, tunnel …). Uniqueness is per record key within
 * the domain (vdom.md guardrail #2) — never global.
 */
export const objectName = withUi(
  z
    .string()
    .min(1)
    .max(63)
    .regex(/^[A-Za-z0-9][A-Za-z0-9_.-]*$/, 'letters, digits, `_`, `.` and `-` only'),
  { title: 'Name' },
);

/** VRF name — an `objectName` that must exist under `/vrfs` (or be `default`). */
export const vrfName = withUi(objectName, {
  title: 'VRF',
  widget: 'vrf-picker',
  help: 'routing table this object belongs to; `default` is table 0',
});

/** Local login name: POSIX portable, lower-case, 1–32 characters. */
export const username = withUi(
  z.string().regex(/^[a-z_][a-z0-9_-]{0,31}$/, 'lower-case letters, digits, `_` and `-`; must not start with a digit'),
  { title: 'Username' },
);

/** Free-text description shown in lists (single line, ≤ 255 characters). */
export const descriptionText = withUi(z.string().max(255).regex(/^[^\r\n]*$/, 'single line'), {
  title: 'Description',
});

/* ---------------------------------------------------------------------------------------------- numbers */

/** TCP/UDP port number. */
export const portNumber = withUi(z.number().int().min(1).max(65535), { title: 'Port', widget: 'number' });

/** BGP autonomous system number (asplain, 1–4294967295). */
export const asNumber = withUi(z.number().int().min(1).max(4294967295), {
  title: 'AS number',
  widget: 'number',
  help: 'asplain, e.g. 65000 or 4200000000',
});

/** Router ID in dotted-quad form (BGP / OSPF). */
export const routerId = withUi(z.ipv4(), { title: 'Router ID', help: 'dotted quad, e.g. 10.255.0.1' });

/* ------------------------------------------------------------------------------------------------- secrets */

/**
 * Reference to an entry in the secret store (`secret.ref`) — PSKs, private keys, RADIUS shared secrets are
 * never stored inline in the configuration document (00-CONTEXT rule 10).
 */
export const secretRef = withUi(
  z.string().regex(/^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$/, 'expected a secret reference like ipsec/psk/site-a'),
  { title: 'Secret reference', widget: 'secret-ref' },
);

/**
 * Password hash in modular-crypt / PHC format (`$argon2id$…`, `$6$…`, `$2b$…`). Write-only: the API accepts
 * it and never returns it; fixtures use the placeholder `$vrx-test$VRX_TEST_HASH_<id>`.
 */
export const passwordHash = withUi(
  z
    .string()
    .max(512)
    .regex(/^\$[A-Za-z0-9-]{1,32}\$[A-Za-z0-9./+=$,-]+$/, 'expected a crypt/PHC hash like $argon2id$…'),
  { title: 'Password hash', widget: 'password', secret: true },
);

/* -------------------------------------------------------------------------------------------------- time */

/**
 * IANA time zone name (`UTC`, `Asia/Tehran`, `Etc/GMT+3`). Components are capitalised as in tzdata so the
 * name resolves on a case-sensitive filesystem; existence is checked against the runtime's ICU data.
 */
export const timezone = withUi(
  z
    .string()
    .max(64)
    .regex(/^[A-Z][A-Za-z0-9_+-]*(?:\/[A-Z0-9][A-Za-z0-9_+-]*)*$/, 'expected an IANA time zone like Asia/Tehran')
    .refine(isKnownTimeZone, 'unknown IANA time zone'),
  { title: 'Time zone', widget: 'timezone-picker' },
);

export function isKnownTimeZone(name: string): boolean {
  try {
    new Intl.DateTimeFormat('en-US', { timeZone: name });
    return true;
  } catch {
    return false;
  }
}
