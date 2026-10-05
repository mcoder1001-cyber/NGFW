import { z } from 'zod';
import { withUi } from '../ui.js';
import { hostname, ipAddress, objectName } from '../primitives.js';
import {
  descriptionField,
  enabledFlag,
  hostOrIpAddress,
  httpsUrl,
  ikeIdentity,
  ipv4OrIpv6Cidr,
  mtuField,
  resolvableHostname,
  secretRefOf,
  transportPort,
  u32Int,
  underlayVrfRef,
  vrfRef,
  wireguardKey,
} from './_shared/primitives.js';

/**
 * `vpn` — VPN: route-based IPsec (native VPP IKEv2 + ESP), WireGuard, PKI and remote access.
 *
 * Shape (docs/04-api-datamodel.md, prompts/P11-strongswan-vpp.md §2, WBS D6.1–D6.5, D6.9):
 *   ipsec { settings, proposals { name → { ike, esp } }, tunnels { name → … } }
 *   wireguard { interfaces { name → { …, peers { name → … } } } }
 *   pki { cas { … }, certificates { … }, hsm? }
 *   remoteAccess { name → IKEv2 + EAP profile with client pools }
 *
 * Guardrails: `vrf` is first-class on every tunnel/interface/profile (vdom.md #1) and means the **overlay** (the
 * FIB the protected traffic / tunnel interface belongs to); `underlayVrf` is where the outer packets live
 * (`localAddr`, `remoteAddr`, WireGuard listen address) — the same convention as `tunnels.*`. Names are unique
 * within their record only (vdom.md #2). Secrets rule (00-CONTEXT #10, D-051): PSKs, private keys, pre-shared
 * keys, EAP passwords … are ONLY ever referenced through `<kind>/<name>` handles (`POST /api/v1/secrets` returns
 * one; the API checks existence); every object here is a `strictObject`, so an inline `psk`, `privateKey`,
 * `password` … field is rejected and a pasted secret fails the reference pattern.
 *
 * Domain root: every sub-tree is `.prefault({})` / `.default({})` (D-017, D-053), so `RootConfig.parse({}).vpn`
 * is fully populated (`ipsec.settings`, empty records). Helper primitives live in `./_shared/primitives.ts`, which
 * `index.ts` does not re-export (D-054).
 *
 * Owner: P02c. Only P02c edits this file.
 */

// ---------------------------------------------------------------------------------------------------------------
// IPsec algorithms (strongSwan keyword names; kernel-vpp maps them onto VPP crypto algorithms)
// ---------------------------------------------------------------------------------------------------------------

/** AEAD ciphers carry their own integrity — a proposal using one must not name an `integ` algorithm. */
export const IPSEC_AEAD_CIPHERS = [
  'aes128gcm8',
  'aes128gcm12',
  'aes128gcm16',
  'aes192gcm16',
  'aes256gcm8',
  'aes256gcm12',
  'aes256gcm16',
  'chacha20poly1305',
] as const;

export const IPSEC_IKE_ENCRYPTION = [
  'aes128',
  'aes192',
  'aes256',
  'aes128ctr',
  'aes256ctr',
  ...IPSEC_AEAD_CIPHERS,
  '3des',
] as const;

/** ESP additionally allows `null` encryption (integrity only). */
export const IPSEC_ESP_ENCRYPTION = [...IPSEC_IKE_ENCRYPTION, 'null'] as const;

export const IPSEC_INTEGRITY = [
  'sha1',
  'sha256',
  'sha384',
  'sha512',
  'md5',
  'aesxcbc',
  'aescmac',
] as const;

export const IPSEC_PRF = [
  'prfsha1',
  'prfsha256',
  'prfsha384',
  'prfsha512',
  'prfmd5',
  'prfaesxcbc',
  'prfaescmac',
] as const;

/** IANA DH groups 1, 2, 5, 14–26, 31, 32 by strongSwan keyword (WBS D6.2 "DH 1-24/31"). */
export const IPSEC_DH_GROUPS = [
  'modp768',
  'modp1024',
  'modp1536',
  'modp2048',
  'modp3072',
  'modp4096',
  'modp6144',
  'modp8192',
  'ecp256',
  'ecp384',
  'ecp521',
  'modp1024s160',
  'modp2048s224',
  'modp2048s256',
  'ecp192',
  'ecp224',
  'curve25519',
  'curve448',
] as const;

const aead: ReadonlySet<string> = new Set(IPSEC_AEAD_CIPHERS);

/** `integ` is required for classic ciphers and forbidden for AEAD ciphers. */
function checkIntegrity(
  v: { encr: string; integ?: string | undefined },
  ctx: z.RefinementCtx,
): void {
  if (aead.has(v.encr) && v.integ !== undefined) {
    ctx.addIssue({
      code: 'custom',
      path: ['integ'],
      message: `${v.encr} is an AEAD cipher and must not be combined with an integrity algorithm`,
    });
  }
  if (!aead.has(v.encr) && v.integ === undefined) {
    ctx.addIssue({
      code: 'custom',
      path: ['integ'],
      message: `${v.encr} needs an integrity algorithm`,
    });
  }
}

