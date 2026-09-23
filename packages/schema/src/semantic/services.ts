import { QOS_SOURCE_MAX } from '../domains/services.js';
import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';
import {
  addressConfiguredInVrf,
  type Cidr,
  cidrContains,
  cidrsOverlap,
  interfaceIndex,
  isUnspecified,
  knownVrfs,
  parseCidr,
  prefixesOfInterfaces,
} from './tunnels-common.js';

/**
 * Semantic validators for `services`: VRFs exist; every interface an instance binds to exists and belongs to the
 * instance VRF; DHCP subnets lie within a prefix of one of the server's interfaces and do not overlap across
 * servers of a VRF; relay / exporter / agent / bind addresses are configured in the VRF (the relay source in the
 * server VRF, F13); DNS listen sockets are unique per VRF; QoS attachments reference existing interfaces,
 * policers, shapers and maps, map ids are unique and marked output values fit the output header. Intra-object
 * rules (pools inside the subnet, families, duplicates) are schema refinements in `domains/services.ts`.
 * Owner: P02c.
 */

const P = (...segments: (string | number)[]): string => jsonPointer('services', ...segments);

const vrfExists: ValidatorDefinition = {
  name: 'services.vrf-exists',
  domains: ['services', 'vrfs'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const vrfs = knownVrfs(config);
    const check = (pointer: string, vrf: string | undefined): void => {
      if (vrf !== undefined && !vrfs.has(vrf))
        issues.push({ pointer, message: `VRF '${vrf}' does not exist` });
    };
    const s = config.services;
    for (const [name, d] of Object.entries(s.dhcp.servers))
      check(P('dhcp', 'servers', name, 'vrf'), d.vrf);
    for (const [name, r] of Object.entries(s.dhcp.relays)) {
      check(P('dhcp', 'relays', name, 'vrf'), r.vrf);
      check(P('dhcp', 'relays', name, 'serverVrf'), r.serverVrf);
    }
    for (const [name, r] of Object.entries(s.dns.resolvers))
      check(P('dns', 'resolvers', name, 'vrf'), r.vrf);
    check(P('snmp', 'vrf'), s.snmp.vrf);
    for (const [name, e] of Object.entries(s.ipfix.exporters))
      check(P('ipfix', 'exporters', name, 'vrf'), e.vrf);
    check(P('ipfix', 'sflow', 'vrf'), s.ipfix.sflow?.vrf);
    check(P('ntp', 'vrf'), s.ntp.vrf);
    return issues;
  },
};

const interfaceReferences: ValidatorDefinition = {
  name: 'services.interface-references',
  domains: ['services', 'interfaces', 'tunnels', 'vpn'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const index = interfaceIndex(config);
    const check = (pointer: string, name: string, vrf?: string): void => {
      const info = index.get(name);
      if (info === undefined) {
        issues.push({ pointer, message: `interface '${name}' does not exist` });
      } else if (vrf !== undefined && info.vrf !== vrf) {
        issues.push({
          pointer,
          message: `interface '${name}' is in VRF '${info.vrf}', not in the instance VRF '${vrf}'`,
        });
      }
    };
    const s = config.services;
    for (const [name, d] of Object.entries(s.dhcp.servers)) {
      d.interfaces.forEach((i, k) => check(P('dhcp', 'servers', name, 'interfaces', k), i, d.vrf));
    }
    for (const [name, r] of Object.entries(s.dhcp.relays)) {
      r.interfaces.forEach((i, k) => check(P('dhcp', 'relays', name, 'interfaces', k), i, r.vrf));
    }
    s.lldp.interfaces.forEach((l, k) =>
      check(P('lldp', 'interfaces', k, 'interface'), l.interface),
    );
    s.ipfix.flowprobe.interfaces.forEach((f, k) =>
      check(P('ipfix', 'flowprobe', 'interfaces', k, 'interface'), f.interface),
    );
    s.ipfix.sflow?.interfaces.forEach((i, k) => check(P('ipfix', 'sflow', 'interfaces', k), i));
    return issues;
  },
};

