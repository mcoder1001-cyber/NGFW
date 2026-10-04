import Alert from '@mui/material/Alert';
import FormControlLabel from '@mui/material/FormControlLabel';
import Switch from '@mui/material/Switch';
import Box from '@mui/material/Box';
import Tab from '@mui/material/Tab';
import Tabs from '@mui/material/Tabs';
import Typography from '@mui/material/Typography';
import type { GridColDef } from '@ngfw/ui-kit/data-grid';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { CollectionView, type CollectionRow } from '../../../config/collection';
import { StatusCell } from '../../../config/widgets/cells';
import { PageHeader } from '../../../shell/PageHeader';
import { ifaceKeys } from '../../interfaces/queries';
import { tunnelKeys, useTunnelsState } from './queries';
import { ADVANCED, KINDS, liveTunnel, liveStatus, type KindKey, type TunnelItem } from './model';

export const NS = 'tunnels';
const ALSO_INVALIDATE = [ifaceKeys.state, tunnelKeys.state] as const;

function KindView({ kind }: { kind: KindKey }) {
  const { t } = useTranslation(NS);
  const state = useTunnelsState();
  const items = state.isError ? undefined : state.data?.items;
  const spec = useMemo(() => ({ domain: 'tunnels' as const, path: [kind], ns: NS }), [kind]);
  const columns = useMemo<GridColDef<CollectionRow<TunnelItem>>[]>(() => {
    const cols: GridColDef<CollectionRow<TunnelItem>>[] = [
      {
        field: 'engineName',
        headerName: t('col.engineName'),
        width: 150,
        valueGetter: (_v, row) => liveTunnel(items, kind, row.label)?.interface ?? t('unavailable'),
      },
      { field: 'src', headerName: t('col.src'), width: 150 },
      { field: 'dst', headerName: t('col.dst'), width: 150 },
    ];
    cols.push(
      {
        field: 'liveSrc',
        headerName: t('col.liveSrc'),
        width: 150,
        valueGetter: (_v, row) => liveTunnel(items, kind, row.label)?.src ?? t('unavailable'),
      },
      {
        field: 'liveDst',
        headerName: t('col.liveDst'),
        width: 150,
        valueGetter: (_v, row) => liveTunnel(items, kind, row.label)?.dst ?? t('unavailable'),
      },
      {
        field: 'underlay',
        headerName: t('col.underlay'),
        width: 110,
        valueGetter: (_v, row) =>
          liveTunnel(items, kind, row.label)?.underlayTableId ?? t('unavailable'),
      },
      {
        field: 'liveVrf',
        headerName: t('col.liveVrf'),
        width: 130,
        valueGetter: (_v, row) => {
          const live = liveTunnel(items, kind, row.label);
          return live
            ? `${live.ipv4TableId ?? '—'} / ${live.ipv6TableId ?? '—'}`
            : t('unavailable');
        },
      },
      {
        field: 'counters',
        headerName: t('col.counters'),
        width: 180,
        valueGetter: (_v, row) => {
          const c = liveTunnel(items, kind, row.label)?.counters;
          return c ? `${c.rxPackets} / ${c.txPackets}` : t('unavailable');
        },
      },
    );
    if (kind === 'vxlan' || kind === 'vxlanGpe')
      cols.push({ field: 'vni', headerName: t('col.vni'), width: 100 });
    if (kind === 'gre') cols.push({ field: 'type', headerName: t('col.type'), width: 100 });
    if (kind === 'ipip') cols.push({ field: 'mode', headerName: t('col.mode'), width: 100 });
    return cols;
  }, [kind, t, items]);
  return (
    <>
      {state.isError && <Alert severity="warning">{t('stateError')}</Alert>}
      {ADVANCED.includes(kind) && (
        <Alert severity="warning" sx={{ mb: 2 }}>
          {t(`limits.${kind}`)}
        </Alert>
      )}
      {kind === 'ipip' && (
        <Alert severity="info" sx={{ mb: 2 }}>
          {t('limits.sixrd')}
        </Alert>
      )}
      <CollectionView<TunnelItem>
        spec={spec}
        label={t(`kind.${kind}`)}
        itemLabel={(k) => t('item', { kind: t(`kind.${kind}`), name: k })}
        keyHeader={t('col.name')}
        addLabel={t('add', { kind: t(`kind.${kind}`) })}
        columns={columns}
        status={{
          header: t('col.status'),
          width: 150,
          render: (row) => <StatusCell status={liveStatus(liveTunnel(items, kind, row.label))} />,
        }}
        options={{ alsoInvalidate: ALSO_INVALIDATE }}
      />
    </>
  );
}

export function TunnelsPage() {
  const { t } = useTranslation(NS);
  const [advanced, setAdvanced] = useState(false);
  const [kind, setKind] = useState<KindKey>('gre');
  return (
    <PageHeader title={t('title')}>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('intro')}
      </Typography>
      <FormControlLabel
        control={
          <Switch
            checked={advanced}
            onChange={(_, checked) => {
              setAdvanced(checked);
              if (!checked && ADVANCED.includes(kind)) setKind(KINDS[0].key);
            }}
          />
        }
        label={t('advanced')}
      />
      <Tabs
        variant="scrollable"
        value={kind}
        onChange={(_, v: KindKey) => setKind(v)}
        aria-label={t('kinds')}
        sx={{ mb: 2 }}
      >
        {KINDS.filter((k) => advanced || !ADVANCED.includes(k.key)).map((k) => (
          <Tab
            key={k.key}
            value={k.key}
            label={t(`kind.${k.key}`)}
            id={`tunnels-tab-${k.key}`}
            aria-controls={`tunnels-panel-${k.key}`}
          />
        ))}
      </Tabs>
      <Box role="tabpanel" id={`tunnels-panel-${kind}`} aria-labelledby={`tunnels-tab-${kind}`}>
        <KindView key={kind} kind={kind} />
      </Box>
    </PageHeader>
  );
}
