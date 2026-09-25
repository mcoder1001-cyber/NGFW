import RefreshIcon from '@mui/icons-material/Refresh';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import CircularProgress from '@mui/material/CircularProgress';
import Link from '@mui/material/Link';
import Stack from '@mui/material/Stack';
import Tab from '@mui/material/Tab';
import Tabs from '@mui/material/Tabs';
import Typography from '@mui/material/Typography';
import { useQueryClient } from '@tanstack/react-query';
import { Suspense, useState, type ComponentType } from 'react';
import { useTranslation } from 'react-i18next';
import { Link as RouterLink } from 'react-router';
import { RelaysTab, ReservationsTab, ServersTab, SubnetsTab } from './ConfigTabs';
import { LeasesTab } from './LeasesTab';
import { qk } from '../../../config/queries';
import { DHCP_STATE_KEY } from './queries';

const SECTIONS: readonly { id: string; Component: ComponentType }[] = [
  { id: 'servers', Component: ServersTab },
  { id: 'subnets', Component: SubnetsTab },
  { id: 'reservations', Component: ReservationsTab },
  { id: 'relays', Component: RelaysTab },
  { id: 'leases', Component: LeasesTab },
];

/**
 * F-kea-dhcp-relay: the DHCP tab of the Services page — Kea servers, subnets & pools (with pool utilisation),
 * reservations and VPP relays, all edited in the candidate through the generic pointer routes (the pending-change bar
 * shows the diff), plus the lease browser (server-side paging). The DHCP client of an interface is configured in the
 * interface drawer.
 */
export function DhcpPage() {
  const { t } = useTranslation('kea-dhcp-relay');
  const [section, setSection] = useState('servers');
  const current = SECTIONS.find((s) => s.id === section) ?? SECTIONS[0]!;
  const Current = current.Component;
  const qc = useQueryClient();
  const [refreshing, setRefreshing] = useState(false);
  // D-132: state polls every 30 s at most; Refresh reads the daemons, VPP and the candidate now
  const refresh = async () => {
    setRefreshing(true);
    try {
      await Promise.all([
        qc.invalidateQueries({ queryKey: DHCP_STATE_KEY }),
        qc.invalidateQueries({ queryKey: qk.candidate('services') }),
      ]);
    } finally {
      setRefreshing(false);
    }
  };
  return (
    <Box>
      <Stack direction="row" alignItems="flex-start" gap={2} sx={{ mb: 1 }}>
        <Typography color="text.secondary" sx={{ flex: 1 }}>
          {t('intro')}
        </Typography>
        <Button
          size="small"
          variant="outlined"
          startIcon={<RefreshIcon />}
          disabled={refreshing}
          onClick={() => void refresh()}
        >
          {t('refresh')}
        </Button>
      </Stack>
      <Alert severity="info" sx={{ mb: 2 }}>
        {t('clientHint')}{' '}
        <Link component={RouterLink} to="/interfaces">
          {t('clientLink')}
        </Link>
      </Alert>
      <Tabs
        value={current.id}
        onChange={(_, id: string) => setSection(id)}
        aria-label={t('sections')}
        variant="scrollable"
        sx={{ mb: 2 }}
      >
        {SECTIONS.map((s) => (
          <Tab
            key={s.id}
            value={s.id}
            label={t(`tabs.${s.id}`)}
            id={`dhcp-tab-${s.id}`}
            aria-controls={`dhcp-panel-${s.id}`}
          />
        ))}
      </Tabs>
      <Box
        role="tabpanel"
        id={`dhcp-panel-${current.id}`}
        aria-labelledby={`dhcp-tab-${current.id}`}
      >
        <Suspense fallback={<CircularProgress aria-label={t('loading')} />}>
          <Current />
        </Suspense>
      </Box>
    </Box>
  );
}

export default DhcpPage;
