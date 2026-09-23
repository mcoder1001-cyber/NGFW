import { z } from 'zod';
import { withUi } from '../../ui.js';
import { hostname, ipAddress, ipv4Cidr, ipv6Cidr, objectName } from '../../primitives.js';

/**
 * Group-(c) helper primitives (vpn, tunnels, services, ha) — **not part of the package API** (D-054).
 *
 * `index.ts` re-exports the domain files with `export *`; this module is imported by them but never re-exported,
 * so nothing here is visible to consumers of `@ngfw/schema`. That keeps the later dedupe with P02a's
 * `primitives.ts` (`secretRef`, `hostOrIp`, `mtu`, `portNumber`, `ipCidr`, `uint32`, `descriptionText`) an
 * internal refactor rather than a breaking change: only the *field names and JSON Schema* of the domains are
 * contractual. Owner: P02c.
 */

// ---------------------------------------------------------------------------------------------------------------
// Secret references (D-051)
// ---------------------------------------------------------------------------------------------------------------

/**
 * Kinds of entries in the secret store (`secret(kind, ref)` in docs/04): `psk` pre-shared / shared secrets,
 * `key` private and symmetric keys, `cert` certificates (PEM, public), `password` passphrases, PINs and
 * community strings, `token` bearer / enrolment tokens.
 */
export const SECRET_KINDS = ['psk', 'key', 'cert', 'password', 'token'] as const;
export type SecretKind = (typeof SECRET_KINDS)[number];

const SECRET_NAME = '[A-Za-z0-9][A-Za-z0-9_.-]{0,62}';

/**
 * Reference to a stored secret: exactly `<kind>/<name>` with an enumerated kind (D-051). Returned by
 * `POST /api/v1/secrets`; the schema validates the **format** only, the API validates that the reference
 * **exists** — the pattern makes pasting the secret itself (`MyS3cretPSK2026`, a hex or base64 key, a PEM block)
 * a schema error, it is a structural guard, not the secrecy mechanism.
 *
 * P02a's `primitives.ts` will provide the shared `secretRef` / `secretRefOf(kind)`; this local copy has the same
 * shape and is swapped for it in the dedupe (D-054).
 */
export function secretRefOf(kind: SecretKind | readonly SecretKind[]) {
  const kinds = typeof kind === 'string' ? [kind] : kind;
  const alternatives = kinds.join('|');
  return withUi(
    z
      .string()
      .min(1)
      .max(127)
      .regex(
        new RegExp(`^(?:${alternatives})/${SECRET_NAME}$`),
        `expected a secret reference like ${kinds[0]}/<name> (POST /api/v1/secrets returns one), never the secret itself`,
      ),
    {
      title: 'Secret reference',
      widget: 'secret-ref',
      help: `Reference to a stored secret of kind ${kinds.join(' or ')}. The secret itself is never part of the configuration.`,
    },
  );
}

/** A secret reference of any kind (`psk/…`, `key/…`, `cert/…`, `password/…`, `token/…`). */
export const secretReference = secretRefOf(SECRET_KINDS);

// ---------------------------------------------------------------------------------------------------------------
// Network primitives
// ---------------------------------------------------------------------------------------------------------------

/** TCP/UDP port 1–65535. */
export const transportPort = withUi(z.int().min(1).max(65535), { title: 'Port', widget: 'number' });

/**
 * RFC 1123 hostname that is *not* all digits and dots — `203.0.113.999` is a mistyped address, not a name to
 * resolve at apply time (review F12).
 */
export const resolvableHostname = hostname.refine(
  (v) => !/^[0-9.]+$/.test(v),
  'expected an IP address or a hostname (an all-numeric label sequence is a mistyped address)',
);

/** An IP address or an RFC 1123 hostname (resolved by the agent at apply time). */
export const hostOrIpAddress = withUi(z.union([ipAddress, resolvableHostname]), {
  title: 'Address or hostname',
});

