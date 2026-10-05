import type { RootConfig } from '../index.js';
import { jsonPointer } from '../pointer.js';
import { ipFamily, parseIp, interfaceIndex } from './tunnels-common.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/** Warnings are advisory and must never reject an otherwise valid commit. */
export function redistributionWarnings(config: RootConfig): SemanticIssue[] {
  const issues: SemanticIssue[] = [];
  for (const a of ['bgp', 'ospf', 'isis', 'rip'] as const) {
    for (const b of ['bgp', 'ospf', 'isis', 'rip'] as const) {
      if (a >= b) continue;
      const ab = Object.entries(config.routing[b]?.redistribute ?? {}).find(([s]) => s === a)?.[1];
      const ba = Object.entries(config.routing[a]?.redistribute ?? {}).find(([s]) => s === b)?.[1];
      if (ab && ba && !ab.routeMap && !ba.routeMap)
        issues.push({
          pointer: jsonPointer('routing', b, 'redistribute', a),
          message: `Redistribution loop ${a} ↔ ${b}: use route maps to filter feedback`,
        });
    }
  }
  return issues;
}

export const bfdRedistributionValidators: readonly ValidatorDefinition[] = [
  {
    name: 'routing.bfd-redistribution-port-and-address',
    domains: ['routing', 'interfaces', 'tunnels', 'vpn'],
    validate(config) {
      const issues: SemanticIssue[] = [];
      const index = interfaceIndex(config, true);
      const sessions = config.routing.bfd?.sessions ?? [];
      const occupied = new Set(sessions.map((s) => `${ipFamily(s.localAddress)}/${s.multihop}`));
      const profiles = config.routing.bfd?.profiles ?? {};
      sessions.forEach((s, n) => {
        const i = index.get(s.interface);
        if (
          i &&
          !i.addresses.some(
            (a) => a.family === ipFamily(s.localAddress) && a.address === parseIp(s.localAddress),
          )
        )
          issues.push({
            pointer: jsonPointer('routing', 'bfd', 'sessions', String(n), 'localAddress'),
            message: 'BFD local address must be assigned to its interface',
          });
      });
      for (const p of ['ospf', 'isis'] as const) {
        for (const [name, i] of Object.entries(config.routing[p]?.interfaces ?? {})) {
          if (i.bfdProfile && !profiles[i.bfdProfile])
            issues.push({
              pointer: jsonPointer('routing', p, 'interfaces', name, 'bfdProfile'),
              message: 'BFD profile does not exist',
            });
          if (i.bfdProfile && !i.bfd)
            issues.push({
              pointer: jsonPointer('routing', p, 'interfaces', name, 'bfdProfile'),
              message: 'BFD profile requires BFD enabled',
            });
          const families =
            p === 'ospf' ? [4] : (index.get(name)?.addresses.map((a) => a.family) ?? [4, 6]);
          if (i.bfd && families.some((f) => occupied.has(`${f}/false`)))
            issues.push({
              pointer: jsonPointer('routing', p, 'interfaces', name, 'bfd'),
              message:
                'VPP holds the BFD UDP port for this address family; FRR BFD cannot coexist on this box',
            });
        }
      }
      const reported = new Set<string>();
      for (const [peer, n] of Object.entries(config.routing.bgp?.neighbors ?? {})) {
        const group = n.peerGroup ? config.routing.bgp?.peerGroups[n.peerGroup] : undefined;
        const multihop = (n.ebgpMultihop ?? group?.ebgpMultihop ?? 1) > 1;
        if ((n.bfd || group?.bfd) && occupied.has(`${ipFamily(peer)}/${multihop}`)) {
          const pointer = n.bfd
            ? jsonPointer('routing', 'bgp', 'neighbors', peer, 'bfd')
            : jsonPointer('routing', 'bgp', 'peerGroups', n.peerGroup!, 'bfd');
          if (!reported.has(pointer))
            issues.push({
              pointer,
              message:
                'VPP holds the BFD UDP port for this address family and hop type; FRR BFD cannot coexist on this box',
            });
          reported.add(pointer);
        }
      }
      return issues;
    },
  },
];
