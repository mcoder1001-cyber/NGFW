import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';
import { interfaceIndex, knownVrfs } from './tunnels-common.js';

/**
 * Semantic validators for `services.hostStack` (F-host-stack): referenced VRFs, interfaces and app namespaces exist,
 * a namespace bound to an interface uses that interface's VRF, and session rules never share a document with
 * Auto-SDL (`services.autoSdl.enabled`, F-rpf-adl-pbr): VPP has ONE session-layer rule engine — session rules need
 * `rule-table`, Auto-SDL needs `sdl`.
 */

const P = (...segments: (string | number)[]): string =>
  jsonPointer('services', 'hostStack', ...segments);

const hostStackRefs: ValidatorDefinition = {
  name: 'services.host-stack-references',
  domains: ['services', 'vrfs', 'interfaces'],
  validate(config) {
    const hs = config.services.hostStack;
    if (hs === undefined) return [];
    const issues: SemanticIssue[] = [];
    const vrfs = knownVrfs(config);
    const ifaces = interfaceIndex(config);
    for (const [id, ns] of Object.entries(hs.namespaces)) {
      if (!vrfs.has(ns.vrf))
        issues.push({
          pointer: P('namespaces', id, 'vrf'),
          message: `VRF '${ns.vrf}' does not exist`,
        });
      if (ns.interface !== undefined) {
        const i = ifaces.get(ns.interface);
        if (i === undefined)
          issues.push({
            pointer: P('namespaces', id, 'interface'),
            message: `interface '${ns.interface}' does not exist`,
          });
        else if (i.vrf !== ns.vrf)
          issues.push({
            pointer: P('namespaces', id, 'vrf'),
            message: `interface '${ns.interface}' is in VRF '${i.vrf}', the namespace names '${ns.vrf}'`,
          });
      }
    }
    hs.sessionRules.forEach((r, i) => {
      if (r.appNamespace !== undefined && !(r.appNamespace in hs.namespaces))
        issues.push({
          pointer: P('sessionRules', i, 'appNamespace'),
          message: `app namespace '${r.appNamespace}' does not exist`,
        });
    });
    if (hs.tcpSourceAddresses !== undefined && !vrfs.has(hs.tcpSourceAddresses.vrf))
      issues.push({
        pointer: P('tcpSourceAddresses', 'vrf'),
        message: `VRF '${hs.tcpSourceAddresses.vrf}' does not exist`,
      });
    return issues;
  },
};

const hostStackRtEngine: ValidatorDefinition = {
  name: 'services.host-stack-rt-engine',
  domains: ['services'],
  validate(config) {
    const hs = config.services.hostStack;
    if (hs === undefined || hs.sessionRules.length === 0) return [];
    // `services.autoSdl` belongs to F-rpf-adl-pbr; read defensively so this rule works before and after it lands.
    const autoSdl = (config.services as Record<string, unknown>)['autoSdl'] as
      { enabled?: unknown } | undefined;
    if (autoSdl?.enabled !== true) return [];
    return [
      {
        pointer: P('sessionRules'),
        message:
          'session rules need the rule-table session engine, services.autoSdl needs sdl: VPP has one engine — use one of them',
      },
    ];
  },
};

export const hostStackValidators: readonly ValidatorDefinition[] = [
  hostStackRefs,
  hostStackRtEngine,
];
