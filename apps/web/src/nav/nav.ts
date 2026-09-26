import type { RootKey } from '@ngfw/schema';
import { DEV_ROUTES } from '../build-flags';
import type { DomainInfo } from '../schema/registry';

/** Left-navigation groups (docs/05-ui-spec.md screen inventory), in display order. */
export const NAV_GROUPS = ['dashboard', 'interfaces', 'routing', 'firewall', 'vpn', 'services', 'system', 'tools', 'dev'] as const;
export type NavGroupId = (typeof NAV_GROUPS)[number];

/**
 * Which group each schema domain (root key) belongs to. Navigation entries themselves come from the schema's
 * top-level keys (vdom.md guardrail 4); this table only places them. A new root key without a mapping lands
 * in `system` (and `nav.test.ts` reminds you to place it).
 */
export const DOMAIN_GROUP: Record<RootKey, NavGroupId> = {
  system: 'system',
  dataplane: 'system',
  interfaces: 'interfaces',
  vrfs: 'routing',
  routing: 'routing',
  nat: 'firewall',
  objects: 'firewall',
  acl: 'firewall',
  vpn: 'vpn',
  tunnels: 'vpn',
  services: 'services',
  ha: 'system',
  management: 'system',
};

export interface NavItem {
  id: string;
  path: string;
  /** i18n key (`nav:` namespace) for the label. */
  labelKey: string;
  /** Fallback label when the key is missing (schema title). */
  fallbackLabel: string;
  /** Schema domain backing this entry, when it is one. */
  domain?: RootKey;
  /** `false` → the route renders the "not yet available" page. Never fake data. */
  available: boolean;
}

export interface NavGroup {
  id: NavGroupId;
  labelKey: string;
  items: NavItem[];
}

/** Domains whose screen is built (the rest render "not yet available"); P08 adds interfaces. */
export const BUILT_DOMAINS: ReadonlySet<RootKey> = new Set<RootKey>([
  'interfaces',
  // Feature domains: one line under the feature's anchor (wave-A-hotspots W2).
  'services', // F-snmp (unanchored: no wave-BC: F-snmp anchor in BUILT_DOMAINS)
  // wave-BC: F-tunnels
  // wave-BC: F-vrrp-config-sync
  // wave-BC: F-srv6
  // wave-BC: F-lisp
  'vpn',
  // wave-A: F-vrf-static-ecmp
  'vrfs',
  'routing',
  // wave-A: F-object-model
  'objects',
  // wave-A: F-acl
  'acl',
  // wave-A: F-nat44-ed-sessions
  // wave-A: P11
  // wave-A: F-wireguard
  // wave-A: F-kea-dhcp-relay
  // wave-A: F-unbound-chrony-syslog
  // wave-BC: F-ipfix-sflow (unanchored)
  // F-qos-flat (unanchored)
  'services',
]);

export function domainPath(key: RootKey): string {
  const group = DOMAIN_GROUP[key] ?? 'system';
  return group === key ? `/${key}` : `/${group}/${key}`;
}

/**
 * Developer demo entries. `DEV_ROUTES` is a build-time literal, so in a production build these are empty and the
 * strings are not in the bundle (scripts/check-no-dev-routes.mjs).
 */
const DEV_NAV_ITEMS: NavItem[] = DEV_ROUTES
  ? [
      { id: 'dev-schema-form', path: '/dev/schema-form', labelKey: 'dev:nav.schemaForm', fallbackLabel: 'SchemaForm demo', available: true },
      { id: 'dev-data-grid', path: '/dev/data-grid', labelKey: 'dev:nav.dataGrid', fallbackLabel: 'DataGrid demo', available: true },
      { id: 'dev-stream', path: '/dev/stream', labelKey: 'dev:nav.stream', fallbackLabel: 'Stream demo', available: true },
    ]
  : [];
const DEV_GROUP_LABEL = DEV_ROUTES ? 'dev:nav.group' : '';

/**
 * Build the navigation from the schema domains plus the fixed non-schema screens. The "Developer" group exists only when
 * `devRoutes` is on (defaults to the build flag; a production build has no entries to add either way).
 */