export const IkeProposalSchema = z
  .strictObject({
    encr: withUi(z.enum(IPSEC_IKE_ENCRYPTION), { title: 'Encryption', widget: 'select' }),
    integ: withUi(z.enum(IPSEC_INTEGRITY), { title: 'Integrity', widget: 'select' }).optional(),
    prf: withUi(z.enum(IPSEC_PRF), {
      title: 'PRF',
      widget: 'select',
      help: 'Defaults to the PRF matching the integrity algorithm',
    }).optional(),
    dh: withUi(z.enum(IPSEC_DH_GROUPS), { title: 'DH group', widget: 'select' }),
  })
  .superRefine(checkIntegrity);

export const EspProposalSchema = z
  .strictObject({
    encr: withUi(z.enum(IPSEC_ESP_ENCRYPTION), { title: 'Encryption', widget: 'select' }),
    integ: withUi(z.enum(IPSEC_INTEGRITY), { title: 'Integrity', widget: 'select' }).optional(),
    dh: withUi(z.enum(IPSEC_DH_GROUPS), {
      title: 'PFS DH group',
      widget: 'select',
      help: 'Perfect forward secrecy for CHILD_SA rekeying; omit to disable',
    }).optional(),
  })
  .superRefine(checkIntegrity);

/** One IKE + ESP algorithm combination, referenced by name from tunnels and remote-access profiles. */
export const IpsecProposalSchema = z.strictObject({
  description: descriptionField.optional(),
  ike: withUi(IkeProposalSchema, { title: 'IKE (phase 1)', group: 'ike' }),
  esp: withUi(EspProposalSchema, { title: 'ESP (phase 2)', group: 'esp' }),
});

// ---------------------------------------------------------------------------------------------------------------
// IPsec tunnels
// ---------------------------------------------------------------------------------------------------------------

export const IpsecAuthSchema = withUi(
  z.discriminatedUnion('method', [
    z.strictObject({
      method: z.literal('psk'),
      secretRef: withUi(secretRefOf('psk'), { title: 'Pre-shared key (reference)' }),
    }),
    z.strictObject({
      method: z.literal('cert'),
      certificate: withUi(objectName, {
        title: 'Local certificate',
        help: 'Name in vpn.pki.certificates',
      }),
      peerCertificate: withUi(objectName, {
        title: 'Pinned peer certificate',
        help: 'Peer leaf certificate name in vpn.pki.certificates; its public key verifies peer AUTH, without CA-chain or presented-certificate verification',
      }).optional(),
      remoteCa: withUi(objectName, {
        title: 'Remote CA',
        help: 'CA-chain trust is not supported by native IPsec; use peerCertificate',
      }).optional(),
    }),
  ]),
  { title: 'Authentication' },
);

export const IpsecDpdSchema = z.strictObject({
  enabled: enabledFlag,
  delaySec: withUi(z.int().min(1).max(86400).default(30), {
    title: 'DPD delay (s)',
    widget: 'number',
  }),
  timeoutSec: withUi(z.int().min(1).max(86400).default(150), {
    title: 'DPD timeout (s)',
    widget: 'number',
    help: 'IKEv1 only; IKEv2 uses the retransmission timeout',
  }),
  action: withUi(z.enum(['clear', 'trap', 'restart']).default('restart'), {
    title: 'DPD action',
    widget: 'select',
  }),
});

export const IpsecRekeySchema = z.strictObject({
  ikeSec: withUi(z.int().min(60).max(604800).default(14400), {
    title: 'IKE SA lifetime (s)',
    widget: 'number',
  }),
  espSec: withUi(z.int().min(60).max(604800).default(3600), {
    title: 'CHILD SA lifetime (s)',
    widget: 'number',
  }),
  espBytes: withUi(z.int().min(1).max(Number.MAX_SAFE_INTEGER), {
    title: 'CHILD SA lifetime (bytes)',
    widget: 'number',
  }).optional(),
  espPackets: withUi(z.int().min(1).max(Number.MAX_SAFE_INTEGER), {
    title: 'CHILD SA lifetime (packets)',
    widget: 'number',
  }).optional(),
  reauth: withUi(z.boolean().default(false), {
    title: 'Re-authenticate instead of rekeying the IKE SA',
    widget: 'switch',
  }),
});

// Native VPP configures CHILD lifetimes; IKE lifetime, reauth and packet limits
// have no supported per-profile setter. Keep historical schemas for other consumers.
export const NativeIpsecRekeySchema = IpsecRekeySchema.omit({
  ikeSec: true,
  reauth: true,
  espPackets: true,
});

const startAction = z.enum(['none', 'start', 'trap']);

/** `%any` = accept any responder/initiator address (responder-only tunnels). */
export const IPSEC_ANY_PEER = '%any' as const;

