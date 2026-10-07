import type { ValidatorDefinition } from './registry.js';
import { interfaceIndex, parseIp } from './tunnels-common.js';

export const haStateSyncValidators: readonly ValidatorDefinition[] = [
  {
    name: 'ha.ha-state-sync-cluster',
    domains: ['ha', 'nat', 'interfaces', 'tunnels', 'vpn'],
    validate(config) {
      const cluster = config.ha.cluster;
      if (!cluster) return [];
      const sync = cluster.stateSync;
      const issues = [];
      for (const kind of ['nat', 'acl', 'ipsec'] as const) {
        if (sync[kind] && !cluster.enabled)
          issues.push({
            pointer: `/ha/cluster/stateSync/${kind}`,
            message: 'State synchronisation requires an enabled HA cluster',
          });
      }
      if (cluster.enabled && sync.nat && config.nat.mode === 'ei') {
        if (!sync.natListener)
          issues.push({
            pointer: '/ha/cluster/stateSync/natListener',
            message: 'NAT44-EI HA requires an explicit listener',
          });
        if (!sync.natFailover)
          issues.push({
            pointer: '/ha/cluster/stateSync/natFailover',
            message: 'NAT44-EI HA requires an explicit failover peer',
          });
        if (!cluster.interface)
          issues.push({
            pointer: '/ha/cluster/interface',
            message: 'Unauthenticated NAT HA requires a dedicated sync interface',
          });
        if (cluster.interface && sync.natListener) {
          const iface = interfaceIndex(config).get(cluster.interface);
          if (
            !iface?.addresses.some(
              (p) => p.family === 4 && p.address === parseIp(sync.natListener!.address),
            )
          )
            issues.push({
              pointer: '/ha/cluster/stateSync/natListener/address',
              message:
                'NAT HA listener must be an IPv4 address owned by the dedicated sync interface',
            });
        }
        if (sync.natListener?.address === sync.natFailover?.address && sync.natListener)
          issues.push({
            pointer: '/ha/cluster/stateSync/natFailover/address',
            message: 'HA peer must differ from local listener',
          });
      }
      return issues;
    },
  },
];
