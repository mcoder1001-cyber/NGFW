import { status, type handleUnaryCall } from '@grpc/grpc-js';
import type { ListNeighborsRequest, ListNeighborsResponse, NeighborEntry } from '@ngfw/proto';
import type { FakeAgent } from '../../testing/fake-agent.js';

/**
 * Fake-agent behaviour of F-neighbors-ra (wired by the one line under the F-neighbors-ra anchor of
 * `testing/fake-agent.ts`, wave-A-hotspots P5). ListNeighbors answers from the fake's applied state — the static
 * neighbours of `routing.neighbors.static` — plus learned entries a test adds with {@link setFakeNeighbors}, filtered,
 * sorted and paged the way the agent does it (docs/contracts/proto.md §11). It never touches VPP.
 */
type Json = Record<string, unknown>;

const learned = new WeakMap<FakeAgent, NeighborEntry[]>();

/** Learned (dynamic) entries the fake reports in addition to the configured static ones. */
export function setFakeNeighbors(fake: FakeAgent, rows: Partial<NeighborEntry>[]): void {
  learned.set(
    fake,
    rows.map((r) => ({
      interface: '',
      ip: '',
      mac: '',
      family: (r.ip ?? '').includes(':') ? 'ipv6' : 'ipv4',
      state: 'dynamic',
      noFibEntry: false,
      ageSec: 0,
      vrf: 'default',
      tableId: 0,
      ...r,
    })),
  );
}

function staticRows(current: Json): NeighborEntry[] {
  const routing = (current['routing'] ?? {}) as Json;
  const nb = (routing['neighbors'] ?? {}) as Json;
  const ifs = (current['interfaces'] ?? {}) as Record<string, Json>;
  const vrfs = (current['vrfs'] ?? {}) as Record<string, Json>;
  const vrfOf = (name: string): string => {
    const [parent, sub] = name.split(/\.(?=[0-9]+$)/);
    const node =
      sub === undefined
        ? ifs[name]
        : ((ifs[parent!]?.['subinterfaces'] ?? {}) as Record<string, Json>)[sub];
    return typeof node?.['vrf'] === 'string' ? node['vrf'] : 'default';
  };
  return ((nb['static'] as Json[] | undefined) ?? []).map((s) => {
    const ip = String(s['ip']);
    const vrf = vrfOf(String(s['interface']));
    return {
      interface: String(s['interface']),
      ip,
      mac: String(s['mac']).toLowerCase(),
      family: ip.includes(':') ? 'ipv6' : 'ipv4',
      state: 'static',
      noFibEntry: s['noFibEntry'] === true,
      ageSec: 0,
      vrf,
      tableId: typeof vrfs[vrf]?.['id'] === 'number' ? (vrfs[vrf]['id'] as number) : 0,
    };
  });
}

const cmp: Record<string, (a: NeighborEntry, b: NeighborEntry) => number> = {
  interface: (a, b) => a.interface.localeCompare(b.interface),
  ip: (a, b) => a.ip.localeCompare(b.ip),
  mac: (a, b) => a.mac.localeCompare(b.mac),
  age: (a, b) => a.ageSec - b.ageSec,
  vrf: (a, b) => a.vrf.localeCompare(b.vrf),
  state: (a, b) => a.state.localeCompare(b.state),
};

export function neighborsRaFake(fake: FakeAgent): {
  listNeighbors: handleUnaryCall<ListNeighborsRequest, ListNeighborsResponse>;
} {
  return {
    listNeighbors: (call, cb) => {
      const r = call.request;
      fake.calls.push({ method: 'ListNeighbors', request: r });
      if (fake.failAllWith !== undefined) {
        cb({ code: fake.failAllWith, details: 'fake agent: failing every call' });
        return;
      }
      if (r.owner && r.owner !== fake.owner) {
        cb({
          code: status.INVALID_ARGUMENT,
          details: `owner "${r.owner}" does not match "${fake.owner}"`,
        });
        return;
      }
      if (r.limit > 1000) {
        cb({ code: status.INVALID_ARGUMENT, details: `limit ${r.limit} exceeds 1000` });
        return;
      }
      const search = r.search.toLowerCase();
      const rows = [...staticRows(fake.current), ...(learned.get(fake) ?? [])].filter(
        (n) =>
          (!r.vrf || n.vrf === r.vrf) &&
          (!r.interface || n.interface === r.interface) &&
          (!r.family || n.family === r.family) &&
          (!r.state || n.state === r.state) &&
          (!search ||
            n.ip.includes(search) ||
            n.mac.includes(search) ||
            n.interface.toLowerCase().includes(search)),
      );
      const by = cmp[r.sort || 'interface'] ?? cmp['interface']!;
      rows.sort((a, b) => (r.descending ? -1 : 1) * (by(a, b) || cmp['ip']!(a, b)));
      const limit = r.limit || 100;
      cb(null, {
        neighbors: rows.slice(r.offset, r.offset + limit),
        total: rows.length,
        owner: fake.owner,
        retrievedAt: new Date(),
      });
    },
  };
}