/** An IPv4 or IPv6 prefix in CIDR notation. */
export const ipv4OrIpv6Cidr = withUi(z.union([ipv4Cidr, ipv6Cidr]), {
  title: 'IPv4 or IPv6 CIDR',
  widget: 'cidr',
});

/**
 * WireGuard public key: base64 of exactly 32 bytes → 44 characters, the last group padded with one `=` and its
 * third character restricted to the values a 32-byte input can produce. Private and pre-shared keys are secrets
 * and go through `key/…` / `psk/…` references.
 */
export const wireguardKey = withUi(
  z
    .string()
    .length(44)
    .regex(
      /^[A-Za-z0-9+/]{42}[AEIMQUYcgkosw048]=$/,
      'expected a base64-encoded 32-byte WireGuard key (44 characters ending in =)',
    ),
  { title: 'WireGuard public key' },
);

/** DNS owner name: labels of letters/digits/`_`/`-`, optional leading `*.`, optional trailing dot, or `.` (root). */
export const dnsName = withUi(
  z
    .string()
    .min(1)
    .max(253)
    .regex(
      /^(?:\.|(?:\*\.)?(?:[A-Za-z0-9_][A-Za-z0-9_-]{0,62}\.)*[A-Za-z0-9_][A-Za-z0-9_-]{0,62}\.?)$/,
      'expected a DNS name like example.com, _sip._tcp.example.com or .',
    ),
  { title: 'DNS name' },
);

// ---------------------------------------------------------------------------------------------------------------
// Common object fields
// ---------------------------------------------------------------------------------------------------------------

/** Free text ≤ 255 without control characters (D-049: nothing that reaches a daemon config may carry CR/LF). */
// eslint-disable-next-line no-control-regex
const NO_CONTROL_CHARS = /^[^\x00-\x1f\x7f-\x9f]*$/;

export const descriptionField = withUi(
  z.string().max(255).regex(NO_CONTROL_CHARS, 'control characters are not allowed'),
  {
    title: 'Description',
    widget: 'textarea',
  },
);

export const enabledFlag = withUi(z.boolean().default(true), {
  title: 'Enabled',
  widget: 'switch',
});

/** The VRF an object belongs to (vdom.md #1: first-class, never implicit). */
export const vrfRef = withUi(objectName.default('default'), {
  title: 'VRF',
  widget: 'vrf-picker',
  help: 'Routing table this object belongs to (first-class, never implicit)',
});

/**
 * The VRF the *encapsulated / outer* packets live in: tunnel `src`/`dst`, IPsec `localAddr`/`remoteAddr`,
 * WireGuard listen address and peer endpoints. `vrf` is then the overlay (the tunnel interface's FIB, or the
 * FIB the traffic selectors apply to).
 */
export const underlayVrfRef = withUi(objectName.default('default'), {
  title: 'Underlay VRF',
  widget: 'vrf-picker',
  help: 'FIB the outer (encapsulated) packets are looked up in; local/source addresses must be configured on an interface in it',
});

export const u32Int = z.int().min(0).max(4294967295);

export const mtuField = withUi(z.int().min(68).max(9216), { title: 'MTU', widget: 'number' });

export const httpsUrl = withUi(
  z.url({ protocol: /^https?$/, hostname: z.regexes.domain }).max(2048),
  {
    title: 'URL',
  },
);

/**
 * IKE identity: FQDN, e-mail, IP address, `@keyid` or an X.509 DN (`C=CH, O=Example, CN=peer`). Printable ASCII
 * without `"` or control characters, so it can be rendered into swanctl.conf with strict escaping.
 */
export const ikeIdentity = withUi(
  z
    .string()
    .min(1)
    .max(255)
    .regex(/^[!#-~][ !#-~]*$/, 'printable ASCII without double quotes or control characters'),
  { title: 'IKE identity', help: 'FQDN, e-mail, IP address, @keyid or X.509 DN' },
);
