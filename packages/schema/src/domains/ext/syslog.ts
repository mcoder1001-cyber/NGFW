import { z } from 'zod';
import { hostname, secretRefOf } from '../../primitives.js';
import { withUi } from '../../ui.js';

/**
 * F-unbound-chrony-syslog (WBS D7.7): the RF-4 stand-ins of `management.syslog[i]` (D-055) moved into the contract
 * (D-086). Every key is optional and absent means the RF-4 renderer default, so documents written before this
 * change parse to exactly the same value:
 *
 *   facilities   absent/empty = every facility (`*.<severity>`), otherwise `kern,daemon.<severity>`
 *   format       absent = `rfc5424` (fixed template `vrx_rfc5424`; TCP uses octet-counted framing, RFC 6587)
 *   queueSize    absent = 10000 messages (rsyslog `queue.size` of the export action, LinkedList queue)
 *   tls          only with `protocol: tls` (semantic rule `management.unbound-chrony-syslog-tls`): CA reference
 *                required; client certificate and key references go together; the key needs the API→agent secret
 *                channel (PENDING-secret-channel), which the agent refuses with a DryRun error until it exists
 *
 * The TLS material is referenced (D-051: `cert/<name>`, `key/<name>`), never inline.
 */

/** `x-vrx-ui` group of every field of this feature (wave-A-hotspots C1: group = task slug). */
export const UNBOUND_CHRONY_SYSLOG_GROUP = 'unbound-chrony-syslog';

/** Syslog facilities (RFC 5424 §6.2.1 names as rsyslog spells them). */
export const SYSLOG_FACILITIES = [
  'kern',
  'user',
  'mail',
  'daemon',
  'auth',
  'syslog',
  'lpr',
  'news',
  'uucp',
  'cron',
  'authpriv',
  'ftp',
  'local0',
  'local1',
  'local2',
  'local3',
  'local4',
  'local5',
  'local6',
  'local7',
] as const;
export const SyslogFacility = z.enum(SYSLOG_FACILITIES);
export type SyslogFacility = z.infer<typeof SyslogFacility>;

/** Wire format of the forwarded messages (the renderer's fixed template set — never a user template). */
export const SyslogFormat = z.enum(['rfc5424', 'rfc3164']);
export type SyslogFormat = z.infer<typeof SyslogFormat>;

/** How rsyslog authenticates the collector (anonymous TLS is not offered). */
export const SyslogTlsAuthMode = z.enum(['x509/name', 'x509/certvalid']);
export type SyslogTlsAuthMode = z.infer<typeof SyslogTlsAuthMode>;

export const SyslogTlsSchema = z
  .strictObject({
    caRef: withUi(secretRefOf('cert'), {
      title: 'CA certificate (reference)',
      help: 'Verifies the collector certificate (kind cert)',
      group: UNBOUND_CHRONY_SYSLOG_GROUP,
      order: 1,
    }),
    certRef: withUi(secretRefOf('cert'), {
      title: 'Client certificate (reference)',
      help: 'Only for collectors that require client authentication; goes with keyRef',
      group: UNBOUND_CHRONY_SYSLOG_GROUP,
      order: 2,
    }).optional(),
    keyRef: withUi(secretRefOf('key'), {
      title: 'Client private key (reference)',
      help: 'Goes with certRef (kind key)',
      group: UNBOUND_CHRONY_SYSLOG_GROUP,
      order: 3,
    }).optional(),
    authMode: withUi(SyslogTlsAuthMode, {
      title: 'Peer authentication',
      help: 'x509/name: certificate valid and its name in permittedPeers (default); x509/certvalid: valid certificate only',
      widget: 'select',
      group: UNBOUND_CHRONY_SYSLOG_GROUP,
      order: 4,
    }).optional(),
    permittedPeers: withUi(z.array(hostname).max(16), {
      title: 'Permitted peer names',
      help: 'Collector certificate names accepted with x509/name; empty = the collector host name',
      group: UNBOUND_CHRONY_SYSLOG_GROUP,
      order: 5,
    }).optional(),
  })
  .refine((tls) => (tls.certRef === undefined) === (tls.keyRef === undefined), {
    message: 'certRef and keyRef must be given together',
    path: ['keyRef'],
  });
export type SyslogTls = z.infer<typeof SyslogTlsSchema>;

/**
 * The feature keys of one `management.syslog[i]` entry, spread into `SyslogServerSchema` by one line under the
 * feature's key (wave-BC-numbers SY4). Proto: `SyslogTarget` 6 `facilities`, 7 `format`, 8 `queue_size`, 9 `tls`.
 */
export const syslogTargetExtKeys = {
  facilities: withUi(z.array(SyslogFacility).max(SYSLOG_FACILITIES.length), {
    title: 'Facilities',
    help: 'Forward only these facilities; empty = all',
    group: UNBOUND_CHRONY_SYSLOG_GROUP,
    order: 6,
  }).optional(),
  format: withUi(SyslogFormat, {
    title: 'Format',
    help: 'rfc5424 (default; octet-counted over TCP) or the traditional rfc3164 line',
    widget: 'select',
    group: UNBOUND_CHRONY_SYSLOG_GROUP,
    order: 7,
  }).optional(),
  queueSize: withUi(z.int().min(100).max(1000000), {
    title: 'Queue size (messages)',
    help: 'Messages held while the collector is unreachable (default 10000)',
    widget: 'number',
    group: UNBOUND_CHRONY_SYSLOG_GROUP,
    order: 8,
  }).optional(),
  tls: withUi(SyslogTlsSchema, {
    title: 'TLS',
    help: 'Required with protocol tls; ignored otherwise (rejected)',
    group: UNBOUND_CHRONY_SYSLOG_GROUP,
    order: 9,
  }).optional(),
} as const;
