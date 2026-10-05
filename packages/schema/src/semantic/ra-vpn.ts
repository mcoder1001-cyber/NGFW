import { jsonPointer } from '../pointer.js';
import { cidrsOverlap, interfaceIndex, parseCidr } from './tunnels-common.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/** The independent engine is authorized; runtime activation remains failclosed until implemented. */
export const raVpnValidators: readonly ValidatorDefinition[] = [
  {
    name: 'vpn.remote-access-native-capability',
    domains: ['vpn'],
    validate(config) {
      return Object.entries(config.vpn.remoteAccess)
        .filter(([, profile]) => profile.enabled)
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([name]) => ({
          pointer: jsonPointer('vpn', 'remoteAccess', name, 'enabled'),
          message:
            'Remote-access VPN is unavailable until the independent engine is operational; disable this profile before committing. Disabled profiles are inactive drafts.',
        }));
    },
  },
];

/** Independent engine contracts are validated while runtime remains failclosed. */
export const raVpnTransportValidator: ValidatorDefinition = {
  name: 'vpn.remote-access-private-transit',
  domains: ['vpn', 'interfaces', 'tunnels'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const occupied = [...interfaceIndex(config).values()].flatMap((iface) =>
      iface.addresses.map((cidr) => ({ cidr, label: `interface '${iface.name}'` })),
    );
    const endpoints = new Set<string>();
    for (const [name, profile] of Object.entries(config.vpn.remoteAccess).sort(([a], [b]) =>
      a.localeCompare(b),
    )) {
      const at = (...parts: (string | number)[]) =>
        jsonPointer('vpn', 'remoteAccess', name, ...parts);
      const error = (pointer: string, message: string) => issues.push({ pointer, message });
      const transport = profile.transport;
      if (!transport) {
        if (profile.enabled)
          error(
            at('transport'),
            'Independent remote access requires explicit outer and inner transit addresses.',
          );
        continue;
      }
      const local = parseCidr(
        `${profile.localAddr}/${profile.localAddr.includes(':') ? 128 : 32}`,
      )!;
      const endpointKey = `${profile.underlayVrf}/${local.family}/${local.address}`;
      if (endpoints.has(endpointKey))
        error(
          at('localAddr'),
          'Remote-access routed endpoint is already owned by another profile.',
        );
      endpoints.add(endpointKey);
      if (
        occupied.some(({ cidr }) => cidr.family === local.family && cidr.address === local.address)
      ) {
        error(
          at('localAddr'),
          'Remote-access endpoint must be dedicated; it is already assigned to an interface.',
        );
      }
      if (
        Object.values(config.vpn.ipsec.tunnels).some(
          (tunnel) =>
            tunnel.enabled &&
            tunnel.underlayVrf === profile.underlayVrf &&
            tunnel.localAddr === profile.localAddr,
        )
      ) {
        error(
          at('localAddr'),
          'Remote-access endpoint collides with a native site-to-site IKE listener.',
        );
      }
      for (const [kind, link] of Object.entries(transport)) {
        if (!link) continue;
        const vpp = parseCidr(link.vpp)!;
        const namespace = parseCidr(link.namespace)!;
        const family = kind === 'outer' ? local.family : kind === 'inner' ? 4 : 6;
        if (
          vpp.family !== family ||
          namespace.family !== family ||
          vpp.prefixLength !== (family === 4 ? 31 : 127) ||
          namespace.prefixLength !== vpp.prefixLength ||
          vpp.first !== namespace.first ||
          vpp.address === namespace.address
        ) {
          error(
            at('transport', kind),
            'Transit sides must be distinct addresses in the same /31 IPv4 or /127 IPv6 point-to-point subnet of the required family.',
          );
          continue;
        }
        for (const other of occupied) {
          if (cidrsOverlap(vpp, other.cidr))
            error(at('transport', kind), `Transit subnet overlaps ${other.label}.`);
        }
        for (const [otherName, otherProfile] of Object.entries(config.vpn.remoteAccess)) {
          for (const pool of otherProfile.pools) {
            if (cidrsOverlap(vpp, parseCidr(pool.prefix)!))
              error(
                at('transport', kind),
                `Transit subnet overlaps client pool '${otherName}/${pool.name}'.`,
              );
          }
        }
        if (cidrsOverlap(vpp, local))
          error(
            at('transport', kind),
            'Transit subnet overlaps the public remote-access endpoint.',
          );
        occupied.push({ cidr: vpp, label: `remote-access transit '${name}/${kind}'` });
      }
      profile.pools.forEach((pool, index) => {
        const cidr = parseCidr(pool.prefix)!;
        if (cidr.family === 6 && !transport.innerIpv6)
          error(
            at('transport', 'innerIpv6'),
            'IPv6 client pools require explicit IPv6 protected transit addresses.',
          );
        for (const iface of interfaceIndex(config).values()) {
          if (iface.addresses.some((subnet) => cidrsOverlap(cidr, subnet)))
            error(at('pools', index, 'prefix'), `Client pool overlaps interface '${iface.name}'.`);
        }
        if (cidrsOverlap(cidr, local))
          error(
            at('pools', index, 'prefix'),
            'Client pool overlaps the public remote-access endpoint.',
          );
      });
    }
    return issues;
  },
};
