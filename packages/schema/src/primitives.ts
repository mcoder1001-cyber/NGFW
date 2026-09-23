import { z } from 'zod';
import { withUi } from './ui.js';

/**
 * Reusable field primitives shared by every domain. Use these instead of re-declaring string patterns so the
 * generated JSON Schema (format / pattern) is identical everywhere and the UI can pick matching widgets.
 *
 * TODO(P02a): harden (nasty inputs: zone-ids, leading zeros, uppercase/mixed MACs, 64-char labels, IDNs …),
 * reach 100% branch coverage, and add any missing primitive (e.g. `vlanId`, `portRange`, `objectName`).
 */

/** IPv4 prefix in CIDR notation, e.g. `10.0.0.1/24` (host bits allowed — interface addresses keep them). */
export const ipv4Cidr = withUi(z.cidrv4(), {
  title: 'IPv4 CIDR',
  widget: 'cidr',
  help: 'e.g. 10.0.0.1/24',
});

/** IPv6 prefix in CIDR notation, e.g. `2001:db8::1/64`. */
export const ipv6Cidr = withUi(z.cidrv6(), {
  title: 'IPv6 CIDR',
  widget: 'cidr',
  help: 'e.g. 2001:db8::1/64',
});

/** A single IPv4 or IPv6 address (no prefix length). */
export const ipAddress = withUi(z.union([z.ipv4(), z.ipv6()]), {
  title: 'IP address',
  widget: 'ip',
});

/** Unicast MAC address `aa:bb:cc:dd:ee:ff` (also accepts `-` separators and upper case; normalise in P02a). */
export const macAddress = withUi(
  z
    .string()
    .regex(
      /^[0-9a-fA-F]{2}([:-])(?:[0-9a-fA-F]{2}\1){4}[0-9a-fA-F]{2}$/,
      'expected a MAC address like aa:bb:cc:dd:ee:ff',
    ),
  { title: 'MAC address', widget: 'mac' },
);

/**
 * VPP interface name as shown by `show interface`: `TenGigabitEthernet0/0/0`, `GigabitEthernet0/8/0.100`,
 * `loop0`, `host-w1-eth0`, `memif0/0`, `vxlan_tunnel0`, `ipsec0`. VPP limits names to 63 bytes.
 * Names contain `/` — always escape them with `jsonPointer()` when building pointers.
 */
export const vppInterfaceName = withUi(
  z
    .string()
    .min(1)
    .max(63)
    .regex(
      /^[A-Za-z][A-Za-z0-9_-]*(?:\/[0-9]+)*(?:\.[0-9]+)?$/,
      'expected a VPP interface name like TenGigabitEthernet0/0/0',
    ),
  { title: 'Interface', widget: 'interface-picker' },
);

/** RFC 1123 hostname: labels of 1–63 alphanumerics/hyphens, not starting or ending with `-`, ≤ 253 total. */
export const hostname = withUi(
  z
    .string()
    .min(1)
    .max(253)
    .regex(
      /^(?!-)[A-Za-z0-9-]{1,63}(?<!-)(?:\.(?!-)[A-Za-z0-9-]{1,63}(?<!-))*$/,
      'expected an RFC 1123 hostname',
    ),
  { title: 'Hostname' },
);

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
