import type { RootKey } from '@ngfw/schema';
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

export function domainPath(key: RootKey): string {
  const group = DOMAIN_GROUP[key] ?? 'system';
  return group === key ? `/${key}` : `/${group}/${key}`;
}

/** Build the navigation from the schema domains plus the fixed non-schema screens. */
export function buildNav(domains: readonly DomainInfo[]): NavGroup[] {
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
      available: false,
    });
  }
  groups.get('system')!.push(
    { id: 'users', path: '/system/users', labelKey: 'nav:users', fallbackLabel: 'Users', available: false },
    { id: 'revisions', path: '/system/revisions', labelKey: 'nav:revisions', fallbackLabel: 'Revisions', available: false },
  );
  groups.get('tools')!.push({ id: 'tools', path: '/tools', labelKey: 'nav:tools', fallbackLabel: 'Tools', available: false });
  groups.get('dev')!.push(
    { id: 'dev-schema-form', path: '/dev/schema-form', labelKey: 'nav:devSchemaForm', fallbackLabel: 'SchemaForm demo', available: true },
    { id: 'dev-data-grid', path: '/dev/data-grid', labelKey: 'nav:devDataGrid', fallbackLabel: 'DataGrid demo', available: true },
    { id: 'dev-stream', path: '/dev/stream', labelKey: 'nav:devStream', fallbackLabel: 'Stream demo', available: true },
  );
  return NAV_GROUPS.map((id) => ({ id, labelKey: `nav:groups.${id}`, items: groups.get(id)! })).filter((g) => g.items.length > 0);
}
