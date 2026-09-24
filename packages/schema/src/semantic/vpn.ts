import type { RootConfig } from '../index.js';
import { IPSEC_AEAD_CIPHERS, IPSEC_ANY_PEER } from '../domains/vpn.js';
import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';
import {
  addressConfiguredInVrf,
  type Cidr,
  cidrsOverlap,
  interfaceIndex,
  knownVrfs,
  parseCidr,
  walkStrings,
} from './tunnels-common.js';

/**
 * Semantic validators for `vpn` (tier (b), cross-object rules the Zod schema cannot express):
 * proposals / PKI objects / IPIP tunnels referenced by name exist, `vrf` (overlay) and `underlayVrf` exist, local
 * addresses are configured on an interface in the **underlay** VRF, route-based tunnels and their IPIP interface
 * agree on both VRFs, no duplicate peers (F6: several `%any` responders on one address are fine when their IDs or
 * auth differ) / WireGuard instances / listen sockets / peer keys, remote-access pools do not overlap, and no key
 * material anywhere in the group-(c) subtrees; WireGuard addresses do not overlap other interfaces of their VRF
 * (F10); proposals fit the tunnel's protocol and IKE version (F14). Pointers are built with `jsonPointer()`. Owner: P02c.
 */

const P = (...segments: (string | number)[]): string => jsonPointer('vpn', ...segments);

function tunnelsOf(config: RootConfig) {
  return Object.entries(config.vpn.ipsec.tunnels);
}

function remoteAccessOf(config: RootConfig) {
  return Object.entries(config.vpn.remoteAccess);
}

function wireguardOf(config: RootConfig) {
  return Object.entries(config.vpn.wireguard.interfaces);
}

const proposalExists: ValidatorDefinition = {
  name: 'vpn.proposal-exists',
  domains: ['vpn'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const proposals = config.vpn.ipsec.proposals;
    const check = (pointer: string, name: string): void => {
      if (!(name in proposals)) {
        issues.push({ pointer, message: `IPsec proposal '${name}' does not exist` });
      }
    };
    for (const [name, t] of tunnelsOf(config))
      check(P('ipsec', 'tunnels', name, 'proposal'), t.proposal);
    for (const [name, r] of remoteAccessOf(config))
      check(P('remoteAccess', name, 'proposal'), r.proposal);
    return issues;
  },
};

const pkiReferenceExists: ValidatorDefinition = {
  name: 'vpn.pki-reference-exists',
  domains: ['vpn'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const cas = config.vpn.pki.cas;
    const certificates = config.vpn.pki.certificates;
    const needCert = (pointer: string, name: string): void => {
      if (!(name in certificates)) {
        issues.push({
          pointer,
          message: `certificate '${name}' does not exist in vpn.pki.certificates`,
        });
      }
    };
    const needCa = (pointer: string, name: string): void => {
      if (!(name in cas))
        issues.push({ pointer, message: `CA '${name}' does not exist in vpn.pki.cas` });
    };
    for (const [name, t] of tunnelsOf(config)) {
      if (t.auth.method === 'cert') {
        needCert(P('ipsec', 'tunnels', name, 'auth', 'certificate'), t.auth.certificate);
        if (t.auth.remoteCa !== undefined)
          needCa(P('ipsec', 'tunnels', name, 'auth', 'remoteCa'), t.auth.remoteCa);
      }
    }
    for (const [name, r] of remoteAccessOf(config)) {
      needCert(P('remoteAccess', name, 'certificate'), r.certificate);
      if (r.clientCa !== undefined) needCa(P('remoteAccess', name, 'clientCa'), r.clientCa);
    }
    for (const [name, c] of Object.entries(certificates)) {
      if (c.ca !== undefined) needCa(P('pki', 'certificates', name, 'ca'), c.ca);
    }
    return issues;
  },
};

