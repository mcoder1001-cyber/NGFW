import { z } from 'zod';
import {
  descriptionText,
  hostOrIp,
  passwordHash,
  portNumber,
  secretRefOf,
  username,
  vrfName,
} from '../primitives.js';
import { withUi } from '../ui.js';
import { DEFAULT_VRF } from './vrfs.js';

/**
 * `management` — local users, AAA (RADIUS / TACACS+), TLS for the API and remote syslog (docs/04-api-datamodel.md).
 *
 * - Guardrail (vdom.md #3): a user's role assignment is `{ role, scope: '*' }`; `scope` becomes a tenant name
 *   when multi-tenancy lands (widening the literal is an additive contract change).
 * - Secrets (00-CONTEXT rule 10): `passwordHash` is write-only (never returned by GET, never logged); RADIUS /
 *   TACACS+ shared secrets and TLS keys are `secretRef`s into the secret store, never inline.
 * - Semantic rules (`../semantic/management.ts`): once users are configured at least one of them is a usable admin
 *   (D-048: an empty list is valid — the API seeds the first admin), unique usernames, unique AAA / syslog servers,
 *   VRFs exist.
 */

export const UserRole = z.enum(['admin', 'operator', 'readonly']);
export type UserRole = z.infer<typeof UserRole>;

// eslint-disable-next-line no-control-regex -- the comment part must not contain control characters (D-049)
const SSH_KEY_LINE = /^(?:ssh-ed25519|ssh-rsa|ecdsa-sha2-nistp(?:256|384|521)|sk-ssh-ed25519@openssh\.com|sk-ecdsa-sha2-nistp256@openssh\.com) [A-Za-z0-9+/]+={0,3}(?: [^\u0000-\u001f\u007f-\u009f]{1,255})?$/;

/** OpenSSH `authorized_keys` line: `<type> <base64> [comment]` (public material — not a secret). */
export const sshPublicKey = withUi(
  z
    .string()
    .max(4096)
    .regex(
      SSH_KEY_LINE,
      'expected an OpenSSH public key line like "ssh-ed25519 AAAA… comment"',
    ),
  { title: 'SSH public key', widget: 'textarea' },
);

export const UserSchema = z.strictObject({
  username: withUi(username, { title: 'Username', order: 1 }),
  role: withUi(UserRole, {
    title: 'Role',
    help: 'admin: everything; operator: edit and commit configuration; readonly: view only',
    widget: 'select',
    order: 2,
  }),
  scope: withUi(z.literal('*').default('*'), {
    title: 'Scope',
    help: 'role scope; `*` = whole device (tenant scopes are reserved for later)',
    order: 3,
  }),
  passwordHash: withUi(passwordHash.optional(), {
    title: 'Password hash',
    help: 'crypt/PHC hash; write-only. A user without a password can only log in with an SSH key',
    widget: 'password',
    secret: true,
    order: 4,
  }),
  sshKeys: withUi(z.array(sshPublicKey).max(16).default([]), {
    title: 'SSH public keys',
    order: 5,
  }),
  fullName: withUi(descriptionText.optional(), { title: 'Full name', order: 6 }),
  disabled: withUi(z.boolean().default(false), {
    title: 'Disabled',
    help: 'keep the account but refuse logins',
    order: 7,
  }),
});
export type UserConfig = z.infer<typeof UserSchema>;

const timeoutSec = withUi(z.number().int().min(1).max(60).default(5), {
  title: 'Timeout (s)',
  widget: 'number',
});

export const RadiusServerSchema = z.strictObject({
  address: withUi(hostOrIp, { title: 'Server', order: 1 }),
  authPort: withUi(portNumber.default(1812), { title: 'Authentication port', order: 2 }),
  acctPort: withUi(portNumber.default(1813), { title: 'Accounting port', order: 3 }),
  secretRef: withUi(secretRefOf('psk'), {
    title: 'Shared secret',
    help: 'reference into the secret store, e.g. psk/radius-primary',
    order: 4,
  }),
  timeoutSec: withUi(timeoutSec, { order: 5 }),
  vrf: withUi(vrfName.default(DEFAULT_VRF), { title: 'VRF', order: 6 }),
});

