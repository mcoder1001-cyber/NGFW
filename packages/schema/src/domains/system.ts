import { z } from 'zod';
import { hostname, ipAddress, multilineText, timezone, vrfName } from '../primitives.js';
import { withUi } from '../ui.js';
import { DEFAULT_VRF } from './vrfs.js';

/**
 * `system` — hostname, time zone, login banners and the DNS client of the router itself (docs/04-api-datamodel.md).
 * Every field has a default so `{}` is a complete system section. NTP is modelled once, in `services.ntp`
 * (chrony is a rendered daemon, D-050).
 */

/**
 * Multi-line banner text (rendered into /etc/issue, /etc/motd and the web login page): printable, LF and TAB
 * only — no CR / ESC / BEL / C1 that could forge or hide lines on a terminal (D-049).
 */
const bannerText = withUi(multilineText(4096), { widget: 'textarea' });

export const BannerSchema = z.strictObject({
  login: withUi(bannerText.optional(), {
    title: 'Pre-login banner',
    help: 'shown before authentication (SSH issue, web login page)',
    order: 1,
  }),
  motd: withUi(bannerText.optional(), {
    title: 'Message of the day',
    help: 'shown after a successful login',
    order: 2,
  }),
});

/**
 * DNS client of the router itself (resolv.conf). Named `SystemDnsSchema` so it cannot collide with the DNS
 * *service* schema of `services` under `export *` (D-047). The TS identifier is not part of the JSON contract.
 */
export const SystemDnsSchema = z.strictObject({
  servers: withUi(z.array(ipAddress).max(8).default([]), {
    title: 'Name servers',
    help: 'upstream resolvers for the system itself',
    order: 1,
  }),
  searchDomains: withUi(z.array(hostname).max(6).default([]), {
    title: 'Search domains',
    order: 2,
  }),
  vrf: withUi(vrfName.default(DEFAULT_VRF), {
    title: 'VRF',
    help: 'VRF used to reach the name servers',
    order: 3,
  }),
});

export const SystemSchema = withUi(
  z.strictObject({
    hostname: withUi(hostname.default('vrx'), {
      title: 'Hostname',
      help: 'RFC 1123 host name, e.g. vrx-a or vrx-a.lab.example',
      group: 'identity',
      order: 1,
    }),
    timezone: withUi(timezone.default('UTC'), {
      title: 'Time zone',
      help: 'IANA name, e.g. Asia/Tehran',
      group: 'identity',
      order: 2,
    }),
    banner: withUi(BannerSchema.prefault({}), { title: 'Banners', group: 'identity', order: 3 }),
    dns: withUi(SystemDnsSchema.prefault({}), {
      title: 'DNS client',
      group: 'name-resolution',
      order: 4,
    }),
  }),
  {
    title: 'System',
    description: 'Hostname, timezone, login banners and DNS client settings.',
    order: 10,
  },
);

export type SystemConfig = z.infer<typeof SystemSchema>;
