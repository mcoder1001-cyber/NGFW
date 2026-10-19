import { PgSettingsIcon as SettingsIcon, PgDarkModeIcon as DarkModeIcon, PgLightModeIcon as LightModeIcon, PgDashboardIcon as DashboardIcon, PgRouterIcon as RouterIcon, PgShieldIcon as ShieldIcon, PgLanIcon as LanIcon, PgVpnKeyIcon as VpnKeyIcon, PgBuildIcon as BuildIcon } from '@amnafzar/common-ui-lib/icons';
import { PgBox as Box, PgChip as Chip, PgDrawer as Drawer, PgLinearProgress as LinearProgress } from '@amnafzar/common-ui-lib/primitives';
import { useMediaQuery as useMediaQuery, useTheme } from '@amnafzar/common-ui-lib/styles';
import { PgGlobalAppBar, PgBreadcrumb, PgIconButton, Sidenav } from '@amnafzar/common-ui-lib/shell';
import { UI_KIT_NS } from '@ngfw/ui-kit';
import { useWsStatus } from '@ngfw/ui-kit/ws';
import { lazy, Suspense, useEffect, useMemo, useState, type ComponentProps } from 'react';
import { useTranslation } from 'react-i18next';
import { Outlet, useLocation } from 'react-router';
import { DEV_ROUTES } from '../build-flags';
import { buildNav, currentNavPath, isCollapsible } from '../nav/nav';
import { domains } from '../schema/registry';
import { ConfirmBanner } from '../config/ConfirmBanner';
import { PendingChangeBar, SyncBanner } from '../config/PendingChangeBar';
import { LicenseBanner } from '../domains/system/licensing/LicenseBanner'; // wave-BC: F-licensing (unanchored)
import { useUiSettings } from '../settings/UiSettings';
import { UserMenu } from './UserMenu';
import '../common-ui.css';

const SettingsPopover = lazy(() => import('./SettingsPopover').then(({ SettingsPopover }) => ({ default: SettingsPopover })));


// Common UI keeps the navigation rail dark in both themes. These category colors
// stay readable there; selection is still indicated by the shared row highlight.
function navigationIcon(Icon: typeof DashboardIcon, color: string) {
  return function NavigationIcon(props: ComponentProps<typeof DashboardIcon>) {
    return <Icon {...props} style={{ ...props.style, color }} />;
  };
}

const icons = {
  dashboard: navigationIcon(DashboardIcon, '#60A5FA'),
  interfaces: navigationIcon(RouterIcon, '#22D3EE'),
  routing: navigationIcon(LanIcon, '#60A5FA'),
  objects: navigationIcon(ShieldIcon, '#94A3B8'),
  'dynamic-routing': navigationIcon(LanIcon, '#60A5FA'),
  'routing-objects': navigationIcon(LanIcon, '#94A3B8'),
  firewall: navigationIcon(ShieldIcon, '#FBBF24'),
  vpn: navigationIcon(VpnKeyIcon, '#A78BFA'),
  services: navigationIcon(RouterIcon, '#34D399'),
  system: navigationIcon(SettingsIcon, '#94A3B8'),
  tools: navigationIcon(BuildIcon, '#FB923C'),
  dev: navigationIcon(BuildIcon, '#FB923C'),
};