export const TacacsServerSchema = z.strictObject({
  address: withUi(hostOrIp, { title: 'Server', order: 1 }),
  port: withUi(portNumber.default(49), { title: 'Port', order: 2 }),
  secretRef: withUi(secretRefOf('psk'), {
    title: 'Shared secret',
    help: 'reference into the secret store, e.g. psk/tacacs-primary',
    order: 3,
  }),
  timeoutSec: withUi(timeoutSec, { order: 4 }),
  vrf: withUi(vrfName.default(DEFAULT_VRF), { title: 'VRF', order: 5 }),
});

export const AuthMethod = z.enum(['local', 'radius', 'tacacs']);
export type AuthMethod = z.infer<typeof AuthMethod>;

export const AaaSchema = z
  .strictObject({
    order: withUi(z.array(AuthMethod).min(1).max(3).default(['local']), {
      title: 'Authentication order',
      help: 'methods tried in order until one answers; `local` should stay in the list',
      order: 1,
    }),
    radius: withUi(
      z
        .strictObject({
          servers: withUi(z.array(RadiusServerSchema).max(8).default([]), {
            title: 'Servers',
            itemKey: ['address', 'authPort'],
          }),
        })
        .prefault({}),
      { title: 'RADIUS', order: 2 },
    ),
    tacacs: withUi(
      z
        .strictObject({
          servers: withUi(z.array(TacacsServerSchema).max(8).default([]), {
            title: 'Servers',
            itemKey: ['address', 'port'],
          }),
        })
        .prefault({}),
      { title: 'TACACS+', order: 3 },
    ),
  })
  .refine((aaa) => new Set(aaa.order).size === aaa.order.length, {
    message: 'each authentication method may appear only once',
    path: ['order'],
  })
  .refine((aaa) => !aaa.order.includes('radius') || aaa.radius.servers.length > 0, {
    message: 'radius is in the authentication order but no RADIUS server is configured',
    path: ['radius', 'servers'],
  })
  .refine((aaa) => !aaa.order.includes('tacacs') || aaa.tacacs.servers.length > 0, {
    message: 'tacacs is in the authentication order but no TACACS+ server is configured',
    path: ['tacacs', 'servers'],
  });

export const TlsSchema = z
  .strictObject({
    certificateRef: withUi(secretRefOf('cert').optional(), {
      title: 'Certificate',
      help: 'PEM certificate chain in the secret store; absent = self-signed certificate generated on first boot',
      order: 1,
    }),
    privateKeyRef: withUi(secretRefOf('key').optional(), {
      title: 'Private key',
      help: 'matching private key in the secret store',
      order: 2,
    }),
    minVersion: withUi(z.enum(['1.2', '1.3']).default('1.2'), {
      title: 'Minimum TLS version',
      widget: 'select',
      order: 3,
    }),
  })
  .refine((tls) => (tls.certificateRef === undefined) === (tls.privateKeyRef === undefined), {
    message: 'certificateRef and privateKeyRef must be given together',
    path: ['privateKeyRef'],
  });

export const SyslogSeverity = z.enum([
  'emergency',
  'alert',
  'critical',
  'error',
  'warning',
  'notice',
  'info',
  'debug',
]);

export const SyslogServerSchema = z.strictObject({
  address: withUi(hostOrIp, { title: 'Collector', order: 1 }),
  port: withUi(portNumber.default(514), { title: 'Port', order: 2 }),
  protocol: withUi(z.enum(['udp', 'tcp', 'tls']).default('udp'), {
    title: 'Transport',
    widget: 'select',
    order: 3,
  }),
  severity: withUi(SyslogSeverity.default('info'), {
    title: 'Minimum severity',
    help: 'forward messages at this severity and above',
    widget: 'select',
    order: 4,
  }),
  vrf: withUi(vrfName.default(DEFAULT_VRF), { title: 'VRF', order: 5 }),
});

export const ManagementSchema = withUi(
  z.strictObject({
    users: withUi(z.array(UserSchema).max(256).default([]), {
      title: 'Local users',
      itemKey: ['username'],
      group: 'access',
      order: 1,
    }),
    aaa: withUi(AaaSchema.prefault({}), { title: 'AAA', group: 'access', order: 2 }),
    tls: withUi(TlsSchema.prefault({}), { title: 'TLS', group: 'api', order: 3 }),
    syslog: withUi(z.array(SyslogServerSchema).max(8).default([]), {
      title: 'Remote syslog',
      itemKey: ['address', 'port', 'protocol'],
      group: 'logging',
      order: 4,
    }),
  }),
  {
    title: 'Management',
    description: 'Local users, AAA, TLS and syslog.',
    order: 130,
  },
);

export type ManagementConfig = z.infer<typeof ManagementSchema>;
