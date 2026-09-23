import { z } from 'zod';
import { hostname, hostOrIp, ipAddress, timezone, vrfName } from '../primitives.js';
import { withUi } from '../ui.js';
import { DEFAULT_VRF } from './vrfs.js';

/**
 * `system` — hostname, time zone, login banners, NTP (chrony) and DNS client (resolv.conf / Unbound forward)
 * settings (docs/04-api-datamodel.md). Every field has a default so `{}` is a complete system section.
 */

/** Multi-line banner text (rendered into /etc/issue, /etc/motd and the web login page). */
const bannerText = withUi(z.string().max(4096), { widget: 'textarea' });

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

export const NtpServerSchema = z.strictObject({
  address: withUi(hostOrIp, { title: 'Server', order: 1 }),
  prefer: withUi(z.boolean().default(false), { title: 'Preferred', order: 2 }),
  iburst: withUi(z.boolean().default(true), {
    title: 'iburst',
    help: 'send a burst of packets at start-up for fast initial sync',
    order: 3,
  }),
});

export const NtpSchema = z.strictObject({
  enabled: withUi(z.boolean().default(true), { title: 'Enabled', order: 1 }),
  servers: withUi(z.array(NtpServerSchema).max(16).default([]), {
    title: 'Servers',
    itemKey: ['address'],
    order: 2,
  }),
  vrf: withUi(vrfName.default(DEFAULT_VRF), {
    title: 'VRF',
    help: 'VRF used to reach the NTP servers',
    order: 3,
  }),
});

export const DnsSchema = z.strictObject({
  servers: withUi(z.array(ipAddress).max(8).default([]), {
    title: 'Name servers',
    help: 'upstream resolvers for the system itself',
    order: 1,
  }),
  searchDomains: withUi(z.array(hostname).max(6).default([]), { title: 'Search domains', order: 2 }),
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
    ntp: withUi(NtpSchema.prefault({}), { title: 'NTP', group: 'time', order: 4 }),
    dns: withUi(DnsSchema.prefault({}), { title: 'DNS client', group: 'name-resolution', order: 5 }),
  }),
  {
    title: 'System',
    description: 'Hostname, timezone, login banner, NTP and DNS client settings.',
    order: 10,
  },
);

export type SystemConfig = z.infer<typeof SystemSchema>;
