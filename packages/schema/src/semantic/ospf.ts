import { vrfExists } from '../domains/vrfs.js';
import { ospfAreaNumber } from '../domains/routing.js';
import { interfaceIndex } from './tunnels-common.js';
import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

export const ospfValidators: readonly ValidatorDefinition[] = [
  {
    name: 'routing.ospf-family-and-process',
    domains: ['routing', 'interfaces', 'vrfs', 'tunnels'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const index = interfaceIndex(config, true);
      for (const proto of ['ospf', 'ospf6'] as const) {
        const o = config.routing[proto];
        if (!o) continue;
        const add = (path: string[], message: string) =>
          issues.push({ pointer: jsonPointer('routing', proto, ...path), message });
        if (
          !o.routerId &&
          ![...index.values()].some(
            (i) => i.vrf === o.vrf && i.addresses.some((a) => a.family === 4),
          )
        )
          add(['routerId'], 'router id is required when the VRF has no IPv4 address');
        for (const [id, area] of Object.entries(o.areas)) {
          if (ospfAreaNumber(id) === 0 && area.type !== 'normal')
            add(['areas', id, 'type'], 'backbone area cannot be stub or NSSA');
          if (area.type === 'normal' && area.noSummary)
            add(['areas', id, 'noSummary'], 'noSummary requires stub or NSSA');
        }
        const areas = new Set(Object.keys(o.areas).map(ospfAreaNumber));
        for (const [name, iface] of Object.entries(o.interfaces)) {
          const i = index.get(name);
          if (!i) {
            if (proto === 'ospf6') add(['interfaces', name], 'interface does not exist');
            continue;
          }
          if (i.vrf !== o.vrf)
            add(['interfaces', name], 'interface must belong to the process VRF');
          if (!i.addresses.some((a) => a.family === (proto === 'ospf' ? 4 : 6)))
            add(
              ['interfaces', name],
              `interface requires an IPv${proto === 'ospf' ? 4 : 6} address`,
            );
          if (proto === 'ospf6' && !areas.has(ospfAreaNumber(iface.area)))
            add(['interfaces', name, 'area'], 'OSPFv3 area is not defined');
          if (
            iface.deadIntervalSec !== undefined &&
            iface.deadIntervalSec <= (iface.helloIntervalSec ?? 10)
          )
            add(
              ['interfaces', name, 'deadIntervalSec'],
              'dead interval must exceed hello interval',
            );
        }
        if (proto === 'ospf6') {
          if (!vrfExists(config.vrfs, o.vrf)) add(['vrf'], 'VRF does not exist');
          for (const [src, opt] of Object.entries(o.redistribute))
            if (opt?.routeMap && !config.routing.policy.routeMaps[opt.routeMap])
              add(['redistribute', src, 'routeMap'], 'route map does not exist');
        }
      }
      return issues;
    },
  },
];
