import { status, type handleUnaryCall, type ServiceError } from '@grpc/grpc-js';
import type {
  BgpInstanceState,
  ListRoutesRequest,
  ListRoutesResponse,
  RoutingRibEntry,
  RoutingStateRequest,
  RoutingStateResponse,
} from '@ngfw/proto';
import { canonicalPrefix, isPlainObject } from '@ngfw/schema';
import type { FakeAgent } from '../../testing/fake-agent.js';
import { vrfStaticEcmpFake } from '../vrf-static-ecmp/fake.js';

/**
 * P12 behaviour of the in-process fake agent (wave-A-hotspots P5; wired by one line in testing/fake-agent.ts):
 *
 *   - `RoutingState`: the BGP instance of the applied `routing.bgp` with every neighbour in the state `bgpFakeState`
 *     says (default Established, 100 prefixes received), linux-cp pairs from `interfaces.<n>.lcp`, a RIB lookup answered
 *     from `bgpFakeState.frrRoutes`; FRR "not running" while no BGP is applied;
 *   - `ListRoutes`: F-vrf-static-ecmp's fake, plus the FRR routes of `bgpFakeState.frrRoutes` as source `lcp-rt-dynamic`
 *     (a linux-nl route) — F-vrf's fake rejects that source name, so this wrapper answers it and delegates the rest.
 */
type Json = Record<string, unknown>;

export interface FrrFakeRoute {
  vrf: string;
  prefix: string;
  protocol: string;
  nextHop: string;
}

export interface BgpFakeState {
  /** Neighbour state by address (default "Established"). */
  neighborState: Record<string, string>;
  /** Prefixes received per neighbour (default 100 when Established, else 0). */
  prefixesReceived: Record<string, number>;
  /** Routes FRR installed (and linux-nl synced into VPP). */
  frrRoutes: FrrFakeRoute[];
}

export const bgpFakeState: BgpFakeState = {
  neighborState: {},
  prefixesReceived: {},
  frrRoutes: [],
};

const err = (code: status, details: string): Partial<ServiceError> => ({ code, details });

function bgpInstances(doc: Json): BgpInstanceState[] {
  const routing = isPlainObject(doc['routing']) ? doc['routing'] : {};
  const bgp = routing['bgp'];
  if (!isPlainObject(bgp)) return [];
  const neighbors = isPlainObject(bgp['neighbors']) ? bgp['neighbors'] : {};
  const groups = isPlainObject(bgp['peerGroups']) ? bgp['peerGroups'] : {};
  return [
    {
      vrf: typeof bgp['vrf'] === 'string' ? bgp['vrf'] : 'default',
      asn: Number(bgp['asn'] ?? 0),
      routerId: typeof bgp['routerId'] === 'string' ? bgp['routerId'] : '0.0.0.0',
      neighbors: Object.entries(neighbors)
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([address, raw]) => {
          const n = isPlainObject(raw) ? raw : {};
          const group =
            typeof n['peerGroup'] === 'string' && isPlainObject(groups[n['peerGroup']])
              ? (groups[n['peerGroup']] as Json)
              : {};
          const shut = n['shutdown'] === true;
          const state = shut
            ? 'Idle (Admin)'
            : (bgpFakeState.neighborState[address] ?? 'Established');
          const up = state === 'Established';
          const rx = up ? (bgpFakeState.prefixesReceived[address] ?? 100) : 0;
          return {
            address,
            remoteAs: Number(n['remoteAs'] ?? group['remoteAs'] ?? 0),
            state,
            uptimeSec: up ? '3600' : '0',
            prefixesReceived: rx,
            prefixesSent: up ? 1 : 0,
            flaps: 0,
            established: up ? 1 : 0,
            description: typeof n['description'] === 'string' ? n['description'] : '',
            afis: [{ afi: 'ipv4Unicast', prefixesReceived: rx, prefixesSent: up ? 1 : 0 }],
            messagesReceived: up ? '10' : '0',
            messagesSent: up ? '10' : '0',
          };
        }),
    },
  ];
}

