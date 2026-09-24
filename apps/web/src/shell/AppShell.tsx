import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import MenuIcon from '@mui/icons-material/Menu';
import SettingsIcon from '@mui/icons-material/Settings';
import AppBar from '@mui/material/AppBar';
import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import Collapse from '@mui/material/Collapse';
import Divider from '@mui/material/Divider';
import Drawer from '@mui/material/Drawer';
import IconButton from '@mui/material/IconButton';
import LinearProgress from '@mui/material/LinearProgress';
import List from '@mui/material/List';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemText from '@mui/material/ListItemText';
import Toolbar from '@mui/material/Toolbar';
import Typography from '@mui/material/Typography';
import { UI_KIT_NS } from '@ngfw/ui-kit';
import { useWsStatus } from '@ngfw/ui-kit/ws';
import { Suspense, useId, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { NavLink, Outlet, useLocation } from 'react-router';
import { DEV_ROUTES } from '../build-flags';
import { buildNav, currentNavPath, isCollapsible, type NavGroup } from '../nav/nav';
import { domains } from '../schema/registry';
import { ConfirmBanner } from '../config/ConfirmBanner';
import { PendingChangeBar, SyncBanner } from '../config/PendingChangeBar';
import { SettingsPopover } from './SettingsPopover';
import { UserMenu } from './UserMenu';

const DRAWER_WIDTH = 264;

interface NavListProps {
  nav: readonly NavGroup[];
  current: string | undefined;
  open: ReadonlySet<string>;
  onToggle: (groupId: string) => void;
  onNavigate: () => void;
}

function NavList({ nav, current, open, onToggle, onNavigate }: NavListProps) {
  const { t } = useTranslation(['common', 'nav']);
  // NavList is mounted twice while the phone drawer is open (temporary + permanent drawer): ids must stay unique
  const baseId = useId();
  const itemButton = (item: NavGroup['items'][number], indent: number) => (
    <ListItemButton
      key={item.id}
      component={NavLink}
      to={item.path}
      end
      selected={item.path === current}
      onClick={onNavigate}
      sx={{ paddingInlineStart: (theme) => theme.spacing(indent) }}
    >
      <ListItemText primary={t(item.labelKey, { defaultValue: item.fallbackLabel })} />
      {!item.available && <Chip size="small" variant="outlined" label={t('nav:soon')} />}
    </ListItemButton>
  );
  return (
    <List component="nav" aria-label={t('menu.navigation')} dense sx={{ pt: 0 }}>
      {nav.map((group) => {
        // a group with a single entry (Dashboard, Tools) is a plain top-level link: nothing to expand
        if (!isCollapsible(group)) {
          return (
            <Box key={group.id} component="li" sx={{ listStyle: 'none' }}>
              {itemButton(group.items[0]!, 2)}
            </Box>
          );
        }
        const expanded = open.has(group.id);
        const holdsCurrent = group.items.some((i) => i.path === current);
        const listId = `${baseId}-${group.id}`;
        return (
          <Box key={group.id} component="li" sx={{ listStyle: 'none' }}>
            {/* collapsed over the current page: the link's aria-current is hidden with it, so the header carries it ('true', not
                'page' — exactly one element is the page, review L4) */}
            <ListItemButton
              onClick={() => onToggle(group.id)}
              aria-expanded={expanded}
              aria-controls={listId}
              aria-current={holdsCurrent && !expanded ? 'true' : undefined}
            >
              <ListItemText
                primary={t(group.labelKey)}
                slotProps={{
                  primary: {
                    sx: {
                      fontWeight: holdsCurrent ? 700 : 500,
                      color: (theme) => (holdsCurrent ? theme.palette.primary.main : theme.palette.text.primary),
                    },
                  },
                }}
              />
              {expanded ? <ExpandLessIcon fontSize="small" /> : <ExpandMoreIcon fontSize="small" />}
            </ListItemButton>
            <Collapse in={expanded}>
              <List component="ul" id={listId} dense disablePadding>
                {group.items.map((item) => itemButton(item, 4))}
              </List>
            </Collapse>
          </Box>
        );
      })}
    </List>
  );
}

/**
 * Which navigation groups are expanded. All start collapsed except the one holding the current page, which opens on load
 * and on every navigation (a deep link must show where you are, D-117); the user can still close it. Shared by both drawers
 * so the phone drawer remembers what was open.
 */
function useNavGroups(devRoutes: boolean) {
  const nav = useMemo(() => buildNav(domains, { devRoutes }), [devRoutes]);
  const { pathname } = useLocation();
  // Exactly one item is current (review L4): the longest nav path that is the location or a parent of it, so `/vpn/tunnels`
  // selects "Tunnels" only, not also "VPN" (`/vpn`).
  const current = useMemo(() => currentNavPath(nav, pathname), [nav, pathname]);
  const currentGroup = nav.find((g) => isCollapsible(g) && g.items.some((i) => i.path === current))?.id;
  const [open, setOpen] = useState<ReadonlySet<string>>(() => new Set(currentGroup ? [currentGroup] : []));
  // open the current page's group when the location changes (state adjusted during render, not in an effect)
  const [seenPath, setSeenPath] = useState(pathname);
  if (seenPath !== pathname) {
    setSeenPath(pathname);
    if (currentGroup && !open.has(currentGroup)) setOpen(new Set(open).add(currentGroup));
  }
  const toggle = (id: string) =>
    setOpen((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  return { nav, current, open, toggle };
}

/** Application frame: fixed AppBar, grouped left navigation (from the schema's root keys), routed main area. */
export function AppShell({ devRoutes = DEV_ROUTES }: { devRoutes?: boolean }) {
  const { t } = useTranslation(['common', UI_KIT_NS]);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [settingsAnchor, setSettingsAnchor] = useState<HTMLElement | null>(null);
  const wsStatus = useWsStatus();
  const closeMobile = () => setMobileOpen(false);
  const { nav, current, open, toggle } = useNavGroups(devRoutes);

  const drawerContent = (
    <>
      <Toolbar />
      <Divider />
      <NavList nav={nav} current={current} open={open} onToggle={toggle} onNavigate={closeMobile} />
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
          <UserMenu />
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
        <ConfirmBanner />
        <SyncBanner />
        <PendingChangeBar />
        <Suspense fallback={<LinearProgress aria-label={t('loading')} />}>
          <Outlet />
        </Suspense>
      </Box>
    </Box>
  );
}
