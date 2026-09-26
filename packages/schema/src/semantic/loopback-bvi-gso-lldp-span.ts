import {
  nsimWheelSlots,
  NSIM_WHEEL_SLOTS_MAX,
  RESERVED_LOOPBACK_MAX,
  RESERVED_LOOPBACK_MIN,
} from '../domains/ext/loopback-bvi-gso-lldp-span.js';
import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';
import { interfaceIndex } from './tunnels-common.js';

/**
 * Semantic rules of F-loopback-bvi-gso-lldp-span (`interfaces.<if>.gso`, `interfaces.<if>.mirror`, `services.nsim`,
 * the D-105 loopback reservation). Pointers are built with `jsonPointer()` (VPP interface names contain `/`).
 * "Exists" means: named in the document — `interfaces` (and sub-interfaces), `tunnels` (gre/ipip/vxlan) or
 * `vpn.wireguard` (the `services.interface-references` index), so an ERSPAN destination may be a GRE tunnel of
 * `tunnels.gre`.
 */

const LOOPBACK = /^loop([0-9]+)$/;
/** Sub-interfaces are named `<parent>.<id>`; VPP's nsim needs hardware interfaces (nsim.c: VNET_SW_INTERFACE_TYPE_HARDWARE). */
const isSub = (name: string): boolean => name.includes('.');
/** VPP's own `local0` has no output path (no GSO, no mirroring, no nsim). */
const LOCAL0 = 'local0';

/** Largest VPP packets_per_drop (u32): dropFraction below 1 / 2^32 cannot be expressed. */
const PACKETS_PER_DROP_MAX = 4294967295;