export function bgpFake(agent: FakeAgent): {
  routingState: handleUnaryCall<RoutingStateRequest, RoutingStateResponse>;
  listRoutes: handleUnaryCall<ListRoutesRequest, ListRoutesResponse>;
} {
  const inner = vrfStaticEcmpFake(agent).listRoutes;
  return {
    routingState: (call, cb) => {
      const r = call.request;
      agent.calls.push({ method: 'RoutingState', request: r });
      if (r.owner && r.owner !== agent.owner)
        return cb(err(status.INVALID_ARGUMENT, `owner '${r.owner}' ≠ '${agent.owner}'`));
      if (r.ribPrefixes.length > 100)
        return cb(
          err(status.INVALID_ARGUMENT, `${r.ribPrefixes.length} rib_prefixes, at most 100`),
        );
      const unknown = r.readers.find((k) => k !== 'bgpSummary');
      if (unknown !== undefined)
        return cb(err(status.INVALID_ARGUMENT, `unknown state reader "${unknown}"`));
      const doc = agent.current;
      const bgp = bgpInstances(doc);
      const running = bgp.length > 0;
      const ifs = isPlainObject(doc['interfaces']) ? doc['interfaces'] : {};
      const lcpPairs = Object.entries(ifs)
        .filter(([, v]) => isPlainObject(v) && isPlainObject(v['lcp']))
        .map(([name, v], i) => {
          const l = (v as Json)['lcp'] as Json;
          return {
            interface: name,
            hostIfName: typeof l['hostIfName'] === 'string' ? l['hostIfName'] : name,
            hostIfType: typeof l['hostIfType'] === 'string' ? l['hostIfType'] : 'tap',
            netns: typeof l['netns'] === 'string' ? l['netns'] : '',
            phySwIfIndex: 10 + i,
            hostSwIfIndex: 100 + i,
            vifIndex: 200 + i,
          };
        });
      const vrf = r.ribVrf || 'default';
      const rib: RoutingRibEntry[] = [];
      for (const p of r.ribPrefixes) {
        const c = canonicalPrefix(p);
        if (c === undefined || c !== p)
          return cb(err(status.INVALID_ARGUMENT, `rib_prefixes: "${p}" is not canonical`));
        for (const fr of bgpFakeState.frrRoutes.filter((x) => x.vrf === vrf && x.prefix === p)) {
          rib.push({
            prefix: p,
            vrf,
            protocol: fr.protocol,
            selected: true,
            installed: true,
            distance: 20,
            metric: 0,
            nextHops: [{ address: fr.nextHop, interface: '', active: true, fib: true }],
          });
        }
      }
      const counts: Record<string, number> = {};
      for (const fr of bgpFakeState.frrRoutes) {
        const k = `${fr.prefix.includes(':') ? 'ipv6' : 'ipv4'}/${fr.vrf}/${fr.protocol}`;
        counts[k] = (counts[k] ?? 0) + 1;
      }
      cb(null, {
        owner: agent.owner,
        retrievedAt: new Date(),
        frrRunning: running,
        frrVersion: running ? '10.7.1' : '',
        error: running ? '' : 'FRR is not running (fake: no routing.bgp applied)',
        bgp,
        ribCounts: running ? counts : {},
        lcpPairs,
        readers: running && r.readers.includes('bgpSummary') ? { bgpSummary: '{}' } : {},
        rib: running ? rib : [],
      });
    },
    listRoutes: (call, cb) => {
      const r = call.request;
      if (r.source !== 'lcp-rt-dynamic') return inner(call, cb);
      agent.calls.push({ method: 'ListRoutes', request: r });
      const vrf = r.vrf || 'default';
      const all = bgpFakeState.frrRoutes
        .filter((x) => x.vrf === vrf)
        .map((x) => ({
          prefix: x.prefix,
          source: 'lcp-rt-dynamic',
          statsIndex: 0,
          paths: [
            {
              type: 'normal',
              nextHop: x.nextHop,
              interface: '',
              tableId: 0,
              weight: 1,
              preference: 0,
              flags: [],
            },
          ],
        }));
      const limit = r.limit || 100;
      cb(null, {
        routes: all.slice(r.offset, r.offset + limit),
        total: all.length,
        owner: agent.owner,
        vrf,
        tableId: 0,
        retrievedAt: new Date(),
      });
    },
  };
}