const dhcpSubnetWithinInterfacePrefix: ValidatorDefinition = {
  name: 'services.dhcp-subnet-within-interface-prefix',
  domains: ['services', 'interfaces', 'tunnels', 'vpn'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const index = interfaceIndex(config);
    for (const [name, d] of Object.entries(config.services.dhcp.servers)) {
      const known = d.interfaces.filter((i) => index.has(i));
      if (known.length === 0) continue; // interface-references already reported the missing interfaces
      const prefixes = prefixesOfInterfaces(index, known);
      for (const [subnetName, sub] of Object.entries(d.subnets)) {
        const cidr = parseCidr(sub.subnet);
        if (cidr === undefined || prefixes.some((p) => cidrContains(p, cidr))) continue;
        issues.push({
          pointer: P('dhcp', 'servers', name, 'subnets', subnetName, 'subnet'),
          message: `${sub.subnet} is not within a prefix configured on ${known.join(', ')}`,
        });
      }
    }
    return issues;
  },
};

const dhcpSubnetsUnique: ValidatorDefinition = {
  name: 'services.dhcp-subnets-unique',
  domains: ['services'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const seen: { server: string; vrf: string; cidr: Cidr }[] = [];
    for (const [name, d] of Object.entries(config.services.dhcp.servers)) {
      for (const [subnetName, sub] of Object.entries(d.subnets)) {
        const cidr = parseCidr(sub.subnet);
        if (cidr === undefined) continue;
        const clash = seen.find(
          (s) => s.vrf === d.vrf && s.server !== name && cidrsOverlap(s.cidr, cidr),
        );
        if (clash !== undefined) {
          issues.push({
            pointer: P('dhcp', 'servers', name, 'subnets', subnetName, 'subnet'),
            message: `${sub.subnet} overlaps a subnet of DHCP server '${clash.server}' in VRF '${d.vrf}'`,
          });
        }
        seen.push({ server: name, vrf: d.vrf, cidr });
      }
    }
    return issues;
  },
};

const bindAddressConfigured: ValidatorDefinition = {
  name: 'services.bind-address-configured',
  domains: ['services', 'interfaces', 'tunnels', 'vpn'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const index = interfaceIndex(config);
    const check = (pointer: string, address: string, vrf: string): void => {
      if (isUnspecified(address)) return;
      if (!addressConfiguredInVrf(index, address, vrf)) {
        issues.push({
          pointer,
          message: `${address} is not configured on any interface in VRF '${vrf}'`,
        });
      }
    };
    const s = config.services;
    for (const [name, r] of Object.entries(s.dhcp.relays)) {
      // VPP dhcp_proxy_config: dhcp_src_address is the source towards dhcp_server in server_vrf_id (F13)
      check(P('dhcp', 'relays', name, 'sourceAddress'), r.sourceAddress, r.serverVrf ?? r.vrf);
    }
    for (const [name, r] of Object.entries(s.dns.resolvers)) {
      r.listen.forEach((l, k) =>
        check(P('dns', 'resolvers', name, 'listen', k, 'address'), l.address, r.vrf),
      );
    }
    const snmp = s.snmp;
    snmp.listen.forEach((l, k) => check(P('snmp', 'listen', k, 'address'), l.address, snmp.vrf));
    for (const [name, e] of Object.entries(s.ipfix.exporters)) {
      check(P('ipfix', 'exporters', name, 'sourceAddress'), e.sourceAddress, e.vrf);
    }
    if (s.ipfix.sflow?.agentAddress !== undefined) {
      check(P('ipfix', 'sflow', 'agentAddress'), s.ipfix.sflow.agentAddress, s.ipfix.sflow.vrf);
    }
    const ntp = s.ntp;
    ntp.listen.forEach((a, k) => check(P('ntp', 'listen', k), a, ntp.vrf));
    return issues;
  },
};

