import { z } from 'zod';
import { secretRefOf } from '../../primitives.js';
import { withUi } from '../../ui.js';

/**
 * F-aaa: external AAA sub-schemas (LDAP, MFA, role mapping, local fallback) that widen `management.aaa`. RADIUS and
 * TACACS+ already live in `management.ts`. OIDC (F-aaa-login) is the browser single sign-on; SAML stays reserved
 * (envelope field 6). Bind passwords and client secrets are `password/<name>` / `token/<name>` secret references,
 * never inline. Everything is enforced in the API (D-040).
 */

const GROUP = 'aaa';

/** LDAP / LDAPS server used to authenticate management logins (bind + search). */
export const AaaLdapServerSchema = z.strictObject({
  url: withUi(
    z
      .string()
      .max(255)
      .regex(/^ldaps?:\/\/[^\s]+$/, 'expected an ldap:// or ldaps:// URL'),
    {
      title: 'Server URL',
      help: 'ldaps://host:636 (preferred) or ldap://host:389 with StartTLS',
      order: 1,
    },
  ),
  bindDn: withUi(z.string().min(1).max(255), {
    title: 'Bind DN',
    help: 'DN the box binds as to search for the user (a read-only service account)',
    order: 2,
  }),
  bindPasswordRef: withUi(secretRefOf('password'), {
    title: 'Bind password',
    help: 'password/<name> secret for the bind DN; never the password itself',
    order: 3,
  }),
  baseDn: withUi(z.string().min(1).max(255), {
    title: 'Base DN',
    help: 'subtree the user search starts from',
    order: 4,
  }),
  userFilter: withUi(z.string().min(1).max(255).default('(uid=%s)'), {
    title: 'User filter',
    help: 'LDAP filter; %s is replaced with the (escaped) login name, e.g. (uid=%s) or (sAMAccountName=%s)',
    order: 5,
  }),
  groupAttr: withUi(z.string().max(64).default('memberOf'), {
    title: 'Group attribute',
    help: 'attribute on the user entry that lists group DNs (mapped to roles via roleMap)',
    order: 6,
  }),
  startTls: withUi(z.boolean().default(false), {
    title: 'StartTLS',
    help: 'upgrade an ldap:// connection to TLS before binding (not needed for ldaps://)',
    order: 7,
  }),
});
export type AaaLdapServer = z.infer<typeof AaaLdapServerSchema>;

export const AaaLdapSchema = z.strictObject({
  servers: withUi(z.array(AaaLdapServerSchema).max(4).default([]), {
    title: 'Servers',
    itemKey: ['url'],
  }),
});

/** External group → local role. An external identity with no mapped group is rejected (least privilege). */
export const AaaRoleMappingSchema = z.strictObject({
  group: withUi(z.string().min(1).max(255), {
    title: 'External group',
    help: 'group name or DN from the identity source',
    order: 1,
  }),
  role: withUi(z.enum(['admin', 'operator', 'readonly']), {
    title: 'Role',
    widget: 'select',
    order: 2,
  }),
});
export type AaaRoleMapping = z.infer<typeof AaaRoleMappingSchema>;

/** Multi-factor policy (TOTP). Enrolment/verification is per user, in the API. */
export const AaaMfaSchema = z.strictObject({
  required: withUi(z.enum(['none', 'admins', 'all']).default('none'), {
    title: 'Require MFA',
    widget: 'select',
    help: 'none; admins (admin role only); all users',
    order: 1,
  }),
  issuer: withUi(
    z
      .string()
      .max(64)
      .regex(/^[\x20-\x7e]+$/, 'printable ASCII')
      .default('vrx'),
    {
      title: 'Issuer',
      help: 'label shown in the authenticator app',
      order: 2,
    },
  ),
});
export type AaaMfa = z.infer<typeof AaaMfaSchema>;

export const aaaLdapField = withUi(AaaLdapSchema.prefault({}), {
  title: 'LDAP',
  group: GROUP,
  order: 4,
});
export const aaaRoleMapField = withUi(z.array(AaaRoleMappingSchema).max(64).default([]), {
  title: 'Role mapping',
  help: 'map external groups to local roles; an external user with no mapped group is refused',
  group: GROUP,
  order: 7,
});
export const aaaMfaField = withUi(AaaMfaSchema.prefault({}), {
  title: 'MFA',
  group: GROUP,
  order: 8,
});
export const aaaFallbackLocalField = withUi(z.boolean().default(true), {
  title: 'Local fallback',
  help: 'allow local-user login when every external server is unreachable',
  group: GROUP,
  order: 9,
});

/**
 * http(s) URL; plain http only for a loopback host (a local development IdP) — a remote IdP is always https, so the
 * authorisation code and the client secret never cross the network in clear.
 */
const oidcUrl = z
  .string()
  .max(255)
  .regex(
    /^(https:\/\/[^\s]+|http:\/\/(127\.0\.0\.1|localhost|\[::1\])(:[0-9]+)?(\/[^\s]*)?)$/,
    'expected an https:// URL (http:// only for 127.0.0.1/localhost)',
  );

/** F-aaa-login: OpenID Connect (authorisation code + PKCE) single sign-on for the web UI. */
export const AaaOidcSchema = z.strictObject({
  issuer: withUi(oidcUrl, {
    title: 'Issuer',
    help: 'the IdP issuer URL; /.well-known/openid-configuration is read from it',
    order: 1,
  }),
  clientId: withUi(z.string().min(1).max(255), { title: 'Client ID', order: 2 }),
  clientSecretRef: withUi(secretRefOf('token'), {
    title: 'Client secret',
    help: 'token/<name> secret with the client secret; never the secret itself',
    order: 3,
  }),
  redirectUri: withUi(oidcUrl, {
    title: 'Redirect URI',
    help: 'registered at the IdP: https://<this box>/api/v1/auth/oidc/callback',
    order: 4,
  }),
  scopes: withUi(
    z
      .array(z.string().regex(/^[\x21\x23-\x5b\x5d-\x7e]{1,64}$/, 'a scope token'))
      .max(16)
      .default(['openid', 'profile', 'email']),
    { title: 'Scopes', help: 'must include openid', order: 5 },
  ),
  usernameClaim: withUi(z.string().min(1).max(64).default('preferred_username'), {
    title: 'Username claim',
    help: 'ID-token claim used as the login name',
    order: 6,
  }),
  roleClaim: withUi(z.string().min(1).max(64).default('groups'), {
    title: 'Group claim',
    help: 'ID-token claim listing the user’s groups (mapped to roles via roleMap)',
    order: 7,
  }),
});
export type AaaOidc = z.infer<typeof AaaOidcSchema>;

export const aaaOidcField = withUi(AaaOidcSchema.optional(), {
  title: 'OpenID Connect',
  group: GROUP,
  order: 5,
});