export const IpsecTunnelSchema = z
  .strictObject({
    enabled: enabledFlag,
    description: descriptionField.optional(),
    engine: withUi(z.enum(['vpp-ikev2']).default('vpp-ikev2'), {
      title: 'IKE engine',
      widget: 'select',
      help: 'Route-based IPsec with native IKEv2',
    }),
    ikeVersion: withUi(z.union([z.literal(1), z.literal(2)]).default(2), {
      title: 'IKE version',
      widget: 'select',
    }),
    mode: withUi(z.enum(['tunnel', 'transport']).default('tunnel'), {
      title: 'Mode',
      widget: 'select',
    }),
    protocol: withUi(z.enum(['esp', 'ah']).default('esp'), { title: 'Protocol', widget: 'select' }),
    localAddr: withUi(ipAddress, {
      title: 'Local address',
      help: 'Must be configured on an interface in the underlay VRF',
    }),
    remoteAddr: withUi(z.union([ipAddress, resolvableHostname, z.literal(IPSEC_ANY_PEER)]), {
      title: 'Remote address',
      help: 'IP, hostname, or %any for a responder-only tunnel',
    }),
    localId: ikeIdentity.optional(),
    remoteId: ikeIdentity.optional(),
    auth: IpsecAuthSchema,
    proposal: withUi(objectName, { title: 'Proposal', help: 'Name in vpn.ipsec.proposals' }),
    localTs: withUi(z.array(ipv4OrIpv6Cidr).max(64).default([]), {
      title: 'Local traffic selectors',
      help: 'One local network; empty uses the wildcard selector of the address family',
    }),
    remoteTs: withUi(z.array(ipv4OrIpv6Cidr).max(64).default([]), {
      title: 'Remote traffic selectors',
    }),
    natT: withUi(z.boolean().default(true), {
      title: 'NAT traversal (UDP encapsulation)',
      widget: 'switch',
    }),
    mobike: withUi(z.boolean(), {
      title: 'MOBIKE',
      widget: 'switch',
      help: 'IKEv2 only; unset = strongSwan default',
    }).optional(),
    fragmentation: withUi(z.enum(['yes', 'no', 'force', 'accept']), {
      title: 'IKE fragmentation',
      widget: 'select',
    }).optional(),
    rekey: withUi(NativeIpsecRekeySchema, { title: 'Rekeying', group: 'rekey' }).prefault({}),
    startAction: withUi(startAction.default('none'), { title: 'Start action', widget: 'select' }),
    closeAction: withUi(startAction.default('none'), { title: 'Close action', widget: 'select' }),
    vrf: withUi(vrfRef, {
      title: 'VRF',
      widget: 'vrf-picker',
      help: 'Overlay: FIB the traffic selectors apply to / the protected IPIP interface belongs to',
    }),
    underlayVrf: withUi(underlayVrfRef, {
      title: 'Underlay VRF',
      widget: 'vrf-picker',
      help: 'FIB IKE and ESP run in: localAddr must be configured on an interface in it; remoteAddr is reached through it',
    }),
    routeBased: withUi(
      z.strictObject({
        ipipInterface: withUi(objectName, {
          title: 'IPIP tunnel',
          help: 'Name in tunnels.ipip; the tunnel is protected (ipsec_tunnel_protect) and traffic is steered by routes',
        }),
      }),
      { title: 'Protected tunnel interface' },
    ),
    esn: withUi(z.boolean().default(false), {
      title: 'Extended sequence numbers',
      widget: 'switch',
    }),
    antiReplay: withUi(z.boolean().default(true), { title: 'Anti-replay', widget: 'switch' }),
  })
  .superRefine((t, ctx) => {
    const issue = (path: string, message: string): void => {
      ctx.addIssue({ code: 'custom', path: [path], message });
    };
    if (t.engine === 'vpp-ikev2') {
      if (t.enabled && t.auth.method === 'cert') {
        if (t.auth.peerCertificate === undefined)
          ctx.addIssue({
            code: 'custom',
            path: ['auth', 'peerCertificate'],
            message:
              'native certificate authentication requires an explicit pinned peer certificate',
          });
        if (t.auth.remoteCa !== undefined)
          ctx.addIssue({
            code: 'custom',
            path: ['auth', 'remoteCa'],
            message:
              'native IPsec does not verify CA chains; omit remoteCa and use peerCertificate',
          });
      }
      if (t.startAction !== 'none')
        issue('startAction', 'native IPsec initiation is an explicit runtime action; use none');
      if (t.routeBased === undefined)
        issue('routeBased', 'native IPsec requires routeBased.ipipInterface');
      if (t.mode !== 'tunnel') issue('mode', 'native IPsec supports tunnel mode only');
      if (t.protocol !== 'esp') issue('protocol', 'native IPsec supports ESP only');
      if (t.localTs.length > 1)
        issue('localTs', 'native IPsec supports one selector per direction');
      if (t.remoteTs.length > 1)
        issue('remoteTs', 'native IPsec supports one selector per direction');
      if (t.remoteAddr === IPSEC_ANY_PEER)
        issue('remoteAddr', 'native route-based IPsec needs a fixed peer address');
      for (const field of ['localId', 'remoteId'] as const) {
        const id = t[field] ?? (field === 'localId' ? t.localAddr : t.remoteAddr);
        if (/^(?:[0-9]{1,3}\.){3}[0-9]{1,3}$/.test(id)) {
          const bytes = id.split('.').map(Number);
          const firstZero = bytes.indexOf(0);
          if (firstZero >= 0 && bytes.slice(firstZero).some((b) => b !== 0)) {
            issue(
              field,
              'VPP identity read-back truncates at an embedded zero byte; use an FQDN identity',
            );
          }
        }
      }
    }
    if (t.engine === 'vpp-ikev2' && t.ikeVersion !== 2) {
      issue('ikeVersion', 'the native VPP IKEv2 engine supports IKEv2 only');
    }
    if (t.mobike === true && t.ikeVersion !== 2) issue('mobike', 'MOBIKE requires IKEv2');
    if (t.routeBased !== undefined && t.mode !== 'tunnel') {
      issue('routeBased', 'a route-based tunnel requires tunnel mode');
    }
    if (t.routeBased === undefined && (t.localTs.length === 0 || t.remoteTs.length === 0)) {
      issue(
        'localTs',
        'a policy-based tunnel needs at least one local and one remote traffic selector',
      );
    }
    if (t.remoteAddr === IPSEC_ANY_PEER && t.startAction === 'start') {
      issue('startAction', 'a tunnel to %any cannot be initiated; use trap or none');
    }
    if (t.remoteAddr === t.localAddr)
      issue('remoteAddr', 'remote address equals the local address');
    const fam = (a: string): 4 | 6 => (a.includes(':') ? 6 : 4);
    if (
      t.remoteAddr !== IPSEC_ANY_PEER &&
      ipAddress.safeParse(t.remoteAddr).success &&
      fam(t.remoteAddr) !== fam(t.localAddr)
    ) {
      issue('remoteAddr', 'local and remote addresses must be of the same address family');
    }
  });