const dnsListenUnique: ValidatorDefinition = {
  name: 'services.dns-listen-unique',
  domains: ['services'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const seen = new Map<string, string>();
    for (const [name, r] of Object.entries(config.services.dns.resolvers)) {
      r.listen.forEach((l, k) => {
        const wildcard = isUnspecified(l.address);
        const keys = wildcard
          ? [`${r.vrf}|*|${l.address.includes(':') ? 6 : 4}|${l.port}`]
          : [
              `${r.vrf}|${l.address}|${l.port}`,
              `${r.vrf}|*|${l.address.includes(':') ? 6 : 4}|${l.port}`,
            ];
        const owner = keys.map((key) => seen.get(key)).find((o) => o !== undefined && o !== name);
        if (owner !== undefined) {
          issues.push({
            pointer: P('dns', 'resolvers', name, 'listen', k),
            message: `${l.address}:${l.port} in VRF '${r.vrf}' is already served by resolver '${owner}'`,
          });
        }
        for (const key of keys) if (!seen.has(key)) seen.set(key, name);
      });
    }
    return issues;
  },
};

const qosReferences: ValidatorDefinition = {
  name: 'services.qos-references',
  domains: ['services', 'interfaces', 'tunnels', 'vpn'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const index = interfaceIndex(config);
    const qos = config.services.qos;
    const need = (
      pointer: string,
      name: string | undefined,
      record: Record<string, unknown>,
      what: string,
    ): void => {
      if (name !== undefined && !(name in record)) {
        issues.push({ pointer, message: `${what} '${name}' does not exist in qos.${what}s` });
      }
    };
    for (const [ifName, a] of Object.entries(qos.interfaces)) {
      const Q = (...rest: (string | number)[]): string => P('qos', 'interfaces', ifName, ...rest);
      if (!index.has(ifName)) {
        issues.push({ pointer: Q(), message: `interface '${ifName}' does not exist` });
      }
      need(Q('policer', 'input'), a.policer?.input, qos.policers, 'policer');
      need(Q('policer', 'output'), a.policer?.output, qos.policers, 'policer');
      need(Q('shaper'), a.shaper, qos.shapers, 'shaper');
      need(Q('mark', 'map'), a.mark?.map, qos.maps, 'map');
    }
    return issues;
  },
};

const qosConsistency: ValidatorDefinition = {
  name: 'services.qos-consistency',
  domains: ['services'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const qos = config.services.qos;
    const ids = new Map<number, string>();
    for (const [name, m] of Object.entries(qos.maps)) {
      if (m.id === undefined) continue;
      const previous = ids.get(m.id);
      if (previous !== undefined) {
        issues.push({
          pointer: P('qos', 'maps', name, 'id'),
          message: `map id ${m.id} is already used by map '${previous}'`,
        });
      } else {
        ids.set(m.id, name);
      }
    }
    for (const [ifName, a] of Object.entries(qos.interfaces)) {
      const mark = a.mark;
      if (mark === undefined) continue;
      const map = qos.maps[mark.map];
      if (map === undefined) continue; // qos-references reported it
      const max = QOS_SOURCE_MAX[mark.output];
      for (const [source, row] of Object.entries(map.rows)) {
        row?.forEach((e, i) => {
          if (e.to > max) {
            issues.push({
              pointer: P('qos', 'maps', mark.map, 'rows', source, i, 'to'),
              message: `${e.to} does not fit the ${mark.output} header (0–${max}) marked on interface '${ifName}'`,
            });
          }
        });
      }
    }
    return issues;
  },
};

export const servicesValidators: readonly ValidatorDefinition[] = [
  vrfExists,
  interfaceReferences,
  dhcpSubnetWithinInterfacePrefix,
  dhcpSubnetsUnique,
  bindAddressConfigured,
  dnsListenUnique,
  qosReferences,
  qosConsistency,
];
