import {
  status,
  type handleServerStreamingCall,
  type handleUnaryCall,
  type ServiceError,
} from '@grpc/grpc-js';
import {
  type ActionOutput,
  type ActionRequest,
  type ListRoutesEntry,
  type ListRoutesPath,
  type ListRoutesRequest,
  type ListRoutesResponse,
} from '@ngfw/proto';
import { canonicalPrefix, ipFamily, isPlainObject, parseCidr, prefixesOverlap } from '@ngfw/schema';
import type { FakeAgent } from '../../testing/fake-agent.js';

/**
 * F-vrf-static-ecmp behaviour of the in-process fake agent (wave-A-hotspots P5; wired by one line in
 * testing/fake-agent.ts): `ListRoutes` over the applied document (connected prefixes = source "interface", static routes =
 * "API", blackhole = a drop path; viaFrr routes are FRR's and absent) with the agent's filters and paging, and `Action`
 * with the agent's ping rules (default VRF only, no source/size, count × interval ≤ 5 s → INVALID_ARGUMENT naming the
 * field) — every request answered unless `pingReplies` says otherwise. Traceroute and other actions: UNIMPLEMENTED.
 */
type Json = Record<string, unknown>;

export interface VrfStaticEcmpFakeState {
  /** Replies the fake ping reports for a target (default: all of them). */
  pingReplies?: (target: string, count: number) => number;
}

export const vrfStaticEcmpFakeState: VrfStaticEcmpFakeState = {};

const err = (code: status, details: string): Partial<ServiceError> => ({ code, details });

interface Row {
  vrf: string;
  entry: ListRoutesEntry;
}

function rows(doc: Json): Row[] {
  const out: Row[] = [];
  const connected = (name: string, node: Json) => {
    const vrf = typeof node['vrf'] === 'string' ? node['vrf'] : 'default';
    for (const fam of ['ipv4', 'ipv6']) {
      for (const a of (node[fam] as string[] | undefined) ?? []) {
        const prefix = canonicalPrefix(a);
        if (prefix === undefined) continue;
        out.push({
          vrf,
          entry: {
            prefix,
            source: 'interface',
            statsIndex: 0,
            paths: [path({ type: 'normal', interface: name })],
          },
        });
      }
    }
  };
  for (const [name, node] of Object.entries((doc['interfaces'] ?? {}) as Json)) {
    if (!isPlainObject(node)) continue;
    connected(name, node);
    for (const [sub, s] of Object.entries((node['subinterfaces'] ?? {}) as Json)) {
      if (isPlainObject(s)) connected(`${name}.${sub}`, s);
    }
  }
  const routing = (doc['routing'] ?? {}) as Json;
  for (const r of (routing['static'] as Json[] | undefined) ?? []) {
    if (r['viaFrr'] === true) continue; // D-072: FRR's, not programmed by the agent
    const hops = (r['nextHops'] as Json[] | undefined) ?? [];
    const distance = typeof r['distance'] === 'number' ? r['distance'] : 0;
    out.push({
      vrf: typeof r['vrf'] === 'string' ? r['vrf'] : 'default',
      entry: {
        prefix: canonicalPrefix(String(r['prefix'])) ?? String(r['prefix']),
        source: 'API',
        statsIndex: 0,
        paths:
          hops.length === 0
            ? [path({ type: 'drop', preference: distance })]
            : hops.map((h) =>
                path({
                  type: 'normal',
                  nextHop: typeof h['address'] === 'string' ? h['address'] : '',
                  interface: typeof h['interface'] === 'string' ? h['interface'] : '',
                  weight: typeof h['weight'] === 'number' ? h['weight'] : 1,
                  preference: distance,
                }),
              ),
      },
    });
  }
  return out;
}

function path(p: Partial<ListRoutesPath>): ListRoutesPath {
  return {
    type: 'normal',
    nextHop: '',
    interface: '',
    tableId: 0,
    weight: 1,
    preference: 0,
    flags: [],
    ...p,
  };
}

function compare(a: string, b: string): number {
  const x = parseCidr(a);
  const y = parseCidr(b);
  if (!x || !y) return a.localeCompare(b);
  if (x.family !== y.family) return x.family - y.family;
  if (x.address !== y.address) return x.address < y.address ? -1 : 1;
  return x.length - y.length;
}

