import { z } from 'zod';
import { withUi } from '../../ui.js';
import { ipPrefix, l4PortNumber } from '../objects.js';

/**
 * `acl.hostSettings` — how the agent renders the input chains of the host firewall (F-host-acl-nftables, WBS D5.3,
 * D-057). The host lists (`acl.host`) and their attachments (`acl.hostAttachments`) are P02b's; this sub-schema adds
 * the settings the nftables renderer needs and that no existing leaf carries (config gap, additive):
 *
 *   defaultInput   policy of every input chain the agent renders (accept | drop)
 *   allowIcmp      accept ICMP / ICMPv6 before the rules (IPv6 neighbour/router discovery is accepted either way)
 *   antiLockout    management TCP ports from `sources` on `interfaces` are accepted at the top of every input chain;
 *                  with `enabled: false` a commit whose own rules would drop that traffic fails (acl.host-anti-lockout)
 *
 * Absent = every default. Linux interface names are validated like `acl.host.<list>.rules[].interface`.
 */

/** Linux interface name (IFNAMSIZ 15) — the same rule as `linuxInterfaceName` in acl.ts. */
const hostInterfaceName = withUi(
  z.string().regex(/^[A-Za-z0-9_.-]{1,15}$/, 'expected a Linux interface name (max 15 chars)'),
  { title: 'Host interface', widget: 'host-interface-picker' },
);

export const HostAclAntiLockoutSchema = withUi(
  z.strictObject({
    enabled: withUi(z.boolean().default(true), {
      title: 'Anti-lockout rule',
      help: 'Accept management SSH/HTTPS at the top of every input chain. Off: the rules themselves must accept it (checked on commit)',
    }),
    sources: withUi(z.array(ipPrefix).max(64).default([]), {
      title: 'Management sources',
      help: 'Prefixes management connections come from; empty = any source',
    }),
    interfaces: withUi(z.array(hostInterfaceName).max(16).default([]), {
      title: 'Management interfaces',
      help: 'Linux interfaces management connections arrive on; empty = any interface',
    }),
    ports: withUi(z.array(l4PortNumber).min(1).max(8).default([22, 443]), {
      title: 'Management TCP ports',
      help: 'SSH and HTTPS by default',
    }),
  }),
  { title: 'Anti-lockout' },
);

export const HostAclSettingsSchema = withUi(
  z.strictObject({
    defaultInput: withUi(z.enum(['accept', 'drop']).default('accept'), {
      title: 'Default input policy',
      help: 'Policy of the input chains: drop = only traffic a rule (or the anti-lockout rule) accepts reaches the host',
    }),
    allowIcmp: withUi(z.boolean().default(true), {
      title: 'Allow ICMP',
      help: 'Accept ICMP and ICMPv6 in input chains before the rules (IPv6 neighbour discovery is always accepted)',
    }),
    antiLockout: HostAclAntiLockoutSchema.prefault({}),
  }),
  { title: 'Host firewall settings', group: 'host-acl-nftables' },
);

export type HostAclAntiLockout = z.infer<typeof HostAclAntiLockoutSchema>;
export type HostAclSettings = z.infer<typeof HostAclSettingsSchema>;
