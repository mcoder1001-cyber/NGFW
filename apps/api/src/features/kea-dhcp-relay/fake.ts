import { status, type handleUnaryCall } from '@grpc/grpc-js';
import type {
  DhcpClientLease,
  DhcpLease,
  DhcpLeasesRequest,
  DhcpLeasesResponse,
  DhcpServerStatus,
} from '@ngfw/proto';
import type { FakeAgent } from '../../testing/fake-agent.js';

/**
 * The fake agent's DhcpLeases (wave-A-hotspots P5). Leases and VPP DHCPv4 client states are what a test sets with
 * `setFakeDhcp`; server status lists the subnets of the state the fake holds (`current.services.dhcp.servers`, i.e.
 * what was applied) with the usage a test set. Paging, filtering and the owner / forced-failure / "services not
 * implemented" checks behave like the real agent (proto.md §11).
 */
export interface FakeDhcpState {
  leases: DhcpLease[];
  clients: Record<string, Partial<DhcpClientLease>>;
  /** subnet id → {assigned, total} for the usage of applied subnets (ids are assigned in document order from 1). */
  usage: Record<number, { assigned: number; total: number }>;
  running: { ipv4: boolean; ipv6: boolean };
}

const states = new WeakMap<FakeAgent, FakeDhcpState>();

type Json = Record<string, unknown>;

/** The fake's DHCP state (created on first use). */
export function fakeDhcp(agent: FakeAgent): FakeDhcpState {
  let s = states.get(agent);
  if (!s) {
    s = { leases: [], clients: {}, usage: {}, running: { ipv4: true, ipv6: true } };
    states.set(agent, s);
  }
  return s;
}

/** Replace parts of the fake's DHCP state. */
export function setFakeDhcp(agent: FakeAgent, patch: Partial<FakeDhcpState>): void {
  Object.assign(fakeDhcp(agent), patch);
}

/** A full DhcpLease from the fields a test cares about. */
export function fakeLease(l: Partial<DhcpLease> & { address: string }): DhcpLease {
  return {
    family: 'ipv4',
    hwAddress: '',
    clientId: '',
    duid: '',
    hostname: '',
    server: '',
    subnet: '',
    subnetId: 0,
    validLifetimeSec: 3600,
    expiresAt: undefined,
    state: 'default',
    leaseType: '',
    prefixLen: 0,
    ...l,
  };
}

function ipKey(a: string): string {
  return a.includes(':')
    ? a
    : a
        .split('.')
        .map((x) => x.padStart(3, '0'))
        .join('.');
}

export function dhcpLeases(
  agent: FakeAgent,
): handleUnaryCall<DhcpLeasesRequest, DhcpLeasesResponse> {
  return (call, cb) => {
    const req = call.request;
    agent.calls.push({ method: 'DhcpLeases', request: req });
    if (agent.failAllWith !== undefined) {
      cb({ code: agent.failAllWith, details: `fake agent: forced ${status[agent.failAllWith]}` });
      return;
    }
    if (req.owner && req.owner !== agent.owner) {
      cb({
        code: status.INVALID_ARGUMENT,
        details: `owner '${req.owner}' ≠ agent owner '${agent.owner}'`,
      });
      return;
    }
    if (!agent.implemented.includes('services')) {
      cb({ code: status.UNIMPLEMENTED, details: 'fake agent: services not implemented' });
      return;
    }
    if (req.family !== '' && req.family !== 'ipv4' && req.family !== 'ipv6') {
      cb({ code: status.INVALID_ARGUMENT, details: `family '${req.family}'` });
      return;
    }
    const st = fakeDhcp(agent);
    const page = req.page || 1;
    const pageSize = req.pageSize || 100;
    const base = { owner: agent.owner, page, pageSize, retrievedAt: new Date() };
    if (req.interface !== '') {
      const c = st.clients[req.interface];
      cb(null, {
        ...base,
        leases: [],
        total: 0,
        truncated: false,
        servers: [],
        client: {
          interface: req.interface,
          configured: c !== undefined,
          state: '',
          address: '',
          router: '',
          dnsServers: [],
          hostname: '',
          mac: '',
          ...c,
        },
      });
      return;
    }
    const servers = ((
      (agent.current['services'] as Json | undefined)?.['dhcp'] as Json | undefined
    )?.['servers'] ?? {}) as Record<string, Json>;
    let id = 0;
    const status4: DhcpServerStatus[] = (['ipv4', 'ipv6'] as const)
      .filter((f) => req.family === '' || req.family === f)
      .map((family) => {
        const subnets = Object.entries(servers)
          .filter(([, s]) => (s['family'] ?? 'ipv4') === family)
          .flatMap(([server, s]) =>
            Object.entries((s['subnets'] ?? {}) as Record<string, Json>).map(([subnet, sub]) => {
              id += 1;
              const u = st.usage[id] ?? { assigned: 0, total: 0 };
              return {
                server,
                subnet,
                prefix: String(sub['subnet'] ?? ''),
                subnetId: id,
                total: String(u.total),
                assigned: String(u.assigned),
                declined: '0',
              };
            }),
          );
        const running = st.running[family];
        const active = subnets.length > 0;
        return {
          family,
          running,
          active,
          actionRequired: active && !running ? 'start' : '',
          reloadSec: '1',
          subnets,
          error: '',
        };
      });
    const needle = req.filter.toLowerCase();
    const all = st.leases
      .filter((l) => req.family === '' || l.family === req.family)
      .filter((l) => req.server === '' || l.server === req.server)
      .filter(
        (l) =>
          needle === '' ||
          [l.address, l.hwAddress, l.clientId, l.duid, l.hostname.toLowerCase()].some((x) =>
            x.includes(needle),
          ),
      )
      .sort((a, b) =>
        a.family === b.family
          ? ipKey(a.address).localeCompare(ipKey(b.address))
          : a.family.localeCompare(b.family),
      );
    cb(null, {
      ...base,
      leases: all.slice((page - 1) * pageSize, page * pageSize),
      total: all.length,
      truncated: false,
      servers: status4,
      client: undefined,
    });
  };
}