/** Common UI frame; route inventory and transactional configuration remain NGFW-owned. */
export function AppShell({ devRoutes = DEV_ROUTES }: { devRoutes?: boolean }) {
  const { t } = useTranslation(['common', 'nav', UI_KIT_NS]);
  const { pathname } = useLocation();
  const theme = useTheme();
  const desktop = useMediaQuery(theme.breakpoints.up('md'));
  const [collapsed, setCollapsed] = useState(false);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [settingsAnchor, setSettingsAnchor] = useState<HTMLElement | null>(null);
  const { resolvedMode, update } = useUiSettings();
  const wsStatus = useWsStatus();
  const nav = useMemo(() => buildNav(domains, { devRoutes }), [devRoutes]);
  const current = currentNavPath(nav, pathname);
  const group = nav.find((g) => g.items.some((item) => item.path === current));
  const page = group?.items.find((item) => item.path === current);
  useEffect(() => {
    setMobileOpen(false);
  }, [pathname]);
  const items = useMemo(() => nav.map((g) => {
    const Icon = icons[g.id];
    const children = g.items.map((item) => ({
      title:
        t(item.labelKey, { defaultValue: item.fallbackLabel }) +
        (item.available ? '' : ` · ${t('nav:soon')}`),
      path: item.path,
      activeOn: item.path,
      current: item.path === current,
      end: true,
    }));
    return isCollapsible(g)
      ? {
          title: t(g.labelKey),
          activeOn: g.items[0]?.path.split('/').slice(0, 2).join('/') ?? '',
          children,
          filledIcon: Icon,
          outlinedIcon: Icon,
        }
      : { ...children[0]!, filledIcon: Icon, outlinedIcon: Icon };
  }), [nav, current, t]);
  const breadcrumb = [
    ...(group && isCollapsible(group) ? [{ title: t(group.labelKey) }] : []),
    { title: page ? t(page.labelKey, { defaultValue: page.fallbackLabel }) : t('appName') },
  ];
  const rail = (mini: boolean) => (
    <Sidenav
      sidenavItems={items}
      loadingModules={false}
      collapsed={mini}
      brand={{ title: t('appName') }}
      labels={{
        searchMenu: t('menu.search'),
        clearSearch: t('menu.clearSearch'),
        loadingMenu: t('loading'),
        navigation: t('menu.navigation'),
      }}
      onExpandRequest={() => setCollapsed(false)}
    />
  );
  const toggleTheme = () => update({ mode: resolvedMode === 'dark' ? 'light' : 'dark' });
  const actions = (
    <>
      <PgIconButton
        aria-label={t('theme.label')}
        onClick={toggleTheme}
      >
        {resolvedMode === 'dark' ? <LightModeIcon /> : <DarkModeIcon />}
      </PgIconButton>
      <Chip
        size="small"
        variant="outlined"
        color={wsStatus === 'open' ? 'success' : 'default'}
        label={t(`ws.${wsStatus}`, { ns: UI_KIT_NS })}
        aria-label={t('stream.label')}
        sx={{ display: { xs: 'none', sm: 'inline-flex' } }}
      />
      <UserMenu />
      <PgIconButton
        aria-label={t('menu.settings')}
        onClick={(e) => setSettingsAnchor(e.currentTarget)}
      >
        <SettingsIcon />
      </PgIconButton>
    </>
  );
  return (
    <Box className="ngfw-common-shell">
      <Box component="a" href="#main" className="ngfw-skip-link">
        {t('skipToContent')}
      </Box>
      {desktop && (
        <Box
          component="aside"
          aria-label={t('menu.navigation')}
          sx={{
            inlineSize: collapsed ? 64 : 250,
            flexShrink: 0,
            transition: 'inline-size 180ms ease',
            height: '100%',
          }}
        >
          {rail(collapsed)}
        </Box>
      )}
      {!desktop && (
        <Drawer
          anchor={theme.direction === 'rtl' ? 'right' : 'left'}
          open={mobileOpen}
          onClose={() => setMobileOpen(false)}
          sx={{ '& .MuiDrawer-paper': { inlineSize: 250 } }}
        >
          {rail(false)}
        </Drawer>
      )}
      <Box className="ngfw-content-shell">
        <PgGlobalAppBar
          menuButton
          menuOpen={desktop ? !collapsed : mobileOpen}
          onMenuClick={() =>
            desktop ? setCollapsed((value) => !value) : setMobileOpen((value) => !value)
          }
          labels={{ menuHide: t('menu.close'), menuShow: t('menu.open') }}
          breadcrumb={<PgBreadcrumb items={breadcrumb} aria-label={t('menu.pageLocation')} />}
          actions={actions}
        />
        {settingsAnchor && (
          <Suspense fallback={null}>
            <SettingsPopover anchorEl={settingsAnchor} onClose={() => setSettingsAnchor(null)} />
          </Suspense>
        )}
        <Box
          component="main"
          id="main"
          tabIndex={-1}
          className="ngfw-page-canvas"
          sx={{ background: theme.palette.gradient.canvas }}
        >
          <ConfirmBanner />
          <SyncBanner />
          <LicenseBanner />
          <PendingChangeBar />
          <Suspense fallback={<LinearProgress aria-label={t('loading')} />}>
            <Outlet />
          </Suspense>
        </Box>
      </Box>
    </Box>
  );
}
