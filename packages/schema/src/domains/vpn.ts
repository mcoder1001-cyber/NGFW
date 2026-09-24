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
 * `vpn` — VPN: IPsec (strongSwan IKE + VPP ESP, optional native VPP IKEv2), WireGuard, PKI and remote access.
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
      remoteCa: withUi(objectName, {
        title: 'Remote CA',
        help: 'Name in vpn.pki.cas trusted for the peer certificate',
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

const startAction = z.enum(['none', 'start', 'trap']);

/** `%any` = accept any responder/initiator address (responder-only tunnels). */
export const IPSEC_ANY_PEER = '%any' as const;

export const IpsecTunnelSchema = z
  .strictObject({
    enabled: enabledFlag,
    description: descriptionField.optional(),
    engine: withUi(z.enum(['strongswan', 'vpp-ikev2']).default('strongswan'), {
      title: 'IKE engine',
      widget: 'select',
      help: 'strongSwan (kernel-vpp) or the native VPP IKEv2 plugin (D6.3, IKEv2 only)',
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
      help: 'Policy-based tunnels: at least one; route-based: defaults to 0.0.0.0/0 and ::/0',
    }),
    remoteTs: withUi(z.array(ipv4OrIpv6Cidr).max(64).default([]), {
      title: 'Remote traffic selectors',
    }),
    dpd: withUi(IpsecDpdSchema, { title: 'Dead peer detection', group: 'dpd' }).prefault({}),
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
    rekey: withUi(IpsecRekeySchema, { title: 'Rekeying', group: 'rekey' }).prefault({}),
    startAction: withUi(startAction.default('start'), { title: 'Start action', widget: 'select' }),
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
      { title: 'Route-based (VTI)' },
    ).optional(),
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

export const IpsecSettingsSchema = z.strictObject({
  cryptoEngine: withUi(z.enum(['auto', 'native', 'ipsecmb', 'openssl']).default('auto'), {
    title: 'VPP crypto engine',
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
  tunnels: withUi(z.record(objectName, IpsecTunnelSchema).default({}), {
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
    help: 'VPP interface name is wg<instance>',
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

export const PkiCaSchema = z.strictObject({
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
});

export const PkiCertificateSchema = z
  .strictObject({
    description: descriptionField.optional(),
    certificateRef: withUi(secretRefOf('cert'), { title: 'Certificate (reference)' }).optional(),
    privateKeyRef: withUi(secretRefOf('key'), { title: 'Private key (reference)' }),
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
  })
  .superRefine((c, ctx) => {
    if (c.certificateRef === undefined && c.acme === undefined) {
      ctx.addIssue({
        code: 'custom',
        path: ['certificateRef'],
        message: 'a certificate needs either certificateRef or an acme block',
      });
    }
  });

export const PkiSchema = z.strictObject({
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

export const RemoteAccessProfileSchema = z
  .strictObject({
    enabled: enabledFlag,
    description: descriptionField.optional(),
    localAddr: withUi(ipAddress, {
      title: 'Local address',
      help: 'Must be configured on an interface in the underlay VRF',
    }),
    localId: ikeIdentity.optional(),
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
export type RemoteAccessProfile = z.infer<typeof RemoteAccessProfileSchema>;