export const ipsecObjectName = withUi(objectName.max(60), { title: 'Name' });

export const IpsecSettingsSchema = z.strictObject({
  cryptoEngine: withUi(z.enum(['auto', 'native', 'ipsecmb', 'openssl']).default('auto'), {
    title: 'Crypto engine',
    widget: 'select',
  }),
  asyncCrypto: withUi(z.boolean().default(false), {
    title: 'Asynchronous crypto',
    widget: 'switch',
    help: 'Dedicated crypto workers / QAT offload (D6.1)',
  }),
  // wave-BC: F-ikev2-native
});

export const IpsecSchema = z.strictObject({
  settings: withUi(IpsecSettingsSchema, { title: 'IPsec settings', group: 'settings' }).prefault(
    {},
  ),
  proposals: withUi(z.record(objectName, IpsecProposalSchema).default({}), {
    title: 'Proposals',
    widget: 'record',
  }),
  tunnels: withUi(z.record(ipsecObjectName, IpsecTunnelSchema).default({}), {
    title: 'Tunnels',
    widget: 'record',
  }),
});

// ---------------------------------------------------------------------------------------------------------------
// WireGuard (VPP wireguard plugin; interface `wg<instance>`)
// ---------------------------------------------------------------------------------------------------------------

export const WireguardPeerSchema = z.strictObject({
  description: descriptionField.optional(),
  publicKey: wireguardKey,
  presharedKeyRef: withUi(secretRefOf('psk'), { title: 'Pre-shared key (reference)' }).optional(),
  endpoint: withUi(
    z.strictObject({
      address: hostOrIpAddress,
      port: transportPort,
    }),
    { title: 'Endpoint', help: 'Omit for peers that always initiate' },
  ).optional(),
  allowedIps: withUi(z.array(ipv4OrIpv6Cidr).min(1).max(256), {
    title: 'Allowed IPs',
    help: 'Cryptokey routing: prefixes accepted from / routed to this peer',
  }),
  persistentKeepaliveSec: withUi(z.int().min(0).max(65535).default(0), {
    title: 'Persistent keepalive (s)',
    widget: 'number',
    help: '0 disables',
  }),
});

export const WireguardInterfaceSchema = z.strictObject({
  enabled: enabledFlag,
  description: descriptionField.optional(),
  instance: withUi(u32Int, {
    title: 'Instance',
    widget: 'number',
    help: 'Engine interface name is wg<instance>',
  }),
  vrf: vrfRef,
  underlayVrf: withUi(underlayVrfRef, {
    title: 'Underlay VRF',
    widget: 'vrf-picker',
    help: 'VRF of the listen address / peer endpoints',
  }),
  listenAddress: withUi(ipAddress, {
    title: 'Listen address',
    help: 'Source address of the tunnel; must be configured on an interface in the underlay VRF',
  }),
  listenPort: withUi(transportPort.default(51820), { title: 'Listen port' }),
  privateKeyRef: withUi(secretRefOf('key'), { title: 'Private key (reference)' }),
  address: withUi(z.array(ipv4OrIpv6Cidr).max(32).default([]), {
    title: 'Interface addresses',
    widget: 'cidr',
  }),
  mtu: mtuField.default(1420),
  peers: withUi(z.record(objectName, WireguardPeerSchema).default({}), {
    title: 'Peers',
    widget: 'record',
  }),
  routeAllowedIps: withUi(z.boolean().default(false), { title: 'Route allowed IPs', widget: 'switch', help: 'Install a route via this interface for every allowed IP of its peers (TNSR does not; F-wireguard)' }), // prettier-ignore
});