const vrfExists: ValidatorDefinition = {
  name: 'vpn.vrf-exists',
  domains: ['vpn', 'vrfs'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const vrfs = knownVrfs(config);
    const check = (pointer: string, vrf: string): void => {
      if (!vrfs.has(vrf)) issues.push({ pointer, message: `VRF '${vrf}' does not exist` });
    };
    for (const [name, t] of tunnelsOf(config)) {
      check(P('ipsec', 'tunnels', name, 'vrf'), t.vrf);
      check(P('ipsec', 'tunnels', name, 'underlayVrf'), t.underlayVrf);
    }
    for (const [name, r] of remoteAccessOf(config)) {
      check(P('remoteAccess', name, 'vrf'), r.vrf);
      check(P('remoteAccess', name, 'underlayVrf'), r.underlayVrf);
    }
    for (const [name, w] of wireguardOf(config)) {
      check(P('wireguard', 'interfaces', name, 'vrf'), w.vrf);
      check(P('wireguard', 'interfaces', name, 'underlayVrf'), w.underlayVrf);
    }
    return issues;
  },
};

const localAddressConfigured: ValidatorDefinition = {
  name: 'vpn.local-address-configured',
  domains: ['vpn', 'interfaces', 'tunnels'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const index = interfaceIndex(config);
    const check = (pointer: string, address: string, vrf: string): void => {
      if (!addressConfiguredInVrf(index, address, vrf)) {
        issues.push({
          pointer,
          message: `${address} is not configured on any interface in VRF '${vrf}'`,
        });
      }
    };
    for (const [name, t] of tunnelsOf(config))
      check(P('ipsec', 'tunnels', name, 'localAddr'), t.localAddr, t.underlayVrf);
    for (const [name, r] of remoteAccessOf(config))
      check(P('remoteAccess', name, 'localAddr'), r.localAddr, r.underlayVrf);
    for (const [name, w] of wireguardOf(config)) {
      check(P('wireguard', 'interfaces', name, 'listenAddress'), w.listenAddress, w.underlayVrf);
    }
    return issues;
  },
};

const routeBasedIpip: ValidatorDefinition = {
  name: 'vpn.route-based-ipip',
  domains: ['vpn', 'tunnels'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const ipips = config.tunnels.ipip;
    const users = new Map<string, string>();
    for (const [name, t] of tunnelsOf(config)) {
      if (t.routeBased === undefined) continue;
      const pointer = P('ipsec', 'tunnels', name, 'routeBased', 'ipipInterface');
      const ipipName = t.routeBased.ipipInterface;
      const ipip = ipips[ipipName];
      if (ipip === undefined) {
        issues.push({
          pointer,
          message: `IPIP tunnel '${ipipName}' does not exist in tunnels.ipip`,
        });
        continue;
      }
      const previous = users.get(ipipName);
      if (previous !== undefined) {
        issues.push({
          pointer,
          message: `IPIP tunnel '${ipipName}' is already protected by IPsec tunnel '${previous}'`,
        });
      }
      users.set(ipipName, name);
      if (ipip.src !== t.localAddr) {
        issues.push({
          pointer,
          message: `IPIP tunnel '${ipipName}' source ${ipip.src} differs from localAddr ${t.localAddr}`,
        });
      }
      if (ipip.underlayVrf !== t.underlayVrf) {
        issues.push({
          pointer,
          message: `IPIP tunnel '${ipipName}' underlay VRF '${ipip.underlayVrf}' differs from the IPsec underlay VRF '${t.underlayVrf}'`,
        });
      }
      if (ipip.vrf !== t.vrf) {
        issues.push({
          pointer,
          message: `IPIP tunnel '${ipipName}' VRF '${ipip.vrf}' differs from the IPsec overlay VRF '${t.vrf}'`,
        });
      }
      if (t.remoteAddr === IPSEC_ANY_PEER) {
        if (ipip.mode !== 'p2mp') {
          issues.push({
            pointer,
            message: `a tunnel to ${IPSEC_ANY_PEER} needs a point-to-multipoint IPIP tunnel`,
          });
        }
      } else if (ipip.mode !== 'p2p') {
        issues.push({
          pointer,
          message: `IPIP tunnel '${ipipName}' must be point-to-point for a fixed peer`,
        });
      } else if (ipip.dst !== undefined && isIp(t.remoteAddr) && ipip.dst !== t.remoteAddr) {
        issues.push({
          pointer,
          message: `IPIP tunnel '${ipipName}' destination ${ipip.dst} differs from remoteAddr ${t.remoteAddr}`,
        });
      }
    }
    return issues;
  },
};

