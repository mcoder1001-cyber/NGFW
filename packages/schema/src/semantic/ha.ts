import type { RootConfig } from '../index.js';
import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';
import { cidrContainsIp, interfaceIndex, ipFamily, knownVrfs, parseIp } from './tunnels-common.js';

/**
 * Semantic validators for `ha`: VRID unique per (interface, address family); interfaces exist and are in the
 * virtual router's VRF; virtual addresses lie within a prefix of the interface and are not claimed twice in a VRF;
 * VRFs exist; the cluster sync interface exists. Owner: P02c.
 */

const V = (name: string, ...rest: (string | number)[]): string =>
  jsonPointer('ha', 'vrrp', name, ...rest);

/** `ha.vrrp` is a record keyed by name (D-053); iterate as `[name, vr]` in document order. */
const vrrpOf = (config: RootConfig) => Object.entries(config.ha.vrrp);

const vridUnique: ValidatorDefinition = {
  name: 'ha.vrrp-vrid-unique',
  domains: ['ha'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const seen = new Map<string, string>();
    for (const [name, v] of vrrpOf(config)) {
      const key = `${v.interface}|${v.addressFamily}|${v.vrId}`;
      const previous = seen.get(key);
      if (previous !== undefined) {
        issues.push({
          pointer: V(name, 'vrId'),
          message: `VRID ${v.vrId} (${v.addressFamily}) on ${v.interface} is already used by virtual router '${previous}'`,
        });
      } else {
        seen.set(key, name);
      }
    }
    return issues;
  },
};

const interfaceExists: ValidatorDefinition = {
  name: 'ha.interface-exists',
  domains: ['ha', 'interfaces', 'tunnels', 'vpn'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const index = interfaceIndex(config);
    for (const [name, v] of vrrpOf(config)) {
      const info = index.get(v.interface);
      if (info === undefined) {
        issues.push({
          pointer: V(name, 'interface'),
          message: `interface '${v.interface}' does not exist`,
        });
      } else if (info.vrf !== v.vrf) {
        issues.push({
          pointer: V(name, 'vrf'),
          message: `interface '${v.interface}' is in VRF '${info.vrf}', not '${v.vrf}'`,
        });
      }
      v.track.forEach((t, k) => {
        if (!index.has(t.interface)) {
          issues.push({
            pointer: V(name, 'track', k, 'interface'),
            message: `interface '${t.interface}' does not exist`,
          });
        }
      });
    }
    const cluster = config.ha.cluster;
    if (cluster?.interface !== undefined && !index.has(cluster.interface)) {
      issues.push({
        pointer: jsonPointer('ha', 'cluster', 'interface'),
        message: `interface '${cluster.interface}' does not exist`,
      });
    }
    return issues;
  },
};

const virtualAddresses: ValidatorDefinition = {
  name: 'ha.vrrp-virtual-addresses',
  domains: ['ha', 'interfaces', 'tunnels', 'vpn'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const index = interfaceIndex(config);
    const claimed = new Map<string, string>();
    for (const [name, v] of vrrpOf(config)) {
      const info = index.get(v.interface);
      v.addresses.forEach((address, k) => {
        const key = `${v.vrf}|${address}`;
        const previous = claimed.get(key);
        if (previous !== undefined) {
          issues.push({
            pointer: V(name, 'addresses', k),
            message: `${address} is already a virtual address of virtual router '${previous}' in VRF '${v.vrf}'`,
          });
        } else {
          claimed.set(key, name);
        }
        if (info === undefined) return;
        const ip = parseIp(address);
        const family = ipFamily(address);
        const prefixes = info.addresses.filter((c) => c.family === family);
        if (ip === undefined || prefixes.length === 0) return; // unnumbered interface: nothing to check against
        if (!prefixes.some((c) => cidrContainsIp(c, family, ip))) {
          issues.push({
            pointer: V(name, 'addresses', k),
            message: `${address} is not within a prefix configured on ${v.interface}`,
          });
        }
      });
    }
    return issues;
  },
};

const vrfExists: ValidatorDefinition = {
  name: 'ha.vrf-exists',
  domains: ['ha', 'vrfs'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const vrfs = knownVrfs(config);
    for (const [name, v] of vrrpOf(config)) {
      if (!vrfs.has(v.vrf))
        issues.push({ pointer: V(name, 'vrf'), message: `VRF '${v.vrf}' does not exist` });
    }
    const cluster = config.ha.cluster;
    if (cluster !== undefined && !vrfs.has(cluster.vrf)) {
      issues.push({
        pointer: jsonPointer('ha', 'cluster', 'vrf'),
        message: `VRF '${cluster.vrf}' does not exist`,
      });
    }
    return issues;
  },
};

export const haValidators: readonly ValidatorDefinition[] = [
  vridUnique,
  interfaceExists,
  virtualAddresses,
  vrfExists,
];
