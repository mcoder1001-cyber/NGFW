import {
  status,
  type handleServerStreamingCall,
  type handleUnaryCall,
  type ServerWritableStream,
} from '@grpc/grpc-js';
import type {
  ActionOutput,
  ActionRequest,
  DataplaneServer,
  DnsStateRequest,
  DnsStateResponse,
  NtpStateRequest,
  NtpStateResponse,
  SyslogEntriesRequest,
  SyslogEntriesResponse,
  SyslogEntry,
  SyslogStateRequest,
  SyslogStateResponse,
} from '@ngfw/proto';

/**
 * F-unbound-chrony-syslog behaviour of the in-process fake agent (wave-A-hotspots P5): the four state RPCs answer from
 * the fake's applied configuration (as if the daemons ran it), the log explorer pages a fixed journal, and
 * ActionRequest.dns_lookup resolves the A/AAAA records of the configured local zones — the path a slot cannot run
 * against the real VPP (the dns plugin is enabled only by the globals owner, D-071). Never touches a daemon.
 */

type Json = Record<string, unknown>;

/** What the fake needs from FakeAgent (structural: no import cycle). */
export interface FakeHost {
  readonly owner: string;
  current: Json;
}

/** The fixed journal of the fake log explorer, newest first. */
export const FAKE_JOURNAL: SyslogEntry[] = [
  ['2026-09-25T03:00:05Z', 'info', 'daemon', 'unbound', 'info: service stopped (unbound 1.24.2).'],
  ['2026-09-25T03:00:04Z', 'warning', 'daemon', 'chronyd', 'System clock wrong by 1.2 seconds'],
  ['2026-09-25T03:00:03Z', 'error', 'local7', 'vrx-test', 'forwarded test line'],
  ['2026-09-25T03:00:02Z', 'notice', 'auth', 'sshd', 'Accepted publickey for root'],
  ['2026-09-25T03:00:01Z', 'debug', 'kern', 'kernel', 'eth0: link up'],
].map(([time, severity, facility, identifier, message], i) => ({
  time: new Date(time!),
  severity: severity!,
  facility: facility!,
  identifier: identifier!,
  pid: 100 + i,
  hostname: 'vrx-fake',
  unit: '',
  message: message!,
}));

const SEVERITIES = [
  'emergency',
  'alert',
  'critical',
  'error',
  'warning',
  'notice',
  'info',
  'debug',
];

function resolvers(host: FakeHost): Record<string, Json> {
  const dns = ((host.current['services'] ?? {}) as Json)['dns'] as Json | undefined;
  return ((dns?.['resolvers'] ?? {}) as Record<string, Json>) ?? {};
}

function absolute(name: string): string {
  const n = name.toLowerCase();
  return n.endsWith('.') ? n : `${n}.`;
}

