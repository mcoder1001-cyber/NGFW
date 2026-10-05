import { createBrowserRouter, createMemoryRouter, Navigate, type RouteObject } from 'react-router';
import { DEV_ROUTES } from './build-flags';
import { BUILT_DOMAINS, domainPath } from './nav/nav';
import { domains } from './schema/registry';
import { RequireAuth } from './auth/RequireAuth';
import { LoginPage } from './pages/LoginPage';
import { AppShell } from './shell/AppShell';
import { NotFoundPage } from './shell/NotFoundPage';
import { RouteErrorPage } from './shell/RouteErrorPage';

/**
 * Developer demos (code-split). `DEV_ROUTES` is a build-time literal: in a production build this list is empty and the
 * demo chunks are not emitted at all (scripts/check-no-dev-routes.mjs).
 */
const DEV_ROUTE_OBJECTS: RouteObject[] = DEV_ROUTES
  ? [
      { path: 'dev/schema-form', lazy: async () => ({ Component: (await import('./pages/dev/SchemaFormDemoPage')).SchemaFormDemoPage }) },
      { path: 'dev/data-grid', lazy: async () => ({ Component: (await import('./pages/dev/DataGridDemoPage')).DataGridDemoPage }) },
      { path: 'dev/stream', lazy: async () => ({ Component: (await import('./pages/dev/StreamDemoPage')).StreamDemoPage }) },
    ]
  : [];

export interface RouteOptions {
  /** Include the `/dev/*` demo routes. Defaults to the build flag (off in production builds). */
  devRoutes?: boolean;
}

/**
 * Domain screens shared by several features (W-seed shells): the page renders the domain placeholder until a feature registers
 * a tab in `domains/<key>/tabs.ts`, so these routes change nothing a user can see.
 */
const SHELL_ROUTES: RouteObject[] = [
  { path: domainPath('vpn').slice(1), lazy: async () => ({ Component: (await import('./domains/vpn/VpnPage')).VpnPage }) },
  { path: domainPath('services').slice(1), lazy: async () => ({ Component: (await import('./domains/services/ServicesPage')).ServicesPage }) },
];
const SHELL_DOMAINS: ReadonlySet<string> = new Set(['vpn', 'services']);