export const loopbackBviGsoLldpSpanValidators: readonly ValidatorDefinition[] = [
  {
    // D-105 (TD-3 M2): loop16000–loop16383 are the agent's V19 quarantine holders.
    name: 'interfaces.loopback-bvi-gso-lldp-span-reserved-loopback',
    domains: ['interfaces'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      for (const name of Object.keys(config.interfaces)) {
        const m = LOOPBACK.exec(name);
        const n = m ? Number(m[1]) : NaN;
        if (n >= RESERVED_LOOPBACK_MIN && n <= RESERVED_LOOPBACK_MAX) {
          issues.push({
            pointer: jsonPointer('interfaces', name),
            message: `loopback instances ${RESERVED_LOOPBACK_MIN}–${RESERVED_LOOPBACK_MAX} are reserved for the agent's quarantine holders (VPP V19); use another loop<N>`,
          });
        }
      }
      return issues;
    },
  },
  {
    name: 'interfaces.loopback-bvi-gso-lldp-span-gso-interface',
    domains: ['interfaces'],
    validate: (config) =>
      Object.entries(config.interfaces)
        .filter(([name, itf]) => itf.gso === true && name === LOCAL0)
        .map(([name]) => ({
          pointer: jsonPointer('interfaces', name, 'gso'),
          message: `GSO needs an interface with an output path; ${LOCAL0} has none`,
        })),
  },
  {
    name: 'interfaces.loopback-bvi-gso-lldp-span-mirror-destination',
    domains: ['interfaces', 'tunnels', 'vpn'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const known = interfaceIndex(config);
      for (const [source, itf] of Object.entries(config.interfaces)) {
        (itf.mirror ?? []).forEach((m, i) => {
          const pointer = jsonPointer('interfaces', source, 'mirror', i, 'destination');
          if (m.destination === source) {
            issues.push({ pointer, message: `a mirror session cannot copy ${source} to itself` });
          } else if (m.destination === LOCAL0 || !known.has(m.destination)) {
            issues.push({
              pointer,
              message: `mirror destination '${m.destination}' does not exist`,
            });
          }
        });
      }
      return issues;
    },
  },
  {
    // A destination that is itself a mirror source would copy the copies (VPP mirrors in a loop).
    name: 'interfaces.loopback-bvi-gso-lldp-span-mirror-loop',
    domains: ['interfaces'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const sources = new Set(
        Object.entries(config.interfaces)
          .filter(([, itf]) => (itf.mirror ?? []).length > 0)
          .map(([name]) => name),
      );
      for (const [source, itf] of Object.entries(config.interfaces)) {
        (itf.mirror ?? []).forEach((m, i) => {
          if (m.destination !== source && sources.has(m.destination)) {
            issues.push({
              pointer: jsonPointer('interfaces', source, 'mirror', i, 'destination'),
              message: `mirror destination '${m.destination}' is itself mirrored (a mirror destination must not be a mirror source)`,
            });
          }
        });
      }
      return issues;
    },
  },
  {
    name: 'interfaces.loopback-bvi-gso-lldp-span-mirror-duplicate',
    domains: ['interfaces'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      for (const [source, itf] of Object.entries(config.interfaces)) {
        const seen = new Set<string>();
        (itf.mirror ?? []).forEach((m, i) => {
          const k = `${m.destination}\u0000${m.level}`;
          if (seen.has(k)) {
            issues.push({
              pointer: jsonPointer('interfaces', source, 'mirror', i),
              message: `${source} already mirrors to '${m.destination}' at the ${m.level} level (one session per destination and level; set direction to both)`,
            });
          }
          seen.add(k);
        });
      }
      return issues;
    },
  },
  {
    name: 'services.loopback-bvi-gso-lldp-span-nsim-range',
    domains: ['services'],
    validate: (config) => {
      const n = config.services.nsim;
      if (n === undefined) return [];
      const issues: SemanticIssue[] = [];
      const slots = nsimWheelSlots(n.delayMs, n.bandwidthMbps, n.packetSize);
      if (slots > NSIM_WHEEL_SLOTS_MAX) {
        issues.push({
          pointer: jsonPointer('services', 'nsim', 'delayMs'),
          message: `delay × bandwidth needs a ${slots}-slot scheduler wheel (more than ${NSIM_WHEEL_SLOTS_MAX}, 32 MiB per thread); lower the delay or the bandwidth, or raise packetSize`,
        });
      }
      if (n.dropFraction > 0 && Math.round(1 / n.dropFraction) > PACKETS_PER_DROP_MAX) {
        issues.push({
          pointer: jsonPointer('services', 'nsim', 'dropFraction'),
          message: `drop fraction ${n.dropFraction} is below 1 / ${PACKETS_PER_DROP_MAX} (VPP packets_per_drop is 32-bit); use 0 for no loss`,
        });
      }
      return issues;
    },
  },
  {
    name: 'services.loopback-bvi-gso-lldp-span-nsim-interfaces',
    domains: ['services', 'interfaces', 'tunnels', 'vpn'],
    validate: (config) => {
      const n = config.services.nsim;
      if (n === undefined) return [];
      const issues: SemanticIssue[] = [];
      const known = interfaceIndex(config);
      const check = (pointer: string, name: string): void => {
        if (name === LOCAL0 || !known.has(name)) {
          issues.push({ pointer, message: `interface '${name}' does not exist` });
        } else if (isSub(name)) {
          issues.push({
            pointer,
            message: `nsim needs a hardware interface; '${name}' is a sub-interface`,
          });
        }
      };
      const xc = n.crossConnect;
      if (xc !== undefined) {
        check(jsonPointer('services', 'nsim', 'crossConnect', 'a'), xc.a);
        check(jsonPointer('services', 'nsim', 'crossConnect', 'b'), xc.b);
        if (xc.a === xc.b) {
          issues.push({
            pointer: jsonPointer('services', 'nsim', 'crossConnect', 'b'),
            message: 'the nsim cross-connect needs two different interfaces',
          });
        }
      }
      const seen = new Set<string>();
      n.outputInterfaces.forEach((name, i) => {
        const pointer = jsonPointer('services', 'nsim', 'outputInterfaces', i);
        if (seen.has(name)) {
          issues.push({ pointer, message: `'${name}' is listed twice` });
          return;
        }
        seen.add(name);
        check(pointer, name);
      });
      return issues;
    },
  },
];