export function vrfStaticEcmpFake(agent: FakeAgent): {
  listRoutes: handleUnaryCall<ListRoutesRequest, ListRoutesResponse>;
  action: handleServerStreamingCall<ActionRequest, ActionOutput>;
} {
  return {
    listRoutes: (call, cb) => {
      const r = call.request;
      agent.calls.push({ method: 'ListRoutes', request: r });
      if (r.owner && r.owner !== agent.owner)
        return cb(err(status.INVALID_ARGUMENT, `owner '${r.owner}' ≠ '${agent.owner}'`));
      const vrf = r.vrf || 'default';
      const vrfs = (agent.current['vrfs'] ?? {}) as Record<string, Json>;
      if (vrf !== 'default' && vrfs[vrf] === undefined)
        return cb(err(status.NOT_FOUND, `VRF "${vrf}" is not in the agent's configuration`));
      if (r.limit > 1000) return cb(err(status.INVALID_ARGUMENT, `limit ${r.limit} > 1000`));
      const within = r.prefix ? parseCidr(r.prefix) : undefined;
      if (r.prefix && within === undefined)
        return cb(err(status.INVALID_ARGUMENT, `prefix "${r.prefix}" is not a CIDR`));
      if (
        r.source &&
        !['API', 'interface', 'adjacency', 'svs', 'default-route', 'special'].includes(r.source)
      )
        return cb(err(status.INVALID_ARGUMENT, `unknown FIB source "${r.source}"`));
      const all = rows(agent.current)
        .filter((x) => x.vrf === vrf)
        .map((x) => x.entry)
        .filter((e) => !r.source || e.source === r.source)
        .filter(
          (e) =>
            !r.family || ipFamily(e.prefix.split('/')[0] ?? '') === (r.family === 'ipv6' ? 6 : 4),
        )
        .filter((e) => {
          if (within === undefined) return true;
          const p = parseCidr(e.prefix);
          return p !== undefined && p.length >= within.length && prefixesOverlap(p, within);
        })
        .sort((a, b) => compare(a.prefix, b.prefix));
      const limit = r.limit || 100;
      const table = vrf === 'default' ? 0 : Number((vrfs[vrf] ?? {})['id'] ?? 0);
      cb(null, {
        routes: all.slice(r.offset, r.offset + limit),
        total: all.length,
        owner: agent.owner,
        vrf,
        tableId: table,
        retrievedAt: new Date(),
      });
    },
    action: (call) => {
      agent.calls.push({ method: 'Action', request: call.request });
      // emit('error'): grpc-js turns it into the call's status and ends the stream (destroy() would never send one)
      const fail = (code: status, details: string) =>
        call.emit('error', Object.assign(new Error(details), { code, details }));
      const ping = call.request.ping;
      if (ping === undefined) {
        return fail(
          status.UNIMPLEMENTED,
          call.request.traceroute !== undefined
            ? 'not implemented by VPP: traceroute: VPP has no traceroute API (fake)'
            : 'actions are not implemented',
        );
      }
      const bad = (field: string, msg: string) =>
        fail(status.INVALID_ARGUMENT, `invalid action argument: ${field}: ${msg}`);
      if (ipFamily(ping.target) === undefined)
        return bad('target', `"${ping.target}" is not an address`);
      if (ping.vrf && ping.vrf !== 'default')
        return bad('vrf', `ping in VRF "${ping.vrf}" (default only)`);
      if (ping.source) return bad('source', 'a source address is not supported');
      if (ping.size) return bad('size', 'a payload size is not supported');
      const count = ping.count || 5;
      const interval = ping.intervalMs || 1000;
      if (count > 100 || count * interval > 5000) return bad('count', 'count × interval > 5 s');
      if (interval < 100) return bad('interval_ms', `${interval} ms < 100 ms`);
      const received = Math.min(
        count,
        vrfStaticEcmpFakeState.pingReplies?.(ping.target, count) ?? count,
      );
      const loss = Math.round((100 * (count - received)) / count);
      const summary = `PING ${ping.target} (default VRF): ${count} packets transmitted, ${received} received, ${loss}% packet loss`;
      call.write({ line: summary });
      call.write({
        done: {
          summary,
          exitCode: received > 0 ? 0 : 1,
          stats: { transmitted: String(count), received: String(received), loss_pct: String(loss) },
        },
      });
      call.end();
    },
  };
}
