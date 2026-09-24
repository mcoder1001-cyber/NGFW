import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';
import { compareIp, ipFamily } from './tunnels-common.js';

/**
 * Cross-field rules of F-kea-dhcp-relay (Kea DHCP servers, VPP DHCP relay, VPP DHCPv4 client) that the P02c rules in
 * `services.ts` / `interfaces.ts` do not cover. The agent re-checks each of them before anything is applied
 * (defence in depth, D-049):
 *
 * - `services.kea-dhcp-relay-reservation-outside-pools`: a reserved address lies outside every pool of its subnet
 *   (Kea hands out pool addresses dynamically; an in-pool reservation can be leased to another client first).
 * - `services.kea-dhcp-relay-one-vrf-per-family`: the enabled servers of one family share one VRF — one kea-dhcp4
 *   (kea-dhcp6) process serves every server of its family and lives in one network namespace (RF-3 renderer rule).
 * - `services.kea-dhcp-relay-relay-source-per-vrf`: relays of one family on one client VRF use one source address
 *   and never list the same server twice (VPP keeps one `dhcp_src_address` per rx VRF and family; a server is keyed
 *   by rx VRF + server VRF + address).
 * - `interfaces.kea-dhcp-relay-dhcp-client-no-static`: an interface (or sub-interface) that runs the DHCPv4 client
 *   has no static IPv4 address (the lease would be a second address of the same interface).
 *
 * Owner: F-kea-dhcp-relay.
 */

const P = (...segments: (string | number)[]): string => jsonPointer('services', ...segments);

const reservationOutsidePools: ValidatorDefinition = {
  name: 'services.kea-dhcp-relay-reservation-outside-pools',
  domains: ['services'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    for (const [name, server] of Object.entries(config.services.dhcp.servers)) {
      for (const [subnetName, subnet] of Object.entries(server.subnets)) {
        for (const [resName, res] of Object.entries(subnet.reservations)) {
          const pool = subnet.pools.findIndex(
            (p) =>
              ipFamily(p.start) === ipFamily(res.ip) &&
              (compareIp(p.start, res.ip) ?? 1) <= 0 &&
              (compareIp(res.ip, p.end) ?? 1) <= 0,
          );
          if (pool >= 0) {
            issues.push({
              pointer: P('dhcp', 'servers', name, 'subnets', subnetName, 'reservations', resName, 'ip'),
              message: `${res.ip} lies inside pool ${pool} (${subnet.pools[pool]?.start}-${subnet.pools[pool]?.end}); reserve an address outside the pools`,
            });
          }
        }
      }
    }
    return issues;
  },
};

const oneVrfPerFamily: ValidatorDefinition = {
  name: 'services.kea-dhcp-relay-one-vrf-per-family',
  domains: ['services'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const first = new Map<string, { server: string; vrf: string }>();
    for (const [name, server] of Object.entries(config.services.dhcp.servers)) {
      if (!server.enabled) continue;
      const seen = first.get(server.family);
      if (seen === undefined) {
        first.set(server.family, { server: name, vrf: server.vrf });
      } else if (seen.vrf !== server.vrf) {
        issues.push({
          pointer: P('dhcp', 'servers', name, 'vrf'),
          message: `one Kea ${server.family} instance serves every enabled ${server.family} server: VRF '${server.vrf}' differs from VRF '${seen.vrf}' of server '${seen.server}'`,
        });
      }
    }
    return issues;
  },
};

const relaySourcePerVrf: ValidatorDefinition = {
  name: 'services.kea-dhcp-relay-relay-source-per-vrf',
  domains: ['services'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const sources = new Map<string, { relay: string; source: string }>();
    const servers = new Map<string, string>();
    for (const [name, relay] of Object.entries(config.services.dhcp.relays)) {
      if (!relay.enabled) continue;
      const vrfKey = `${relay.family}|${relay.vrf}`;
      const seen = sources.get(vrfKey);
      if (seen === undefined) {
        sources.set(vrfKey, { relay: name, source: relay.sourceAddress });
      } else if (seen.source !== relay.sourceAddress) {
        issues.push({
          pointer: P('dhcp', 'relays', name, 'sourceAddress'),
          message: `VPP keeps one relay source address per client VRF and family: relay '${seen.relay}' already uses ${seen.source} in VRF '${relay.vrf}'`,
        });
      }
      const serverVrf = relay.serverVrf ?? relay.vrf;
      relay.servers.forEach((s, i) => {
        const key = `${vrfKey}|${serverVrf}|${s}`;
        const other = servers.get(key);
        if (other !== undefined && other !== name) {
          issues.push({
            pointer: P('dhcp', 'relays', name, 'servers', i),
            message: `${s} is already a server of relay '${other}' for client VRF '${relay.vrf}'`,
          });
        } else {
          servers.set(key, name);
        }
      });
    }
    return issues;
  },
};

const dhcpClientNoStatic: ValidatorDefinition = {
  name: 'interfaces.kea-dhcp-relay-dhcp-client-no-static',
  domains: ['interfaces'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const check = (path: string[], node: { ipv4: readonly string[]; dhcpClient?: unknown }): void => {
      if (node.dhcpClient !== undefined && node.ipv4.length > 0) {
        issues.push({
          pointer: jsonPointer('interfaces', ...path, 'ipv4'),
          message:
            'an interface with a DHCP client obtains its IPv4 address by DHCP: remove the static IPv4 addresses or the DHCP client',
        });
      }
    };
    for (const [name, itf] of Object.entries(config.interfaces)) {
      check([name], itf);
      for (const [id, sub] of Object.entries(itf.subinterfaces)) {
        check([name, 'subinterfaces', id], sub);
      }
    }
    return issues;
  },
};

export const keaDhcpRelayValidators: readonly ValidatorDefinition[] = [
  reservationOutsidePools,
  oneVrfPerFamily,
  relaySourcePerVrf,
  dhcpClientNoStatic,
];