function isIp(text: string): boolean {
  return /^[0-9.]+$/.test(text) || text.includes(':');
}

/**
 * A fixed peer is `(underlayVrf, localAddr, remoteAddr)` — one IKE SA per address pair. `%any` responders are a
 * hub design: strongSwan selects among several `%any` connections on one local address by the peer's identity and
 * authentication, so two of them clash only when `localId`, `remoteId` and the auth method are all identical.
 */
const peerUnique: ValidatorDefinition = {
  name: 'vpn.ipsec-peer-unique',
  domains: ['vpn'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const seen = new Map<string, string>();
    for (const [name, t] of tunnelsOf(config)) {
      if (!t.enabled) continue;
      const any = t.remoteAddr === IPSEC_ANY_PEER;
      const key = any
        ? [
            t.underlayVrf,
            t.localAddr,
            t.remoteAddr,
            t.localId ?? '',
            t.remoteId ?? '',
            t.auth.method,
          ].join('|')
        : `${t.underlayVrf}|${t.localAddr}|${t.remoteAddr}`;
      const previous = seen.get(key);
      if (previous !== undefined) {
        issues.push({
          pointer: P('ipsec', 'tunnels', name, 'remoteAddr'),
          message: any
            ? `tunnel '${previous}' already accepts ${IPSEC_ANY_PEER} on ${t.localAddr} in VRF '${t.underlayVrf}' with the same localId/remoteId/auth method — indistinguishable responders`
            : `tunnel '${previous}' already uses ${t.localAddr} → ${t.remoteAddr} in VRF '${t.underlayVrf}'`,
        });
      } else {
        seen.set(key, name);
      }
    }
    return issues;
  },
};

const wireguardUnique: ValidatorDefinition = {
  name: 'vpn.wireguard-unique',
  domains: ['vpn'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const instances = new Map<number, string>();
    const sockets = new Map<string, string>();
    for (const [name, w] of wireguardOf(config)) {
      const base = ['wireguard', 'interfaces', name];
      const byInstance = instances.get(w.instance);
      if (byInstance !== undefined) {
        issues.push({
          pointer: P(...base, 'instance'),
          message: `instance ${w.instance} (wg${w.instance}) is already used by '${byInstance}'`,
        });
      } else {
        instances.set(w.instance, name);
      }
      const socket = `${w.underlayVrf}|${w.listenAddress}|${w.listenPort}`;
      const bySocket = sockets.get(socket);
      if (bySocket !== undefined) {
        issues.push({
          pointer: P(...base, 'listenPort'),
          message: `${w.listenAddress}:${w.listenPort} in VRF '${w.underlayVrf}' is already used by '${bySocket}'`,
        });
      } else {
        sockets.set(socket, name);
      }
      const keys = new Map<string, string>();
      const allowed = new Map<string, string>();
      for (const [peerName, peer] of Object.entries(w.peers)) {
        const byKey = keys.get(peer.publicKey);
        if (byKey !== undefined) {
          issues.push({
            pointer: P(...base, 'peers', peerName, 'publicKey'),
            message: `same public key as peer '${byKey}'`,
          });
        } else {
          keys.set(peer.publicKey, peerName);
        }
        peer.allowedIps.forEach((prefix, i) => {
          const cidr = parseCidr(prefix);
          const normalized =
            cidr === undefined ? prefix : `${cidr.family}|${cidr.first}|${cidr.prefixLength}`;
          const byPrefix = allowed.get(normalized);
          if (byPrefix !== undefined && byPrefix !== peerName) {
            issues.push({
              pointer: P(...base, 'peers', peerName, 'allowedIps', i),
              message: `${prefix} is already routed to peer '${byPrefix}'`,
            });
          } else {
            allowed.set(normalized, peerName);
          }
        });
      }
    }
    return issues;
  },
};

