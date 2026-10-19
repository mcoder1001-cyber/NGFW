/** Synthetic fixtures for the explicitly enabled, development-only offline UI preview.
 * They are not observations of a router. Unknown routes must remain unavailable.
 * This module performs no network requests and is not imported by production entry points.
 */
import { RootConfig } from '@ngfw/schema';

export const PREVIEW_TIMESTAMP = '2026-10-09T08:00:00.000Z';

/** Schema defaults keep every feature's configuration shape valid. */
export const previewConfig = RootConfig.parse({
  system: { hostname: 'preview-edge', timezone: 'Asia/Tehran', setup: { completed: true, completedAt: PREVIEW_TIMESTAMP } },
  interfaces: {
    'GigabitEthernet0/0/0': { enabled: true, description: 'Preview WAN uplink', mtu: 1500, ipv4: ['192.0.2.2/24'], ipv6: [], vrf: 'default' },
    'GigabitEthernet0/0/1': { enabled: true, description: 'Preview office LAN', mtu: 1500, ipv4: ['10.10.0.1/24'], ipv6: [], vrf: 'default', subinterfaces: { '20': { enabled: true, vlanId: 20, ipv4: ['10.20.0.1/24'], ipv6: [], vrf: 'guest' } } },
    loop0: { enabled: false, description: 'Preview management loopback', ipv4: ['198.51.100.1/32'], ipv6: [], vrf: 'default' },
  },
  vrfs: { default: { id: 0, description: 'Default routing table' }, guest: { id: 20, description: 'Guest network' } },
  routing: { static: [{ prefix: '0.0.0.0/0', vrf: 'default', nextHops: [{ address: '192.0.2.1', interface: 'GigabitEthernet0/0/0' }], description: 'Preview upstream gateway' }] },
  objects: { addresses: { 'office-network': { type: 'network', prefix: '10.10.0.0/24', description: 'Preview office subnet' }, 'guest-network': { type: 'network', prefix: '10.20.0.0/24', description: 'Preview guest subnet' } } },
});

const interfaceItems = Object.entries(previewConfig.interfaces).map(([name, config], index) => ({
  name, kind: 'interface', parent: null,
  state: {
    name, vppName: name, swIfIndex: index + 1, type: name === 'loop0' ? 'loopback' : 'ethernet',
    adminUp: config.enabled, linkUp: config.enabled, mtu: config.mtu ?? 1500, linkMtu: config.mtu ?? 1500,
    mac: `02:00:00:00:00:0${index + 1}`, ipv4: config.ipv4, ipv6: config.ipv6, vrf: config.vrf,
    tableId: 0, parent: '', vlanId: 0, innerVlanId: 0, managed: true,
    linkSpeedKbps: name === 'loop0' ? '0' : '1000000', rxMode: 'interrupt', description: config.description ?? '',
  },
  config, running: config, counters: null, hasPendingChange: false,
}));

const emptyPage = { page: 1, pageSize: 25, items: [], total: 0, retrievedAt: PREVIEW_TIMESTAMP };

