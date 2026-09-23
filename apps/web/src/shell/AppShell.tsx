import MenuIcon from '@mui/icons-material/Menu';
import SettingsIcon from '@mui/icons-material/Settings';
import AppBar from '@mui/material/AppBar';
import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import Divider from '@mui/material/Divider';
import Drawer from '@mui/material/Drawer';
import IconButton from '@mui/material/IconButton';
import LinearProgress from '@mui/material/LinearProgress';
import List from '@mui/material/List';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemText from '@mui/material/ListItemText';
import ListSubheader from '@mui/material/ListSubheader';
import Toolbar from '@mui/material/Toolbar';
import Typography from '@mui/material/Typography';
import { UI_KIT_NS } from '@ngfw/ui-kit';
import { useWsStatus } from '@ngfw/ui-kit/ws';
import { Suspense, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { NavLink, Outlet, useLocation } from 'react-router';
import { DEV_ROUTES } from '../build-flags';
import { buildNav } from '../nav/nav';
import { domains } from '../schema/registry';
import { SettingsPopover } from './SettingsPopover';

const DRAWER_WIDTH = 264;

function NavList({ onNavigate, devRoutes }: { onNavigate: () => void; devRoutes: boolean }) {
  const { t } = useTranslation(['common', 'nav']);
  const nav = useMemo(() => buildNav(domains, { devRoutes }), [devRoutes]);
  const { pathname } = useLocation();
  return (
    <List component="nav" aria-label={t('menu.navigation')} dense sx={{ pt: 0 }}>
      {nav.map((group) => (
        <Box key={group.id} component="li" sx={{ listStyle: 'none' }}>
          <ListSubheader component="div" disableSticky>
            {t(group.labelKey)}
          </ListSubheader>
          <List component="ul" dense disablePadding>
            {group.items.map((item) => (
              <ListItemButton
                key={item.id}
                component={NavLink}
                to={item.path}
                end={item.path === '/'}
                selected={item.path === '/' ? pathname === '/' : pathname === item.path || pathname.startsWith(`${item.path}/`)}
                onClick={onNavigate}
                sx={{ paddingInlineStart: (theme) => theme.spacing(3) }}
              >
                <ListItemText primary={t(item.labelKey, { defaultValue: item.fallbackLabel })} />
                {!item.available && <Chip size="small" variant="outlined" label={t('nav:soon')} />}
              </ListItemButton>
            ))}
          </List>
        </Box>
      ))}
    </List>
  );
}

/** Application frame: fixed AppBar, grouped left navigation (from the schema's root keys), routed main area. */
export function AppShell({ devRoutes = DEV_ROUTES }: { devRoutes?: boolean }) {
  const { t } = useTranslation(['common', UI_KIT_NS]);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [settingsAnchor, setSettingsAnchor] = useState<HTMLElement | null>(null);
  const wsStatus = useWsStatus();
  const closeMobile = () => setMobileOpen(false);

  const drawerContent = (
    <>
      <Toolbar />
      <Divider />
      <NavList onNavigate={closeMobile} devRoutes={devRoutes} />
    </>
  );

  return (
    <Box sx={{ display: 'flex', minBlockSize: '100vh' }}>
      <Box
        component="a"
        href="#main"
        sx={{
          position: 'absolute',
          insetInlineStart: 8,
          top: -200,
          zIndex: (theme) => theme.zIndex.tooltip,
          bgcolor: 'background.paper',
          color: 'text.primary',
          p: 1,
          borderRadius: 1,
          '&:focus': { top: 8 },
        }}
      >
        {t('skipToContent')}
      </Box>
      <AppBar position="fixed" sx={{ zIndex: (theme) => theme.zIndex.drawer + 1 }}>
        <Toolbar>
          <IconButton color="inherit" edge="start" aria-label={t('menu.open')} onClick={() => setMobileOpen(true)} sx={{ display: { md: 'none' }, marginInlineEnd: 1 }}>
            <MenuIcon />
          </IconButton>
          <Typography component="h1" variant="h6" sx={{ flexGrow: 1 }}>
            {t('appName')}
          </Typography>
          <Chip
            size="small"
            variant="outlined"
            color={wsStatus === 'open' ? 'success' : 'default'}
            label={t(`ws.${wsStatus}`, { ns: UI_KIT_NS })}
            aria-label={t('stream.label')}
            sx={{ color: 'inherit', borderColor: 'currentColor', marginInlineEnd: 1 }}
          />
          <IconButton color="inherit" edge="end" aria-label={t('menu.settings')} onClick={(e) => setSettingsAnchor(e.currentTarget)}>
            <SettingsIcon />
          </IconButton>
          <SettingsPopover anchorEl={settingsAnchor} onClose={() => setSettingsAnchor(null)} />
        </Toolbar>
      </AppBar>
      {mobileOpen && (
        <Drawer variant="temporary" open onClose={closeMobile} sx={{ '& .MuiDrawer-paper': { inlineSize: DRAWER_WIDTH } }}>
          {drawerContent}
        </Drawer>
      )}
      <Drawer
        variant="permanent"
        open
        sx={{ display: { xs: 'none', md: 'block' }, inlineSize: DRAWER_WIDTH, flexShrink: 0, '& .MuiDrawer-paper': { inlineSize: DRAWER_WIDTH, boxSizing: 'border-box' } }}
      >
        {drawerContent}
      </Drawer>
      <Box component="main" id="main" tabIndex={-1} sx={{ flexGrow: 1, minInlineSize: 0, p: 3, outline: 'none' }}>
        <Toolbar />
        <Suspense fallback={<LinearProgress aria-label={t('loading')} />}>
          <Outlet />
        </Suspense>
      </Box>
    </Box>
  );
}
