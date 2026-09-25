import type { RootConfig } from '../index.js';
import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';
import {
  type Cidr,
  cidrContainsIp,
  cidrsOverlap,
  ipFamily,
  parseCidr,
  parseIp,
  sameCidr,
} from './tunnels-common.js';

/**
 * F-wireguard rules for `vpn.wireguard` (tier (b)), in addition to P02c's `vpn.wireguard-unique`,
 * `vpn.wireguard-address-overlap`, `vpn.local-address-configured` and `vpn.vrf-exists` (semantic/vpn.ts, not repeated here):
 *
 * - `vpn.wireguard-public-key-unique`: VPP keys peers by public key **VPP-wide** (DF-5: `wireguard_peer_add_v2` refuses a
 *   key that another interface already has), so a key may appear on one interface only. The second occurrence is
 *   reported (duplicates inside one interface stay `vpn.wireguard-unique`'s).
 * - `vpn.wireguard-allowed-ips`: an allowed IP is a network prefix (no host bits — the agent programs it as written and
 *   Retrieve returns it masked) and prefixes of different peers of one interface do not overlap (cryptokey routing
 *   would be ambiguous; the identical prefix stays `vpn.wireguard-unique`'s).
 * - `vpn.wireguard-endpoint-family`: a peer endpoint given as an IP address has the family of the interface's
 *   listen address (VPP sends from `listenAddress`).
 * - `vpn.wireguard-route-loop` (F-wireguard review F1): with `routeAllowedIps`, every allowed IP becomes a route via
 *   `wg<N>` in the interface's `vrf`. VPP stacks each peer's tunnel packets on the route to its endpoint in the peer's
 *   **underlay** VRF, so an allowed IP that contains a peer endpoint of any WireGuard interface whose `underlayVrf` is this
 *   `vrf` (the full-tunnel `0.0.0.0/0` peer with `vrf` = `underlayVrf`) would route the tunnel's own UDP into the tunnel:
 *   a forwarding loop. It is refused at the allowed IP; use a narrower allowed IP, another overlay VRF, or leave
 *   `routeAllowedIps` off and add the routes by hand (with a more specific route to the endpoint).
 */

const P = (...segments: (string | number)[]): string => jsonPointer('vpn', ...segments);

function wireguardOf(config: RootConfig) {
  return Object.entries(config.vpn.wireguard.interfaces);
}

const publicKeyUnique: ValidatorDefinition = {
  name: 'vpn.wireguard-public-key-unique',
  domains: ['vpn'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const seen = new Map<string, { iface: string; peer: string }>();
    for (const [ifName, w] of wireguardOf(config)) {
      for (const [peerName, peer] of Object.entries(w.peers)) {
        const first = seen.get(peer.publicKey);
        if (first === undefined) {
          seen.set(peer.publicKey, { iface: ifName, peer: peerName });
          continue;
        }
        if (first.iface === ifName) continue; // vpn.wireguard-unique reports it
        issues.push({
          pointer: P('wireguard', 'interfaces', ifName, 'peers', peerName, 'publicKey'),
          message: `public key is already used by peer '${first.peer}' of WireGuard interface '${first.iface}' (VPP keys peers by public key across all interfaces)`,
        });
      }
    }
    return issues;
  },
};

const allowedIps: ValidatorDefinition = {
  name: 'vpn.wireguard-allowed-ips',
  domains: ['vpn'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    for (const [ifName, w] of wireguardOf(config)) {
      const taken: { peer: string; text: string; cidr: Cidr }[] = [];
      for (const [peerName, peer] of Object.entries(w.peers)) {
        const mine: { text: string; cidr: Cidr }[] = [];
        peer.allowedIps.forEach((text, i) => {
          const cidr = parseCidr(text);
          if (cidr === undefined) return; // the schema reports it
          const pointer = P('wireguard', 'interfaces', ifName, 'peers', peerName, 'allowedIps', i);
          if (cidr.address !== cidr.first) {
            issues.push({
              pointer,
              message: `${text} has host bits set (a network prefix is expected)`,
            });
            return;
          }
          const clash = taken.find((t) => t.peer !== peerName && cidrsOverlap(t.cidr, cidr));
          if (clash !== undefined && !sameCidr(clash.cidr, cidr)) {
            issues.push({
              pointer,
              message: `${text} overlaps ${clash.text} of peer '${clash.peer}' (allowed IPs of one interface must not overlap)`,
            });
          }
          mine.push({ text, cidr });
        });
        for (const m of mine) taken.push({ peer: peerName, ...m });
      }
    }
    return issues;
  },
};

const endpointFamily: ValidatorDefinition = {
  name: 'vpn.wireguard-endpoint-family',
  domains: ['vpn'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    for (const [ifName, w] of wireguardOf(config)) {
      const listen = ipFamily(w.listenAddress);
      for (const [peerName, peer] of Object.entries(w.peers)) {
        const address = peer.endpoint?.address;
        if (address === undefined || parseIp(address) === undefined) continue; // none, or a hostname
        const family = ipFamily(address);
        if (family !== listen) {
          issues.push({
            pointer: P('wireguard', 'interfaces', ifName, 'peers', peerName, 'endpoint', 'address'),
            message: `endpoint ${address} is IPv${family} but the interface listens on IPv${listen} address ${w.listenAddress}`,
          });
        }
      }
    }
    return issues;
  },
};

const routeLoop: ValidatorDefinition = {
  name: 'vpn.wireguard-route-loop',
  domains: ['vpn'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    // peer endpoints (IP literals) by the underlay VRF their tunnel packets are routed in
    const endpoints: { vrf: string; address: string; iface: string; peer: string }[] = [];
    for (const [ifName, w] of wireguardOf(config)) {
      for (const [peerName, peer] of Object.entries(w.peers)) {
        const address = peer.endpoint?.address;
        if (address !== undefined && parseIp(address) !== undefined) {
          endpoints.push({ vrf: w.underlayVrf, address, iface: ifName, peer: peerName });
        }
      }
    }
    for (const [ifName, w] of wireguardOf(config)) {
      if (!w.routeAllowedIps) continue;
      for (const [peerName, peer] of Object.entries(w.peers)) {
        peer.allowedIps.forEach((text, i) => {
          const cidr = parseCidr(text);
          if (cidr === undefined) return;
          const hit = endpoints.find(
            (e) =>
              e.vrf === w.vrf && cidrContainsIp(cidr, ipFamily(e.address), parseIp(e.address)!),
          );
          if (hit === undefined) return;
          issues.push({
            pointer: P('wireguard', 'interfaces', ifName, 'peers', peerName, 'allowedIps', i),
            message: `routeAllowedIps would route ${text} via wg${w.instance} in VRF '${w.vrf}', and it contains the endpoint ${hit.address} of peer '${hit.peer}' (interface '${hit.iface}', underlay VRF '${hit.vrf}'): the tunnel's own packets would loop into the tunnel — use a narrower allowed IP, another VRF, or routeAllowedIps off with a more specific route to the endpoint`,
          });
        });
      }
    }
    return issues;
  },
};

/** F-wireguard's validators (one spread line in semantic/index.ts). */
export const wireguardValidators: readonly ValidatorDefinition[] = [
  publicKeyUnique,
  allowedIps,
  endpointFamily,
  routeLoop,
];
