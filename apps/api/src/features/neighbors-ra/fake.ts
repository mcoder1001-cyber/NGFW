import { status, type handleServerStreamingCall, type handleUnaryCall } from '@grpc/grpc-js';
import type {
  ActionOutput,
  ActionRequest,
  ListNeighborsRequest,
  ListNeighborsResponse,
  NeighborEntry,
} from '@ngfw/proto';
import type { FakeAgent } from '../../testing/fake-agent.js';

/**
 * Fake-agent behaviour of F-neighbors-ra (wired by the one line under the F-neighbors-ra anchor of
 * `testing/fake-agent.ts`, wave-A-hotspots P5). ListNeighbors answers from the fake's applied state — the static
 * neighbours of `routing.neighbors.static` — plus learned entries a test adds with {@link setFakeNeighbors}, filtered,
 * sorted and paged the way the agent does it (docs/contracts/proto.md §11). The `arp_flush` Action deletes the fake's
 * learned entries of one configured interface (a name outside the configuration → INVALID_ARGUMENT, like the agent,
 * review M1) or of every configured one, and streams lines + `done`. It never touches VPP.
 *
 * The fake's Action handler is shared: this module's `action` replaces the generic one (the spread comes after it in
 * `impl()`) and answers every OTHER action exactly like the generic handler as F-nat44-ed-sessions fixed it
 * (`emit('error', UNIMPLEMENTED)`, which reaches the client — `destroy` did not). When F-vrf-static-ecmp's Action
 * dispatch lands in the fake, the merger folds `arpFlush` into it as one case.
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

/** The configured (sub-)interface names of the fake's applied state. */
function configuredNames(current: Json): string[] {
  const out: string[] = [];
  for (const [name, itf] of Object.entries((current['interfaces'] ?? {}) as Record<string, Json>)) {
    out.push(name);
    for (const id of Object.keys((itf['subinterfaces'] ?? {}) as Json)) out.push(`${name}.${id}`);
  }
  return out.sort();
}

const grpcError = (code: status, details: string) =>
  Object.assign(new Error(details), { code, details });

function arpFlush(
  fake: FakeAgent,
  call: Parameters<handleServerStreamingCall<ActionRequest, ActionOutput>>[0],
): void {
  const req = call.request.arpFlush!;
  if (fake.failAllWith !== undefined) {
    call.emit('error', grpcError(fake.failAllWith, 'fake agent: failing every call'));
    return;
  }
  if (req.family !== '' && req.family !== 'ipv4' && req.family !== 'ipv6') {
    call.emit(
      'error',
      grpcError(
        status.INVALID_ARGUMENT,
        `invalid request: family "${req.family}" is not ipv4 or ipv6`,
      ),
    );
    return;
  }
  const configured = configuredNames(fake.current);
  if (req.interface !== '' && !configured.includes(req.interface)) {
    call.emit(
      'error',
      grpcError(
        status.INVALID_ARGUMENT,
        `interface "${req.interface}" is not an interface of this configuration (configured, or created by owner "${fake.owner}"): refusing to flush it`,
      ),
    );
    return;
  }
  const targets = req.interface !== '' ? [req.interface] : configured;
  let rows = learned.get(fake) ?? [];
  let deleted = 0;
  for (const name of targets) {
    for (const family of req.family !== '' ? [req.family] : ['ipv4', 'ipv6']) {
      const gone = rows.filter(
        (n) => n.interface === name && n.family === family && n.state === 'dynamic',
      );
      rows = rows.filter((n) => !gone.includes(n));
      deleted += gone.length;
      call.write({ line: `${name} ${family}: deleted ${gone.length} learned entries` });
    }
  }
  learned.set(fake, rows);
  call.write({
    done: {
      summary: `deleted ${deleted} learned entries on ${targets.length} interfaces`,
      exitCode: 0,
      stats: { deleted: String(deleted), interfaces: String(targets.length) },
    },
  });
  call.end();
}

export function neighborsRaFake(fake: FakeAgent): {
  listNeighbors: handleUnaryCall<ListNeighborsRequest, ListNeighborsResponse>;
  // optional in the type only: it replaces the generic handler that precedes the spread in impl() (TS2783 otherwise)
  action?: handleServerStreamingCall<ActionRequest, ActionOutput>;
} {
  return {
    action: (call) => {
      fake.calls.push({ method: 'Action', request: call.request });
      if (call.request.arpFlush !== undefined) {
        arpFlush(fake, call);
        return;
      }
      call.emit('error', grpcError(status.UNIMPLEMENTED, 'actions are not implemented'));
    },
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