export function buildNav(domains: readonly DomainInfo[], { devRoutes = DEV_ROUTES }: { devRoutes?: boolean } = {}): NavGroup[] {
  const groups = new Map<NavGroupId, NavItem[]>(NAV_GROUPS.map((g) => [g, []]));
  groups.get('dashboard')!.push({ id: 'dashboard', path: '/', labelKey: 'nav:dashboard', fallbackLabel: 'Dashboard', available: true });
  for (const d of domains) {
    const group = DOMAIN_GROUP[d.key] ?? 'system';
    groups.get(group)!.push({
      id: d.key,
      path: domainPath(d.key),
      labelKey: `nav:domains.${d.key}`,
      fallbackLabel: d.title,
      domain: d.key,
      available: BUILT_DOMAINS.has(d.key),
    });
  }
  // Feature screens that are not a schema domain: one `groups.get('<group>')!.push({…})` line under the feature's anchor, labelKey in
  // the feature's namespace (wave-A-hotspots W2).
  // wave-BC: F-ospf
  // wave-BC: F-isis-rip
  // wave-BC: F-bfd-redistribution
  // wave-BC: F-mpls-srmpls
  // wave-BC: F-igmp-mfib
  // wave-BC: F-capture-trace
  // wave-A: F-bonding
  groups.get('interfaces')!.push({ id: 'bonds', path: '/interfaces/bonds', labelKey: 'bonding:nav', fallbackLabel: 'Bonds', available: true });
  // wave-A: F-bridge-l2
  groups.get('interfaces')!.push({ id: 'bridging', path: '/interfaces/bridging', labelKey: 'bridge-l2:nav.bridging', fallbackLabel: 'Bridging', available: true });
  // wave-A: F-loopback-bvi-gso-lldp-span
  groups.get('interfaces')!.push({ id: 'lldp', path: '/interfaces/lldp', labelKey: 'loopback-bvi-gso-lldp-span:nav.lldp', fallbackLabel: 'LLDP', available: true }, { id: 'mirroring', path: '/interfaces/mirroring', labelKey: 'loopback-bvi-gso-lldp-span:nav.mirroring', fallbackLabel: 'Port mirroring', available: true });
  groups.get('tools')!.push({ id: 'nsim', path: '/tools/nsim', labelKey: 'loopback-bvi-gso-lldp-span:nav.nsim', fallbackLabel: 'Delay simulator (lab)', available: true });
  // wave-A: F-neighbors-ra
  groups.get('routing')!.push({ id: 'neighbors', path: '/routing/neighbors', labelKey: 'neighbors-ra:nav', fallbackLabel: 'Neighbours', available: true });
  // wave-A: F-rpf-adl-pbr
  groups.get('routing')!.push({ id: 'pbr', path: '/routing/pbr', labelKey: 'rpf-adl-pbr:nav.pbr', fallbackLabel: 'Policy routing', available: true });
  groups.get('firewall')!.push({ id: 'adl', path: '/firewall/adl', labelKey: 'rpf-adl-pbr:nav.adl', fallbackLabel: 'ADL / Auto-SDL', available: true });
  // wave-A: F-host-acl-nftables
  // wave-A: P12
  // web: WEB-2
  groups.get('system')!.push(
    { id: 'users', path: '/system/users', labelKey: 'nav:users', fallbackLabel: 'Users', available: true },
    { id: 'revisions', path: '/system/revisions', labelKey: 'nav:revisions', fallbackLabel: 'Revisions', available: true },
    // Non-domain system items, one per S5 task (wave-BC-numbers.md S5 pack):
    // wave-BC: F-restconf-yang
    // wave-BC: F-aaa
    // wave-BC: F-backup-restore
    // wave-BC: P10
    // wave-BC: P14
    // wave-BC: F-ab-upgrade
    // wave-BC: F-images
    // wave-BC: F-hardening-lite
    // wave-BC: F-licensing (unanchored)
    { id: 'licensing', path: '/system/licensing', labelKey: 'licensing:nav', fallbackLabel: 'Licence', available: true },
  );
  groups.get('tools')!.push({ id: 'tools', path: '/tools', labelKey: 'nav:tools', fallbackLabel: 'Tools', available: false });
  if (devRoutes) groups.get('dev')!.push(...DEV_NAV_ITEMS);
  return NAV_GROUPS.map((id) => ({ id, labelKey: id === 'dev' ? DEV_GROUP_LABEL : `nav:groups.${id}`, items: groups.get(id)! })).filter((g) => g.items.length > 0);
}

/** Groups with more than one entry collapse under a header; single-entry groups render as a plain link. */
export function isCollapsible(group: NavGroup): boolean {
  return group.items.length > 1;
}

/** The single nav path to mark current for `pathname`: the longest item path equal to it or a parent of it. */
export function currentNavPath(nav: readonly NavGroup[], pathname: string): string | undefined {
  let best: string | undefined;
  for (const item of nav.flatMap((g) => g.items)) {
    const hit = item.path === '/' ? pathname === '/' : pathname === item.path || pathname.startsWith(`${item.path}/`);
    if (hit && (best === undefined || item.path.length > best.length)) best = item.path;
  }
  return best;
}