export function unboundChronySyslogFake(
  host: FakeHost,
): Pick<DataplaneServer, 'dnsState' | 'ntpState' | 'syslogState' | 'syslogEntries'> &
  Partial<Pick<DataplaneServer, 'action'>> {
  const ownerOk = (owner: string | undefined): boolean => !owner || owner === host.owner;
  const denied = { code: status.INVALID_ARGUMENT, details: 'owner mismatch' };

  const dnsState: handleUnaryCall<DnsStateRequest, DnsStateResponse> = (call, cb) => {
    if (!ownerOk(call.request.owner)) return cb(denied, null);
    const rs = resolvers(host);
    const enabled = Object.values(rs).filter((r) => r['enabled'] !== false);
    const forwards: DnsStateResponse['forwards'] = [];
    const localZones: DnsStateResponse['localZones'] = [];
    const localData: string[] = [];
    for (const r of enabled) {
      for (const f of (r['forwardZones'] ?? []) as Json[]) {
        forwards.push({
          zone: absolute(String(f['zone'])),
          kind: 'forward',
          flags: [],
          addresses: ((f['forwarders'] ?? []) as Json[]).map((u) => String(u['address'])),
        });
      }
      for (const z of (r['localZones'] ?? []) as Json[]) {
        localZones.push({ zone: absolute(String(z['zone'])), type: String(z['type'] ?? 'static') });
        for (const rec of (z['records'] ?? []) as Json[]) {
          localData.push(
            `${absolute(String(rec['name']))} ${Number(rec['ttlSec'] ?? 3600)} IN ${String(rec['type'])} ${String(rec['data'])}`,
          );
        }
      }
    }
    const vc = (((host.current['services'] ?? {}) as Json)['dns'] as Json | undefined)?.[
      'vppCache'
    ] as Json | undefined;
    cb(null, {
      owner: host.owner,
      retrievedAt: new Date(),
      running: enabled.length > 0,
      status: enabled.length > 0 ? { version: '1.24.2', state: 'is running...' } : {},
      stats:
        enabled.length > 0
          ? { 'total.num.queries': '42', 'total.num.cachehits': '40', 'total.num.cachemiss': '2' }
          : {},
      forwards,
      stubs: [],
      localZones,
      localData: localData.sort(),
      localDataTruncated: false,
      pendingActions: [],
      vppCache: {
        configured: vc?.['enabled'] === true,
        appliedByThisAgent: false,
        upstreams: ((vc?.['upstreams'] ?? []) as string[]).slice().sort(),
      },
      configPath: '/run/vrx-test/fake/unbound/unbound.conf',
      error: '',
    });
  };

  const ntpState: handleUnaryCall<NtpStateRequest, NtpStateResponse> = (call, cb) => {
    if (!ownerOk(call.request.owner)) return cb(denied, null);
    const ntp = (((host.current['services'] ?? {}) as Json)['ntp'] ?? {}) as Json;
    const on = ntp['enabled'] === true;
    const servers = ((ntp['servers'] ?? []) as Json[]).map((s) => String(s['address']));
    cb(null, {
      owner: host.owner,
      retrievedAt: new Date(),
      running: on,
      tracking: on
        ? {
            refId: '7F000001',
            refName: servers[0] ?? '',
            stratum: 3,
            refTime: 0,
            systemTime: 0.000012,
            lastOffset: 0.000003,
            rmsOffset: 0.00001,
            frequency: -1.2,
            residualFreq: 0,
            skew: 0.1,
            rootDelay: 0.001,
            rootDispersion: 0.0005,
            updateInterval: 64,
            leap: 'Normal',
          }
        : undefined,
      sources: on
        ? servers.map((name, i) => ({
            mode: '^',
            state: i === 0 ? '*' : '+',
            name,
            stratum: 2,
            poll: 6,
            reach: '377',
            lastRx: '12',
            offset: 0.000002,
            measured: 0.000002,
            error: 0.0001,
          }))
        : [],
      sourceStats: [],
      serverStats: {},
      pendingActions: [],
      configPath: '/run/vrx-test/fake/chrony/agent/chrony.conf',
      error: '',
    });
  };

  const syslogState: handleUnaryCall<SyslogStateRequest, SyslogStateResponse> = (call, cb) => {
    if (!ownerOk(call.request.owner)) return cb(denied, null);
    const targets = (((host.current['management'] ?? {}) as Json)['syslog'] ?? []) as Json[];
    cb(null, {
      owner: host.owner,
      retrievedAt: new Date(),
      targets: targets.map((t, i) => ({
        index: i,
        action: `vrx_export_${i}_00000000`,
        target: `${String(t['address'])}:${Number(t['port'] ?? 514)}`,
        protocol: t['protocol'] === 'udp' || t['protocol'] === undefined ? 'udp' : 'tcp',
        reported: true,
        processed: '7',
        failed: '0',
        suspended: '0',
        suspendedDuration: '0',
        resumed: '0',
        queueSize: '0',
        enqueued: '7',
        full: '0',
        discardedFull: '0',
        discardedNf: '0',
        maxQueueSize: '1',
      })),
      inputs: { imuxsock: '9' },
      pendingActions: [],
      configPath: '/run/vrx-test/fake/rsyslog/rsyslog.conf',
      error: '',
    });
  };

  const syslogEntries: handleUnaryCall<SyslogEntriesRequest, SyslogEntriesResponse> = (
    call,
    cb,
  ) => {
    const r = call.request;
    if (!ownerOk(r.owner)) return cb(denied, null);
    const pageSize = r.pageSize || 100;
    const page = r.page || 1;
    if (r.severity && !SEVERITIES.includes(r.severity)) {
      return cb({ code: status.INVALID_ARGUMENT, details: `severity ${r.severity}` }, null);
    }
    const max = r.severity ? SEVERITIES.indexOf(r.severity) : 7;
    const q = r.query.toLowerCase();
    const matches = FAKE_JOURNAL.filter(
      (e) =>
        SEVERITIES.indexOf(e.severity) <= max &&
        (!r.facility || e.facility === r.facility) &&
        (!q || e.message.toLowerCase().includes(q) || e.identifier.toLowerCase().includes(q)) &&
        (!r.since || (e.time !== undefined && e.time >= r.since)),
    );
    cb(null, {
      owner: host.owner,
      retrievedAt: new Date(),
      entries: matches.slice((page - 1) * pageSize, page * pageSize),
      page,
      pageSize,
      total: matches.length,
      truncated: false,
      scanned: FAKE_JOURNAL.length,
      source: 'journald',
    });
  };

  const action: handleServerStreamingCall<ActionRequest, ActionOutput> = (
    call: ServerWritableStream<ActionRequest, ActionOutput>,
  ) => {
    const lookup = call.request.dnsLookup;
    if (!lookup) {
      call.destroy(
        Object.assign(new Error('actions are not implemented'), { code: status.UNIMPLEMENTED }),
      );
      return;
    }
    const want = absolute(lookup.name);
    let ipv4 = '';
    let ipv6 = '';
    for (const r of Object.values(resolvers(host))) {
      for (const z of (r['localZones'] ?? []) as Json[]) {
        for (const rec of (z['records'] ?? []) as Json[]) {
          if (absolute(String(rec['name'])) !== want) continue;
          if (rec['type'] === 'A' && !ipv4) ipv4 = String(rec['data']);
          if (rec['type'] === 'AAAA' && !ipv6) ipv6 = String(rec['data']);
        }
      }
    }
    const stats: Record<string, string> = {};
    if (ipv4) {
      stats['ipv4'] = ipv4;
      call.write({ line: `A ${ipv4}` });
    }
    if (ipv6) {
      stats['ipv6'] = ipv6;
      call.write({ line: `AAAA ${ipv6}` });
    }
    const found = Object.keys(stats).length;
    call.write({
      done: {
        summary: found
          ? `${lookup.name}: ${found} address(es)`
          : `${lookup.name}: lookup failed: fake: no such name`,
        exitCode: found ? 0 : 1,
        stats,
      },
    });
    call.end();
  };

  return { dnsState, ntpState, syslogState, syslogEntries, action };
}
