import Alert from '@mui/material/Alert';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import { VRX_STATUSES, type VrxStatus } from '@ngfw/ui-kit';
import type { GridColDef } from '@ngfw/ui-kit/data-grid';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { PageHeader } from '../../shell/PageHeader';
import { CollectionDrawer } from '../collection/CollectionDrawer';
import type { RowState } from '../collection/model';
import { CounterText, IdText, LiveChip, RateText, RowStateChip, StatusCell } from './cells';
import { KeyButton } from './KeyButton';
import { LocalDataGrid } from './LocalDataGrid';
import { Sparkline } from './Sparkline';

interface PreviewRow {
  id: string;
  status: VrxStatus | undefined;
  packets: string;
}

/** Synthetic rows (the page says so): nothing here comes from or goes to the device. */
const ROWS: PreviewRow[] = [
  { id: 'loop100', status: 'up', packets: '18446744073709551615' },
  { id: 'host-w1l0', status: 'down', packets: '9007199254740993' },
  { id: 'GigabitEthernet0/8/0', status: 'adminDown', packets: '42' },
  { id: 'bond0', status: undefined, packets: '0' },
];
const TREND = [3, 5, 4, 8, 13, 9, 11, 16, 12, 18];
const ROW_STATES: RowState[] = ['new', 'changed', 'removed'];
const WS_STATES = ['idle', 'connecting', 'open', 'reconnecting', 'closed'];
const GRID_KEY = ['kit', 'preview'] as const;
const PREFIX = '2001:db8::1/64';
const BPS = 'bps' as const;
const PPS = 'pps' as const;
const TREND_OF = { name: 'loop100' };

/**
 * Developer preview of the config kit's data widgets in the current language and direction (dev builds only).
 * Its own strings live in `dev:kitPreview.*` (review L3), not `config`: `config` ships in the production bundle
 * always, `dev` only when `DEV_ROUTES` is built in, and this page exists only as a dev route.
 */
export function KitPreview() {
  const { t } = useTranslation(['config', 'dev']);
  const [open, setOpen] = useState<string | null>(null);
  const columns = useMemo<GridColDef<PreviewRow>[]>(
    () => [
      {
        field: 'id',
        headerName: t('kit.secrets.col.name'),
        flex: 1,
        renderCell: (p) => (
          <KeyButton tabIndex={p.tabIndex} hasFocus={p.hasFocus} aria-label={t('kit.open', { key: p.row.id })} onClick={() => setOpen(p.row.id)}>
            {p.row.id}
          </KeyButton>
        ),
      },
      { field: 'status', headerName: t('dev:kitPreview.status'), width: 150, renderCell: (p) => <StatusCell status={p.row.status} /> },
      { field: 'packets', headerName: t('dev:kitPreview.counter'), width: 240, renderCell: (p) => <CounterText value={p.row.packets} /> },
    ],
    [t],
  );
  return (
    <PageHeader title={t('dev:kitPreview.title')}>
      <Alert severity="info" sx={{ mb: 2 }}>
        {t('dev:kitPreview.intro')}
      </Alert>
      <Typography component="h3" variant="subtitle1" gutterBottom>
        {t('dev:kitPreview.statusSection')}
      </Typography>
      <Stack direction="row" gap={1} flexWrap="wrap" alignItems="center" sx={{ mb: 1 }}>
        {VRX_STATUSES.map((s) => (
          <StatusCell key={s} status={s} />
        ))}
        <StatusCell status={undefined} />
      </Stack>
      <Stack direction="row" gap={2} flexWrap="wrap" alignItems="center" sx={{ mb: 1 }}>
        <CounterText value="18446744073709551615" />
        <RateText value={1.25e9} unit={BPS} />
        <RateText value={3.4e5} unit={PPS} />
        <Sparkline values={TREND} label={t('dev:kitPreview.trend', TREND_OF)} />
        <IdText>{PREFIX}</IdText>
      </Stack>
      <Stack direction="row" gap={1} flexWrap="wrap" sx={{ mb: 1 }}>
        {ROW_STATES.map((s) => (
          <RowStateChip key={s} state={s} />
        ))}
      </Stack>
      <Stack direction="row" gap={1} flexWrap="wrap" sx={{ mb: 3 }}>
        {WS_STATES.map((s) => (
          <LiveChip key={s} status={s} />
        ))}
      </Stack>
      <Typography component="h3" variant="subtitle1" gutterBottom>
        {t('dev:kitPreview.gridSection')}
      </Typography>
      <Paper variant="outlined" sx={{ blockSize: 280 }}>
        <LocalDataGrid aria-label={t('dev:kitPreview.gridSection')} gridKey={GRID_KEY} rows={ROWS} columns={columns} />
      </Paper>
      <CollectionDrawer open={open !== null} onClose={() => setOpen(null)} title={open ?? ''} label={t('dev:kitPreview.item', { key: open ?? '' })}>
        <Typography>{t('dev:kitPreview.itemBody')}</Typography>
      </CollectionDrawer>
    </PageHeader>
  );
}