const TUNNEL_OR_WG = /^(?:gre|ipip|vxlan_tunnel|wg)[0-9]+$/;

/**
 * WireGuard interface addresses must not overlap a prefix of a physical / sub-interface or another WireGuard
 * interface in the same VRF (review F10). Overlaps with tunnel interfaces are reported by `tunnels.address-overlap`.
 */
const wireguardAddressOverlap: ValidatorDefinition = {
  name: 'vpn.wireguard-address-overlap',
  domains: ['vpn', 'interfaces'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const seen: { owner: string; vrf: string; cidr: Cidr }[] = [];
    for (const info of interfaceIndex(config).values()) {
      if (TUNNEL_OR_WG.test(info.name)) continue;
      for (const cidr of info.addresses) {
        seen.push({ owner: `interface '${info.name}'`, vrf: info.vrf, cidr });
      }
    }
    for (const [name, w] of wireguardOf(config)) {
      w.address.forEach((text, i) => {
        const cidr = parseCidr(text);
        if (cidr === undefined) return;
        const clash = seen.find((s) => s.vrf === w.vrf && cidrsOverlap(s.cidr, cidr));
        if (clash !== undefined) {
          issues.push({
            pointer: P('wireguard', 'interfaces', name, 'address', i),
            message: `${text} overlaps an address of ${clash.owner} in VRF '${w.vrf}'`,
          });
        }
        seen.push({ owner: `WireGuard interface '${name}'`, vrf: w.vrf, cidr });
      });
    }
    return issues;
  },
};

const AEAD = new Set<string>(IPSEC_AEAD_CIPHERS);
const IKEV2_ONLY_DH = new Set(['curve25519', 'curve448']);

/**
 * The proposal a tunnel uses must fit its `protocol` and `ikeVersion` (review F14): AH authenticates only, so its
 * ESP half must be `null` encryption with an integrity algorithm; IKEv1 has no AEAD IKE SAs (RFC 5282 covers only
 * ESP) and no Curve25519/448 groups (RFC 8031 is IKEv2-only). The native VPP IKEv2 engine is already IKEv2-only.
 */
const proposalCompatible: ValidatorDefinition = {
  name: 'vpn.proposal-compatible',
  domains: ['vpn'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const proposals = config.vpn.ipsec.proposals;
    for (const [name, t] of tunnelsOf(config)) {
      const p = proposals[t.proposal];
      if (p === undefined) continue; // vpn.proposal-exists reports it
      const pointer = P('ipsec', 'tunnels', name, 'proposal');
      if (t.protocol === 'ah' && p.esp.encr !== 'null') {
        issues.push({
          pointer,
          message: `AH does not encrypt: proposal '${t.proposal}' must use esp.encr 'null' with an integrity algorithm`,
        });
      }
      if (t.ikeVersion === 1) {
        if (AEAD.has(p.ike.encr)) {
          issues.push({
            pointer,
            message: `IKEv1 has no AEAD IKE SAs: proposal '${t.proposal}' uses ike.encr '${p.ike.encr}'`,
          });
        }
        for (const [half, dh] of [
          ['ike', p.ike.dh],
          ['esp', p.esp.dh],
        ] as const) {
          if (dh !== undefined && IKEV2_ONLY_DH.has(dh)) {
            issues.push({
              pointer,
              message: `${dh} is defined for IKEv2 only: proposal '${t.proposal}' uses it in ${half}.dh`,
            });
          }
        }
      }
    }
    return issues;
  },
};

