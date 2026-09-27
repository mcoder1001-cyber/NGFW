import { z } from 'zod';
import { mtu, secretRefOf } from '../../primitives.js';
import { withUi } from '../../ui.js';

/**
 * F-pppoe-client: `interfaces.<name>.pppoe` — a PPPoE *client* on a WAN interface (ISP dial-up). The box runs `pppd`
 * (rp-pppoe kernel plugin) on a linux-cp tap of `<name>` (or the parent named here), negotiates the session, and the
 * agent mirrors the ISP-assigned IPv4/IPv6 address and default route into VPP. This is the client/dial-up side; the
 * PPPoE *server/AC* (`pppoe_session` decap) is a different feature and stays in `tunnels`.
 *
 * The password is never inline: `passwordRef` points at a `password/<name>` secret. `parent` is optional — by default
 * the pppd session runs over the interface this block sits on; set it only to dial over a different (e.g. VLAN
 * sub-)interface than the one that carries the resulting IP.
 *
 * Cross-object rules (`../../semantic/pppoe.ts`): the parent interface exists and is enabled; a `pppoe` interface has
 * no static addresses of its own (the peer assigns them); MTU ≤ parent MTU − 8 (PPPoE + PPP headers).
 */

const GROUP = 'pppoe';

/** PPPoE AC service name (RFC 2516 Service-Name tag): printable, no control characters, ≤ 255. */
const serviceName = withUi(
  z
    .string()
    .min(1)
    .max(255)
    .regex(/^[\x20-\x7e]+$/, 'printable ASCII only (a PPPoE service name)'),
  {
    title: 'Service name',
    help: 'RFC 2516 Service-Name to request; leave empty to accept any AC',
  },
);

/** Reconnect policy after a dropped or refused session. */
export const PppoeReconnectSchema = z.strictObject({
  holdoffSec: withUi(z.number().int().min(0).max(3600).default(5), {
    title: 'Hold-off (seconds)',
    help: 'wait this long before redialling after the session goes down',
    order: 1,
  }),
  maxFail: withUi(z.number().int().min(0).max(1000).default(0), {
    title: 'Max consecutive failures',
    help: 'give up after this many failed dials in a row; 0 = keep trying forever',
    order: 2,
  }),
});
export type PppoeReconnect = z.infer<typeof PppoeReconnectSchema>;

export const InterfacePppoeSchema = z.strictObject({
  enabled: withUi(z.boolean().default(true), {
    title: 'Enabled',
    help: 'dial the session; disable to keep the settings but stay offline',
    order: 1,
  }),
  parent: withUi(z.string().min(1).max(63).optional(), {
    title: 'Dial over interface',
    widget: 'interface-picker',
    help: 'engine interface the session runs over; default = this interface (set for a VLAN sub-interface WAN)',
    order: 2,
  }),
  username: withUi(
    z
      .string()
      .min(1)
      .max(255)
      .regex(/^[\x21-\x7e]+$/, 'no spaces or control characters'),
    {
      title: 'Username',
      help: 'PPP username (PAP/CHAP) from the ISP',
      order: 3,
    },
  ),
  passwordRef: withUi(secretRefOf('password'), {
    title: 'Password',
    help: 'reference to a stored password secret (password/<name>); never the password itself',
    order: 4,
  }),
  serviceName: withUi(serviceName.optional(), {
    title: 'Service name',
    help: 'RFC 2516 Service-Name to request; leave empty to accept any AC',
    order: 5,
  }),
  mtu: withUi(mtu.default(1492), {
    title: 'MTU',
    help: 'PPPoE payload MTU; 1492 is the Ethernet default (1500 − 8 for the PPPoE/PPP headers)',
    order: 6,
  }),
  mssClamp: withUi(z.boolean().default(true), {
    title: 'Clamp TCP MSS',
    help: 'rewrite the TCP MSS of forwarded SYNs to fit the PPPoE MTU (avoids black-holed large packets)',
    order: 7,
  }),
  defaultRoute: withUi(z.boolean().default(true), {
    title: 'Default route from peer',
    help: 'install a default route via the session (the ISP is the gateway)',
    order: 8,
  }),
  dnsFromPeer: withUi(z.boolean().default(false), {
    title: 'Use peer DNS',
    help: 'use the DNS servers the ISP sends (IPCP) as the system resolvers',
    order: 9,
  }),
  ipv6: withUi(z.enum(['off', 'slaac', 'dhcpv6']).default('off'), {
    title: 'IPv6',
    widget: 'select',
    help: 'off; slaac (accept a /64 via RA over the link); dhcpv6 (request a prefix via DHCPv6-PD)',
    order: 10,
  }),
  reconnect: withUi(PppoeReconnectSchema.default({ holdoffSec: 5, maxFail: 0 }), {
    title: 'Reconnect',
    help: 'what to do when the session drops',
    order: 11,
  }),
});
export type InterfacePppoe = z.infer<typeof InterfacePppoeSchema>;

/** `interfaces.<name>.pppoe` field (absent = not a PPPoE client). */
export const interfacePppoeField = withUi(InterfacePppoeSchema.optional(), {
  title: 'PPPoE client',
  help: 'dial an ISP over PPPoE and use the assigned address on this interface; absent = off',
  group: GROUP,
  order: 60,
});