export const WireguardSchema = z.strictObject({
  interfaces: withUi(z.record(objectName, WireguardInterfaceSchema).default({}), {
    title: 'WireGuard interfaces',
    widget: 'record',
  }),
});

// ---------------------------------------------------------------------------------------------------------------
// PKI (D6.4) — certificates and keys are stored through the secrets API; the document holds references only
// ---------------------------------------------------------------------------------------------------------------

// ----- F-pki: key spec, CSR parameters and certificate facts (docs/status/wave-BC-numbers.md § F-pki) -----

/** Key algorithms of the key pairs the PKI actions generate (ECDSA P-256/P-384, RSA 2048/3072/4096). */
export const PKI_KEY_TYPES = ['ecdsa', 'rsa'] as const;
export const PKI_EC_CURVES = ['p256', 'p384'] as const;
export const PKI_RSA_BITS = [2048, 3072, 4096] as const;

/** How a key pair is (or was) generated: `curve` for ECDSA (default p256), `bits` for RSA (default 2048). */
export const PkiKeySpecSchema = z
  .strictObject({
    type: withUi(z.enum(PKI_KEY_TYPES).default('ecdsa'), { title: 'Key type', widget: 'select' }),
    curve: withUi(z.enum(PKI_EC_CURVES), {
      title: 'Curve',
      widget: 'select',
      help: 'ECDSA only (default p256)',
    }).optional(),
    bits: withUi(z.union([z.literal(2048), z.literal(3072), z.literal(4096)]), {
      title: 'Key size (bits)',
      widget: 'select',
      help: 'RSA only (default 2048)',
    }).optional(),
  })
  .superRefine((k, ctx) => {
    if (k.type === 'ecdsa' && k.bits !== undefined) {
      ctx.addIssue({ code: 'custom', path: ['bits'], message: 'bits applies to RSA keys only' });
    }
    if (k.type === 'rsa' && k.curve !== undefined) {
      ctx.addIssue({
        code: 'custom',
        path: ['curve'],
        message: 'curve applies to ECDSA keys only',
      });
    }
  });

/** Attribute types the PKI actions encode in a distinguished name (E = emailAddress). */
export const PKI_DN_ATTRIBUTES = [
  'CN',
  'O',
  'OU',
  'C',
  'L',
  'ST',
  'DC',
  'E',
  'serialNumber',
] as const;
const pkiDnAttr = `(?:${PKI_DN_ATTRIBUTES.join('|')})`;
const pkiDnValue =
  '[^ ,=+"\\\\<>;#\\x00-\\x1f\\x7f](?:[^,=+"\\\\<>;#\\x00-\\x1f\\x7f]*[^ ,=+"\\\\<>;#\\x00-\\x1f\\x7f])?';

/**
 * A distinguished name in the RFC 4514 reading order, `CN=gw.example.com, O=Example, C=CH`: attributes from
 * PKI_DN_ATTRIBUTES, values without `, = + " \ < > ; #` or control characters (no escapes — FAST MODE). One pattern for
 * the three consumers (no lookaround: valid RE2 and Python `re`).
 */
export const pkiDistinguishedName = withUi(
  z
    .string()
    .min(4)
    .max(1024)
    .regex(
      new RegExp(`^${pkiDnAttr}=${pkiDnValue}(?:, ?${pkiDnAttr}=${pkiDnValue})*$`),
      'expected a distinguished name like CN=gw.example.com, O=Example, C=CH',
    ),
  { title: 'Distinguished name' },
);