/** Route table: domain routes come from the schema's root keys; dev demos exist only in dev builds. */
export function buildRoutes({ devRoutes = DEV_ROUTES }: RouteOptions = {}): RouteObject[] {
  return [
    { path: '/login', element: <LoginPage />, errorElement: <RouteErrorPage /> },
    {
      path: '/',
      element: (
        <RequireAuth>
          <AppShell devRoutes={devRoutes} />
        </RequireAuth>
      ),
      errorElement: <RouteErrorPage />,
      children: [
        // Screens are code-split per route; feature screens (P08+) plug in the same way.
        { index: true, lazy: async () => ({ Component: (await import('./pages/DashboardPage')).DashboardPage }) },
        // P08: the first built domain screen; the others keep their placeholder until their feature task lands
        { path: domainPath('interfaces').slice(1), lazy: async () => ({ Component: (await import('./domains/interfaces/InterfacesPage')).InterfacesPage }) },
        ...SHELL_ROUTES,
        ...domains.filter((d) => !BUILT_DOMAINS.has(d.key) && !SHELL_DOMAINS.has(d.key)).map((d) => ({
          path: domainPath(d.key).slice(1),
          lazy: async () => {
            const { DomainPlaceholderPage } = await import('./pages/DomainPlaceholderPage');
            return { element: <DomainPlaceholderPage domainKey={d.key} /> };
          },
        })),
        // wave-A: UI-domain-editor — no anchor was seeded for this task by W-seed (see
        // UI-domain-editor-questions.md); the generic advanced editor for any domain path (D-125), reached from
        // each `DomainPlaceholderPage` and usable for a built domain too (`/interfaces` and `/config/interfaces`
        // both work). A splat route ranks below every static path above, so it never shadows them.
        { path: 'config/*', lazy: async () => ({ Component: (await import('./domains/advanced/AdvancedEditorPage')).AdvancedEditorPage }) },
        // Feature screens: one lazy route line under the feature's anchor (wave-A-hotspots W1).
        { path: 'system/setup', lazy: async () => ({ Component: (await import('./domains/system/setup/SetupWizardPage')).SetupWizardPage }) },
        // wave-BC: F-tunnels
        { path: 'vpn/tunnels', lazy: async () => ({ Component: (await import('./domains/vpn/tunnels/TunnelsPage')).TunnelsPage }) },
        // wave-BC: P10
        // wave-BC: F-vrrp-config-sync
        { path: 'system/ha', lazy: async () => ({ Component: (await import('./domains/system/ha/HaPage')).HaPage }) },
        // wave-BC: F-ospf
        { path: 'routing/ospf', lazy: async () => ({ Component: (await import('./domains/routing/ospf/OspfPage')).OspfPage }) },
        // wave-BC: F-isis-rip
        { path: 'routing/isis-rip', lazy: async () => ({ Component: (await import('./domains/routing/isis-rip/IsisRipPage')).IsisRipPage }) },
        // wave-BC: P14
        // wave-BC: F-mpls-srmpls
        { path: 'routing/mpls', lazy: async () => ({ Component: (await import('./domains/routing/mpls-srmpls/MplsPage')).MplsPage }) },
        // wave-BC: F-capture-trace
        { path: 'tools/capture', lazy: async () => ({ Component: (await import('./domains/tools/capture-trace/CapturePage')).CapturePage }) },
        // wave-BC: F-bfd-redistribution
        { path: 'routing/bfd', lazy: async () => ({ Component: (await import('./domains/routing/bfd-redistribution/BfdRedistributionPage')).BfdRedistributionPage }) },
        { path: 'routing/redistribution', lazy: async () => ({ Component: (await import('./domains/routing/bfd-redistribution/BfdRedistributionPage')).RedistributionPage }) },
        { path: 'routing/policy', lazy: async () => ({ Component: (await import('./domains/routing/bfd-redistribution/BfdRedistributionPage')).PolicyUsagePage }) },
        // wave-BC: F-igmp-mfib
        { path: 'routing/multicast', lazy: async () => ({ Component: (await import('./domains/routing/igmp-mfib/MulticastPage')).MulticastPage }) },
        // wave-BC: F-hardening-lite
        // wave-BC: F-aaa
        { path: 'system/aaa', lazy: async () => ({ Component: (await import('./domains/system/aaa/AaaPage')).AaaPage }) },
        // wave-BC: F-restconf-yang
        { path: 'system/restconf', lazy: async () => ({ Component: (await import('./domains/system/restconf-yang/RestconfYangPage')).RestconfYangPage }) },
        // wave-BC: F-ab-upgrade
        // wave-BC: F-images
        // wave-BC: F-backup-restore
        // web: WEB-2
        // wave-A: UI-domain-editor
        // wave-A: F-bonding
        { path: 'interfaces/bonds', lazy: async () => ({ Component: (await import('./domains/interfaces/bonding/BondsPage')).BondsPage }) },
        // wave-A: F-bridge-l2
        { path: 'interfaces/bridging', lazy: async () => ({ Component: (await import('./domains/interfaces/bridge-l2/BridgingPage')).BridgingPage }) },
        // wave-A: F-loopback-bvi-gso-lldp-span
        { path: 'interfaces/lldp', lazy: async () => ({ Component: (await import('./domains/interfaces/loopback-bvi-gso-lldp-span/LldpPage')).LldpPage }) },
        { path: 'interfaces/mirroring', lazy: async () => ({ Component: (await import('./domains/interfaces/loopback-bvi-gso-lldp-span/MirroringPage')).MirroringPage }) },
        { path: 'tools/nsim', lazy: async () => ({ Component: (await import('./domains/interfaces/loopback-bvi-gso-lldp-span/NsimPage')).NsimPage }) },
        // wave-A: F-vrf-static-ecmp
        { path: domainPath('vrfs').slice(1), lazy: async () => ({ Component: (await import('./domains/routing/vrf-static-ecmp/VrfsPage')).VrfsPage }) },
        { path: domainPath('routing').slice(1), lazy: async () => ({ Component: (await import('./domains/routing/vrf-static-ecmp/RoutingPage')).RoutingPage }) },
        // wave-A: F-neighbors-ra
        { path: 'routing/neighbors', lazy: async () => ({ Component: (await import('./domains/routing/neighbors-ra/NeighborsPage')).NeighborsPage }) },
        // wave-A: F-rpf-adl-pbr
        { path: 'routing/pbr', lazy: async () => ({ Component: (await import('./domains/routing/rpf-adl-pbr/PbrPage')).PbrPage }) },
        { path: 'firewall/adl', lazy: async () => ({ Component: (await import('./domains/routing/rpf-adl-pbr/AdlPage')).AdlPage }) },
        // wave-A: F-object-model
        { path: domainPath('objects').slice(1), lazy: async () => ({ Component: (await import('./domains/firewall/object-model/ObjectsPage')).ObjectsPage }) },
        // wave-A: F-acl
        { path: domainPath('acl').slice(1), lazy: async () => ({ Component: (await import('./domains/firewall/acl/AclPage')).AclPage }) },
        // wave-A: F-host-acl-nftables
        { path: 'firewall/host-acl', lazy: async () => ({ Component: (await import('./domains/firewall/host-acl-nftables/HostAclPage')).HostAclPage }) },
        // F-global-blocking (unanchored)
        { path: 'firewall/global-blocking', lazy: async () => ({ Component: (await import('./domains/security/global-blocking/GlobalBlockingPage')).GlobalBlockingPage }) },
        // F-bruteforce-block (unanchored)
        { path: 'firewall/auto-block', lazy: async () => ({ Component: (await import('./domains/security/auto-block/AutoBlockPage')).AutoBlockPage }) },
        // wave-BC: F-dashboard-prom-alarms
        { path: 'system/alarms', lazy: async () => ({ Component: (await import('./domains/dashboard/dashboard-prom-alarms/AlarmsPage')).AlarmsPage }) },
        // F-multiwan (unanchored)
        { path: 'routing/wan', lazy: async () => ({ Component: (await import('./domains/routing/multiwan/WanGroupsPage')).WanGroupsPage }) },
        // wave-A: F-nat44-ed-sessions
        { path: domainPath('nat').slice(1), lazy: async () => ({ Component: (await import('./domains/firewall/nat44-ed-sessions/NatPage')).NatPage }) },
        // wave-A: P11
        // wave-A: F-wireguard
        // wave-A: P12
        { path: 'routing/bgp', lazy: async () => ({ Component: (await import('./domains/routing/bgp/BgpPage')).BgpPage }) },
        // wave-A: F-kea-dhcp-relay
        // wave-A: F-unbound-chrony-syslog
        // F-management-ui (unanchored): users live in the Management page's Users tab; the old address redirects there
        { path: 'system/users', element: <Navigate to={`${domainPath('management')}?tab=users`} replace /> },
        { path: domainPath('management').slice(1), lazy: async () => ({ Component: (await import('./domains/system/management/ManagementPage')).ManagementPage }) },
        // F-dataplane-ui (unanchored)
        { path: domainPath('dataplane').slice(1), lazy: async () => ({ Component: (await import('./domains/system/dataplane/DataplanePage')).DataplanePage }) },
        // F-system-identity (unanchored)
        { path: domainPath('system').slice(1), lazy: async () => ({ Component: (await import('./domains/system/identity/SystemIdentityPage')).SystemIdentityPage }) },
        // wave-BC: F-licensing (unanchored)
        { path: 'system/licensing', lazy: async () => ({ Component: (await import('./domains/system/licensing/LicensingPage')).LicensingPage }) },
        { path: 'system/revisions', lazy: async () => ({ Component: (await import('./pages/RevisionsPage')).RevisionsPage }) },
        { path: 'tools', element: <Navigate to="/tools/capture" replace /> }, // F-capture-trace
        ...(devRoutes ? DEV_ROUTE_OBJECTS : []),
        { path: '*', element: <NotFoundPage /> },
      ],
    },
  ];
}

export function createAppRouter() {
  return createBrowserRouter(buildRoutes());
}

export function createTestRouter(initialEntries: string[], options?: RouteOptions) {
  return createMemoryRouter(buildRoutes(options), { initialEntries });
}
