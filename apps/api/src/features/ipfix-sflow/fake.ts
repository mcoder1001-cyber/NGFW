import { status, type handleUnaryCall } from '@grpc/grpc-js';
import type { IpfixExporterState, IpfixStateRequest, IpfixStateResponse } from '@ngfw/proto';

type Json = Record<string, unknown>;

/** What the handler needs from FakeAgent (structural, so the fake needs no new accessors). */
export interface IpfixFakeHost {
  readonly owner: string;
  current: Json;
  calls: { method: string; request: unknown }[];
  failAllWith: status | undefined;
}

const obj = (v: unknown): Json => (v !== null && typeof v === 'object' ? (v as Json) : {});
const num = (v: unknown, d: number): number => (typeof v === 'number' ? v : d);
const bool = (v: unknown, d: boolean): boolean => (typeof v === 'boolean' ? v : d);

/**
 * Fake IpfixState (F-ipfix-sflow, wave-A-hotspots P5): derived from the applied `services.ipfix` the way the agent
 * projects it — the first enabled IPv4 exporter (by name) is exporter 0, flowprobe interfaces with their single
 * variant, sFlow interfaces with hw index = 100 + position. The fake is the globals owner.
 */
export function ipfixStateFake(
  host: IpfixFakeHost,
): handleUnaryCall<IpfixStateRequest, IpfixStateResponse> {
  return (call, cb) => {
    host.calls.push({ method: 'IpfixState', request: call.request });
    if (host.failAllWith !== undefined)
      return cb({ code: host.failAllWith, details: 'fake failure' });
    if (call.request.owner !== '' && call.request.owner !== host.owner) {
      return cb({ code: status.PERMISSION_DENIED, details: `owner ${call.request.owner}` });
    }
    const ix = obj(obj(host.current['services'])['ipfix']);
    const exporters: IpfixExporterState[] = [];
    let first = true;
    for (const name of Object.keys(obj(ix['exporters'])).sort()) {
      const e = obj(obj(ix['exporters'])[name]);
      if (!bool(e['enabled'], true)) continue;
      const c = obj(e['collector']);
      const address = typeof c['address'] === 'string' ? c['address'] : '';
      const def = first && !address.includes(':');
      if (def) first = false;
      exporters.push({
        name,
        defaultExporter: def,
        collector: address,
        collectorPort: num(c['port'], 4739),
        sourceAddress: typeof e['sourceAddress'] === 'string' ? e['sourceAddress'] : '',
        vrf: typeof e['vrf'] === 'string' ? e['vrf'] : 'default',
        pathMtu: num(e['pathMtu'], 512),
        templateIntervalSec: num(e['templateIntervalSec'], 20),
        udpChecksum: bool(e['udpChecksum'], false),
        statIndex: def ? undefined : 40 + exporters.length,
      });
    }
    exporters.sort((a, b) => Number(b.defaultExporter) - Number(a.defaultExporter));
    const fp = obj(ix['flowprobe']);
    const fpIfs = ((fp['interfaces'] as Json[] | undefined) ?? []).map((f) => ({
      interface: String(f['interface'] ?? ''),
      which: bool(f['ip4'], true) ? 'ip4' : bool(f['ip6'], false) ? 'ip6' : 'l2',
      direction: typeof f['direction'] === 'string' ? f['direction'] : 'both',
    }));
    const sf = obj(ix['sflow']);
    const sfOn = bool(sf['enabled'], false);
    const sfIfs = sfOn ? ((sf['interfaces'] as string[] | undefined) ?? []) : [];
    cb(null, {
      exporters,
      flowprobeParams:
        fpIfs.length > 0
          ? {
              recordL2: bool(fp['recordL2'], false),
              recordL3: bool(fp['recordL3'], true),
              recordL4: bool(fp['recordL4'], true),
              activeTimerSec: num(fp['activeTimerSec'], 15),
              passiveTimerSec: num(fp['passiveTimerSec'], 120),
            }
          : undefined,
      flowprobeInterfaces: fpIfs.sort((a, b) => a.interface.localeCompare(b.interface)),
      sflowGlobal: {
        samplingN: sfOn ? num(sf['samplingN'], 10000) : 10000,
        pollingIntervalSec: sfOn ? num(sf['pollingIntervalSec'], 20) : 20,
        headerBytes: sfOn ? num(sf['headerBytes'], 128) : 128,
        direction: 'rx',
        dropMonitoring: false,
      },
      sflowInterfaces: [...sfIfs].sort().map((i, k) => ({ interface: i, hwIfIndex: 100 + k })),
      sflowCounters:
        sfIfs.length > 0 ? [{ name: '/err/sflow/sflow packets processed', value: '1000' }] : [],
      globalsOwner: true,
      notes:
        sfIfs.length > 0
          ? ['VPP samples on the sFlow interfaces; export to collectors needs hsflowd']
          : [],
      owner: host.owner,
      retrievedAt: new Date(),
    });
  };
}