/** A DNS name for a subjectAltName: an RFC 1123 hostname, optionally with a leading `*.` wildcard label. */
const pkiDnsName = z
  .string()
  .min(1)
  .max(253)
  .regex(
    /^(?:\*\.)?[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$/,
    'expected a DNS name (a leading *. wildcard is allowed)',
  );

/** A subject alternative name: an IP address (iPAddress), an e-mail address (rfc822Name) or a DNS name (dNSName). */
export const pkiSubjectAltName = withUi(z.union([ipAddress, z.email(), pkiDnsName]), {
  title: 'Subject alternative name',
});

/** The request parameters of a key pair + CSR made by `POST /api/v1/actions/pki/csr` (kept for re-issue). */
export const PkiCsrSchema = z.strictObject({
  subject: withUi(pkiDistinguishedName, { title: 'Subject' }),
  san: withUi(z.array(pkiSubjectAltName).max(64).default([]), {
    title: 'Subject alternative names',
    help: 'DNS names, IP addresses or e-mail addresses',
  }),
  keySpec: withUi(PkiKeySpecSchema, { title: 'Key' }).prefault({}),
});

/** Colon-separated upper-case hex bytes (a serial number of up to 32 bytes, a SHA-256 fingerprint). */
const pkiHexBytes = (max: number) => new RegExp(`^[0-9A-F]{2}(?::[0-9A-F]{2}){0,${max - 1}}$`);

/**
 * Facts of the certificate behind `certificateRef`, recorded by the PKI actions next to the reference (read-only in the
 * UI; `GET /api/v1/state/pki` re-reads them from the material). The rules below use them: a CA must be CA:TRUE and
 * `expiryAlertDays` must be shorter than the validity.
 */
export const PkiIssuedSchema = z.strictObject({
  subject: withUi(z.string().min(1).max(1024), { title: 'Subject' }),
  issuer: withUi(z.string().min(1).max(1024), { title: 'Issuer' }),
  serial: withUi(z.string().max(95).regex(pkiHexBytes(32), 'expected colon-separated hex bytes'), {
    title: 'Serial number',
  }),
  notBefore: withUi(z.iso.datetime({ offset: true }), { title: 'Valid from', widget: 'datetime' }),
  notAfter: withUi(z.iso.datetime({ offset: true }), { title: 'Valid until', widget: 'datetime' }),
  fingerprint: withUi(
    z.string().length(95).regex(pkiHexBytes(32), 'expected a colon-separated SHA-256 fingerprint'),
    { title: 'SHA-256 fingerprint' },
  ),
  ca: withUi(z.boolean(), {
    title: 'CA certificate (basicConstraints CA:TRUE)',
    widget: 'switch',
  }).optional(),
});

/** Whole days between notBefore and notAfter (NaN when either does not parse). */
function pkiValidityDays(issued: { notBefore: string; notAfter: string }): number {
  return (Date.parse(issued.notAfter) - Date.parse(issued.notBefore)) / 86_400_000;
}

// ----- end F-pki -----

export const PkiCaSchema = z
  .strictObject({
    description: descriptionField.optional(),
    certificateRef: withUi(secretRefOf('cert'), {
      title: 'CA certificate (reference)',
      help: 'PEM stored through POST /api/v1/secrets (kind cert)',
    }),
    crl: withUi(
      z.strictObject({
        url: httpsUrl.optional(),
        refreshIntervalSec: withUi(z.int().min(300).max(2592000).default(86400), {
          title: 'CRL refresh (s)',
          widget: 'number',
        }),
      }),
      { title: 'CRL' },
    ).optional(),
    ocspUrl: withUi(httpsUrl, { title: 'OCSP responder URL' }).optional(),
    // wave-BC: F-pki
    keySpec: withUi(PkiKeySpecSchema, {
      title: 'Key',
      help: 'How the CA key pair was generated (POST /api/v1/actions/pki/ca)',
    }).optional(),
    issued: withUi(PkiIssuedSchema, {
      title: 'Certificate facts',
      help: 'Recorded by the PKI actions',
    }).optional(),
  })
  .superRefine((c, ctx) => {
    // F-pki: a CA certificate must be CA:TRUE (the import/generate actions record basicConstraints in issued.ca)
    if (c.issued?.ca === false) {
      ctx.addIssue({
        code: 'custom',
        path: ['issued', 'ca'],
        message:
          'a CA certificate must have basicConstraints CA:TRUE; import it as a certificate instead',
      });
    }
  });

export const PkiCertificateSchema = z
  .strictObject({
    description: descriptionField.optional(),
    certificateRef: withUi(secretRefOf('cert'), { title: 'Certificate (reference)' }).optional(),
    privateKeyRef: withUi(secretRefOf('key'), { title: 'Private key (reference)' }).optional(),
    ca: withUi(objectName, { title: 'Issuing CA', help: 'Name in vpn.pki.cas' }).optional(),
    acme: withUi(
      z.strictObject({
        directoryUrl: httpsUrl.default('https://acme-v02.api.letsencrypt.org/directory'),
        domains: withUi(z.array(hostname).min(1).max(100), { title: 'Domains' }),
        email: withUi(z.email(), { title: 'Account e-mail' }).optional(),
        challenge: withUi(z.enum(['http-01', 'dns-01']).default('http-01'), {
          title: 'Challenge',
          widget: 'select',
        }),
      }),
      { title: 'ACME', help: 'Certificate is obtained/renewed automatically' },
    ).optional(),
    expiryAlertDays: withUi(z.int().min(1).max(365).default(30), {
      title: 'Expiry alert (days before)',
      widget: 'number',
    }),
    // wave-BC: F-pki
    csr: withUi(PkiCsrSchema, {
      title: 'CSR',
      help: 'Subject, SANs and key of the request (POST /api/v1/actions/pki/csr)',
    }).optional(),
    issued: withUi(PkiIssuedSchema, {
      title: 'Certificate facts',
      help: 'Recorded by the PKI actions',
    }).optional(),
  })
  .superRefine((c, ctx) => {
    if (c.certificateRef === undefined && c.acme === undefined) {
      ctx.addIssue({
        code: 'custom',
        path: ['certificateRef'],
        message: 'a certificate needs either certificateRef or an acme block',
      });
    }
    // F-pki: the alert must fire before the certificate expires (validity from the recorded facts)
    if (c.issued !== undefined) {
      const days = pkiValidityDays(c.issued);
      if (Number.isFinite(days) && c.expiryAlertDays >= days) {
        ctx.addIssue({
          code: 'custom',
          path: ['expiryAlertDays'],
          message: `expiryAlertDays (${c.expiryAlertDays}) must be shorter than the certificate's validity (${Math.floor(days)} days)`,
        });
      }
    }
  });

export const PkiSchema = z
  .strictObject({
    cas: withUi(z.record(objectName, PkiCaSchema).default({}), {
      title: 'Certificate authorities',
      widget: 'record',
    }),
    certificates: withUi(z.record(objectName, PkiCertificateSchema).default({}), {
      title: 'Certificates',
      widget: 'record',
    }),
    hsm: withUi(
      z.strictObject({
        enabled: withUi(z.boolean().default(false), { title: 'Enabled', widget: 'switch' }),
        module: withUi(
          z
            .string()
            .max(255)
            .regex(/^\/[A-Za-z0-9_.+/-]*$/),
          {
            title: 'PKCS#11 module path',
          },
        ),
        tokenLabel: withUi(z.string().min(1).max(32), { title: 'Token label' }).optional(),
        pinRef: withUi(secretRefOf('password'), { title: 'PIN (reference)' }).optional(),
      }),
      { title: 'PKCS#11 / HSM' },
    ).optional(),
  })
  .superRefine((pki, ctx) => {
    // F-pki: a certificate's `ca` must be the CA that issued it, when both facts are recorded
    for (const [name, cert] of Object.entries(pki.certificates)) {
      const ca = cert.ca === undefined ? undefined : pki.cas[cert.ca];
      if (ca?.issued === undefined || cert.issued === undefined) continue;
      if (cert.issued.issuer !== ca.issued.subject) {
        ctx.addIssue({
          code: 'custom',
          path: ['certificates', name, 'ca'],
          message: `certificate '${name}' was issued by '${cert.issued.issuer}', not by CA '${cert.ca}' ('${ca.issued.subject}')`,
        });
      }
    }
  });

// ---------------------------------------------------------------------------------------------------------------
// Remote-access VPN (D6.9): IKEv2 + EAP + client pools
// ---------------------------------------------------------------------------------------------------------------

export const RemoteAccessPoolSchema = z.strictObject({
  name: objectName,
  prefix: withUi(ipv4OrIpv6Cidr, { title: 'Client pool prefix' }),
  dns: withUi(z.array(ipAddress).max(4).default([]), { title: 'DNS servers pushed to clients' }),
});

export const RemoteAccessUserSchema = z.strictObject({
  username: withUi(
    z
      .string()
      .min(1)
      .max(64)
      .regex(/^[A-Za-z0-9][A-Za-z0-9_.@-]*$/),
    {
      title: 'Username',
    },
  ),
  passwordRef: withUi(secretRefOf('password'), { title: 'Password (reference)' }),
  // wave-BC: F-ra-vpn
});

/** Explicit addresses for one private-namespace/VPP transit link. */
export const RemoteAccessTransitLinkSchema = z.strictObject({
  vpp: ipv4OrIpv6Cidr,
  namespace: ipv4OrIpv6Cidr,
});
export const RemoteAccessTransportSchema = z.strictObject({
  outer: RemoteAccessTransitLinkSchema,
  inner: RemoteAccessTransitLinkSchema,
  // IPv6 pool forwarding needs a separately addressed IPv6 inner link.
  innerIpv6: RemoteAccessTransitLinkSchema.optional(),
});

export const RemoteAccessProfileSchema = z
  .strictObject({
    enabled: enabledFlag,
    description: descriptionField.optional(),
    localAddr: withUi(ipAddress, {
      title: 'Local address',
      help: 'Dedicated routed endpoint owned by the private remote-access namespace',
    }),
    localId: ikeIdentity.optional(),
    transport: RemoteAccessTransportSchema.optional(),
    accessPolicy: z
      .strictObject({
        ingress: z.array(objectName).min(1).max(32),
        egress: z.array(objectName).min(1).max(32),
      })
      .optional(),
    outerPolicy: z.strictObject({
      ingress: z.array(objectName).min(1).max(32),
      egress: z.array(objectName).min(1).max(32),
    }).optional(),
    vrf: withUi(vrfRef, {
      title: 'VRF',
      widget: 'vrf-picker',
      help: 'Overlay: FIB the client pools are routed in',
    }),
    underlayVrf: withUi(underlayVrfRef, {
      title: 'Underlay VRF',
      widget: 'vrf-picker',
      help: 'FIB IKE and ESP run in (localAddr, client endpoints)',
    }),
    auth: withUi(
      z.enum(['eap-mschapv2', 'eap-tls', 'eap-radius', 'pubkey']).default('eap-mschapv2'),
      {
        title: 'Client authentication',
        widget: 'select',
      },
    ),
    certificate: withUi(objectName, {
      title: 'Server certificate',
      help: 'Name in vpn.pki.certificates',
    }),
    clientCa: withUi(objectName, {
      title: 'Client CA',
      help: 'Name in vpn.pki.cas (eap-tls / pubkey)',
    }).optional(),
    proposal: withUi(objectName, { title: 'Proposal', help: 'Name in vpn.ipsec.proposals' }),
    pools: withUi(z.array(RemoteAccessPoolSchema).min(1).max(16), { title: 'Client pools' }),
    splitTunnel: withUi(z.array(ipv4OrIpv6Cidr).max(64).default([]), {
      title: 'Split-tunnel prefixes',
      help: 'Empty = full tunnel (0.0.0.0/0, ::/0)',
    }),
    users: withUi(z.array(RemoteAccessUserSchema).max(1024).default([]), {
      title: 'Local EAP users',
    }),
    radius: withUi(
      z.strictObject({
        servers: withUi(
          z
            .array(
              z.strictObject({
                address: hostOrIpAddress,
                port: transportPort.default(1812),
                secretRef: withUi(secretRefOf('psk'), {
                  title: 'RADIUS shared secret (reference)',
                }),
              }),
            )
            .min(1)
            .max(8),
          { title: 'RADIUS servers' },
        ),
      }),
      { title: 'RADIUS' },
    ).optional(),
    dpd: withUi(IpsecDpdSchema, { title: 'Dead peer detection', group: 'dpd' }).prefault({}),
    rekey: withUi(IpsecRekeySchema, { title: 'Rekeying', group: 'rekey' }).prefault({}),
    // wave-BC: F-ra-vpn
  })
  .superRefine((p, ctx) => {
    const issue = (path: string, message: string): void => {
      ctx.addIssue({ code: 'custom', path: [path], message });
    };
    if (p.auth === 'eap-radius' && p.radius === undefined) {
      issue('radius', 'eap-radius needs at least one RADIUS server');
    }
    if (p.auth === 'eap-mschapv2' && p.users.length === 0 && p.radius === undefined) {
      issue('users', 'eap-mschapv2 needs local users or a RADIUS block');
    }
    if ((p.auth === 'eap-tls' || p.auth === 'pubkey') && p.clientCa === undefined) {
      issue('clientCa', `${p.auth} needs a client CA`);
    }
    const seen = new Set<string>();
    p.users.forEach((u, i) => {
      if (seen.has(u.username)) {
        ctx.addIssue({
          code: 'custom',
          path: ['users', i, 'username'],
          message: `duplicate user '${u.username}'`,
        });
      }
      seen.add(u.username);
    });
    const poolNames = new Set<string>();
    p.pools.forEach((pool, i) => {
      if (poolNames.has(pool.name)) {
        ctx.addIssue({
          code: 'custom',
          path: ['pools', i, 'name'],
          message: `duplicate pool '${pool.name}'`,
        });
      }
      poolNames.add(pool.name);
    });
  });

// ---------------------------------------------------------------------------------------------------------------
// Domain root
// ---------------------------------------------------------------------------------------------------------------

export const VpnSchema = withUi(
  z.strictObject({
    ipsec: withUi(IpsecSchema.prefault({}), { title: 'IPsec', group: 'ipsec', order: 1 }),
    wireguard: withUi(WireguardSchema.prefault({}), {
      title: 'WireGuard',
      group: 'wireguard',
      order: 2,
    }),
    pki: withUi(PkiSchema.prefault({}), { title: 'PKI', group: 'pki', order: 3 }),
    remoteAccess: withUi(z.record(objectName, RemoteAccessProfileSchema).default({}), {
      title: 'Remote access (IKEv2 + EAP)',
      group: 'remote-access',
      order: 4,
      widget: 'record',
    }),
  }),
  {
    title: 'VPN',
    description: 'IPsec (strongSwan / native IKEv2), WireGuard, PKI and remote-access VPN.',
    order: 90,
  },
);

export type VpnConfig = z.infer<typeof VpnSchema>;
export type IpsecProposal = z.infer<typeof IpsecProposalSchema>;
export type IpsecTunnel = z.infer<typeof IpsecTunnelSchema>;
export type IpsecAuth = z.infer<typeof IpsecAuthSchema>;
export type WireguardInterface = z.infer<typeof WireguardInterfaceSchema>;
export type WireguardPeer = z.infer<typeof WireguardPeerSchema>;
export type PkiCa = z.infer<typeof PkiCaSchema>;
export type PkiCertificate = z.infer<typeof PkiCertificateSchema>;
export type PkiKeySpec = z.infer<typeof PkiKeySpecSchema>; // F-pki
export type PkiCsr = z.infer<typeof PkiCsrSchema>; // F-pki
export type PkiIssued = z.infer<typeof PkiIssuedSchema>; // F-pki
export type RemoteAccessProfile = z.infer<typeof RemoteAccessProfileSchema>;
