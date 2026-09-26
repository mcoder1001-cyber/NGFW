import RefreshIcon from '@mui/icons-material/Refresh';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import CircularProgress from '@mui/material/CircularProgress';
import Stack from '@mui/material/Stack';
import Tab from '@mui/material/Tab';
import Tabs from '@mui/material/Tabs';
import Typography from '@mui/material/Typography';
import { useQueryClient } from '@tanstack/react-query';
import { Suspense, useState, type ComponentType } from 'react';
import { useTranslation } from 'react-i18next';
import { qk } from '../../../config/queries';
import { QOS_STATE_KEY } from './queries';
import { AttachmentsTab, MapsTab, PolicersTab, ShapersTab } from './Sections';

const SECTIONS: readonly { id: string; Component: ComponentType }[] = [
  { id: 'policers', Component: PolicersTab },
  { id: 'shapers', Component: ShapersTab },
  { id: 'maps', Component: MapsTab },
  { id: 'interfaces', Component: AttachmentsTab },
];

/**
 * F-qos-flat: the QoS tab of the Services page — policers (with live counters and a token-bucket reset), rate limits
 * (the schema's shapers: egress policers, V3), marking maps (DSCP / PCP / EXP translation grids) and interface
 * attachments, all edited in the candidate through the generic pointer routes (the pending-change bar shows the diff).
 * Hierarchical QoS and queues are not offered (V3: not in VPP 26.06).
 */
export function QosPage() {
  const { t } = useTranslation('qos-flat');
  const [section, setSection] = useState('policers');
  const current = SECTIONS.find((s) => s.id === section) ?? SECTIONS[0]!;
  const Current = current.Component;
  const qc = useQueryClient();
  const [refreshing, setRefreshing] = useState(false);
  // D-132: the policer state polls every 30 s at most; Refresh reads VPP (through the agent) and the candidate now
  const refresh = async () => {
    setRefreshing(true);
    try {
      await Promise.all([
        qc.invalidateQueries({ queryKey: QOS_STATE_KEY }),
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
            id={`qos-tab-${s.id}`}
            aria-controls={`qos-panel-${s.id}`}
          />
        ))}
      </Tabs>
      <Box role="tabpanel" id={`qos-panel-${current.id}`} aria-labelledby={`qos-tab-${current.id}`}>
        <Suspense fallback={<CircularProgress aria-label={t('loading')} />}>
          <Current />
        </Suspense>
      </Box>
    </Box>
  );
}

export default QosPage;