/** Explicit GET route bodies only. No catch-all success response: unsupported data is unavailable. */
export const previewStateFixtures: Record<string, unknown> = {
  '/api/v1/state/interfaces': { items: interfaceItems, observationErrors: [] },
  '/api/v1/state/interfaces/bonds': { retrievedAt: PREVIEW_TIMESTAMP, live: true, items: [] },
  '/api/v1/state/l2/bridge-domains': { retrievedAt: PREVIEW_TIMESTAMP, items: [] },
  '/api/v1/state/lldp/neighbors': { ...emptyPage, eventsActive: false },
  '/api/v1/state/neighbors': {
    ...emptyPage, total: 2,
    items: [
      { interface: 'GigabitEthernet0/0/0', swIfIndex: 1, ip: '192.0.2.1', mac: '02:00:00:00:01:01', family: 'ipv4', vrf: 'default', tableId: 0, state: 'dynamic', ageSec: 30, noFibEntry: false },
      { interface: 'GigabitEthernet0/0/1', swIfIndex: 2, ip: '10.10.0.25', mac: '02:00:00:00:10:25', family: 'ipv4', vrf: 'default', tableId: 0, state: 'dynamic', ageSec: 12, noFibEntry: false },
    ],
  },
  '/api/v1/state/system': {
    api: { version: 'offline-preview', startedAt: PREVIEW_TIMESTAMP, wsClients: 0 },
    agent: { reachable: true, vppConnected: true, vppVersion: 'preview', degraded: false },
    runningRevision: 7, pendingCommit: null,
    sync: { state: 'in-sync', reason: '', txnId: null, since: PREVIEW_TIMESTAMP },
  },
  '/api/v1/state/host': {
    hostname: 'preview-edge', uptimeSec: 273600,
    cpu: { cores: 8, model: 'Preview CPU', usagePct: 24, load: [0.42, 0.38, 0.31] },
    memory: { totalBytes: 16 * 2 ** 30, usedBytes: 4 * 2 ** 30, availableBytes: 12 * 2 ** 30 },
    hugepages: { total: 512, free: 384, sizeBytes: 2 ** 21 },
    disks: [{ mount: '/', totalBytes: 100 * 2 ** 30, usedBytes: 28 * 2 ** 30 }],
    history: [{ at: 1791532740000, cpuPct: 20, memUsedPct: 24 }, { at: 1791532800000, cpuPct: 24, memUsedPct: 25 }],
    sampledAt: PREVIEW_TIMESTAMP,
  },
  '/api/v1/state/events': { total: 2, items: [
    { id: 2, ts: PREVIEW_TIMESTAMP, severity: 'info', subsystem: 'preview', code: 'preview-ready', message: 'Offline preview sample loaded', data: null },
    { id: 1, ts: PREVIEW_TIMESTAMP, severity: 'warning', subsystem: 'preview', code: 'sample-only', message: 'All displayed values are synthetic sample data', data: null },
  ] },
  '/api/v1/state/drift': { subsystems: [], changes: [], ignored: [] },
  '/api/v1/state/license': { status: 'community', daysLeft: 0, graceDays: 30, bound: { machineId: false, serial: false }, entitlements: { features: [], limits: {} } },
  '/api/v1/state/dataplane': {
    runtimeThreads: [{ id: 0, name: 'preview_main', type: 'main', cpuId: 0, core: 0, numaSocket: 0 }],
    loadedPlugins: 'Preview only', nicQueues: 'Preview only', runtimeMemory: 'Preview only', runtimeErrors: [],
    startupPath: '/preview/startup.conf', startupPresent: true, workers: 2, corelistWorkers: '', mainCore: 0,
    plugins: {}, onlineCpus: '0-7', hugepagesTotalBytes: '2147483648', hugepagesFreeBytes: '1610612736', error: '', retrievedAt: PREVIEW_TIMESTAMP,
  },
  '/api/v1/state/vpn/wireguard': { retrievedAt: PREVIEW_TIMESTAMP, eventsActive: false, interfaces: [] },
  '/api/v1/state/ipsec/tunnels': { retrievedAt: PREVIEW_TIMESTAMP, eventsActive: false, charonRestarted: false, daemonVersion: 'preview', pendingAction: '', tunnels: [] },
  '/api/v1/state/ipsec/sas': { retrievedAt: PREVIEW_TIMESTAMP, items: [] },
  '/api/v1/state/tunnels': { retrievedAt: PREVIEW_TIMESTAMP, items: [] },
  '/api/v1/state/pki': { generatedAt: PREVIEW_TIMESTAMP, cas: [], certificates: [], agentFiles: { root: '/preview/pki', unavailable: null, files: [] }, unsupported: [] },
  '/api/v1/state/dhcp/leases': { ...emptyPage, truncated: false, servers: [] },
  '/api/v1/state/dhcp/relays': { items: [] },
  '/api/v1/state/logs': { ...emptyPage, scanned: 0, truncated: false },
  '/api/v1/state/captures': { items: [] },
  '/api/v1/state/routes': { ...emptyPage, total: 1, items: [{ vrf: 'default', prefix: '0.0.0.0/0', origin: 'static', source: 'api', nextHops: [{ address: '192.0.2.1', interface: 'GigabitEthernet0/0/0' }], distance: 1, paths: [{ type: 'normal', nextHop: '192.0.2.1', interface: 'GigabitEthernet0/0/0', tableId: 0, weight: 1, preference: 0, flags: [] }], statsIndex: 0 }] },
  '/api/v1/state/bgp': { frrRunning: true, frrVersion: 'preview', retrievedAt: PREVIEW_TIMESTAMP, instances: [], ribCounts: {}, lcpPairs: [] },
  '/api/v1/state/acl/lists': { countersAvailable: false, countersReason: 'Offline preview', agentError: null, retrievedAt: PREVIEW_TIMESTAMP, lists: [], macipLists: [] },
  '/api/v1/state/acl/attachments': { retrievedAt: PREVIEW_TIMESTAMP, interfaces: [], agentError: null },
  '/api/v1/state/alarms': { total: 0, items: [] },
  '/api/v1/state/dns': { running: true, configPath: '/preview/unbound.conf', error: '', pendingActions: [], stats: { 'total.num.queries': '1524', 'total.num.cachehits': '1298', 'total.num.cachemiss': '226' }, forwards: [], localZones: [], localData: [], vppCache: null },
  '/api/v1/state/ntp': { running: true, configPath: '/preview/chrony.conf', error: '', sources: [], tracking: null, pendingActions: [] },
  '/api/v1/state/syslog': { running: true, configPath: '/preview/rsyslog.conf', targets: [], pendingActions: [] },
  '/api/v1/state/vpn/remote-access/capabilities': { engine: 'strongswan-ra', operational: false, supportedAuth: ['eap-tls'], reason: 'Offline preview', editableDisabledDrafts: true },
  '/api/v1/state/vpn/remote-access/sessions': { items: [], nextCursor: '' },
};

export const PREVIEW_CONFIG = previewConfig;
export const PREVIEW_STATE_FIXTURES = previewStateFixtures;

export const previewState = previewStateFixtures;