const remoteAccessPools: ValidatorDefinition = {
  name: 'vpn.remote-access-pools-no-overlap',
  domains: ['vpn'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const pools: {
      pointer: string;
      label: string;
      cidr: NonNullable<ReturnType<typeof parseCidr>>;
    }[] = [];
    for (const [name, r] of remoteAccessOf(config)) {
      r.pools.forEach((pool, i) => {
        const cidr = parseCidr(pool.prefix);
        if (cidr === undefined) return;
        const pointer = P('remoteAccess', name, 'pools', i, 'prefix');
        const label = `${name}/${pool.name}`;
        for (const other of pools) {
          if (cidrsOverlap(cidr, other.cidr)) {
            issues.push({
              pointer,
              message: `client pool ${pool.prefix} overlaps pool '${other.label}'`,
            });
            break;
          }
        }
        pools.push({ pointer, label, cidr });
      });
    }
    return issues;
  },
};

/** Any PEM armour: `BEGIN <LABEL>` between five-dash fences (private keys, certificates, PGP key blocks …). */
const PEM_BANNER = /-----BEGIN [A-Z0-9 ]+-----/;
/** 16+ bytes as hex (a 128-bit key or longer). */
const HEX_KEY = /^(?:[0-9a-fA-F]{2}){16,}$/;
const BASE64 = /^(?:[A-Za-z0-9+/]{4})+(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/;
/**
 * Fields whose values are legitimately hex/base64 and never secret: WireGuard public keys, the SNMP engine id,
 * DHCP option / DNS record data (hex option payloads, DKIM TXT keys).
 */
const KEY_LIKE_EXEMPT: ReadonlySet<string> = new Set(['publicKey', 'engineId', 'data']);

/**
 * Padded base64 of 16+ bytes. Unpadded base64 is not flagged: interface names such as `TenGigabitEthernet0/0/10`
 * are valid base64 too, and the guard must not reject ordinary values.
 */
function looksLikeBase64Key(value: string): boolean {
  return value.length >= 24 && value.endsWith('=') && BASE64.test(value);
}

/**
 * PEM blocks of any kind anywhere, and hex / base64 blobs of 16+ bytes outside the exempt fields, are key material.
 * This is a guard for free-text fields; the mechanism is the `<kind>/<name>` reference format (D-051), which makes
 * a pasted secret a schema error, plus the API's existence check on the reference.
 */
const noInlineSecretMaterial: ValidatorDefinition = {
  name: 'vpn.no-inline-secret-material',
  domains: ['vpn', 'tunnels', 'services', 'ha'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    for (const domain of ['vpn', 'tunnels', 'services', 'ha'] as const) {
      walkStrings(config[domain], (path, value) => {
        const last = path[path.length - 1];
        if (PEM_BANNER.test(value)) {
          issues.push({
            pointer: jsonPointer(domain, ...path),
            message:
              'PEM material is not allowed in the configuration; store it through POST /api/v1/secrets and reference it',
          });
        } else if (
          typeof last === 'string' &&
          (KEY_LIKE_EXEMPT.has(last) || last.endsWith('Ref'))
        ) {
          // `*Ref` values are already constrained to `<kind>/<name>` by the schema (D-051)
          return;
        } else if (looksLikeBase64Key(value) || HEX_KEY.test(value)) {
          issues.push({
            pointer: jsonPointer(domain, ...path),
            message:
              'this looks like key material (base64 or hex); keys are referenced through a secret reference, never inline',
          });
        }
      });
    }
    return issues;
  },
};

export const vpnValidators: readonly ValidatorDefinition[] = [
  proposalExists,
  pkiReferenceExists,
  vrfExists,
  localAddressConfigured,
  routeBasedIpip,
  peerUnique,
  wireguardUnique,
  wireguardAddressOverlap,
  proposalCompatible,
  remoteAccessPools,
  noInlineSecretMaterial,
];
