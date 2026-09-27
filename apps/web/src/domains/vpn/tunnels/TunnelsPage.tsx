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
import { ifaceKeys, useInterfacesState } from '../../interfaces/queries';
import { KINDS, tunnelStatus, vppName, type KindKey, type TunnelItem } from './model';

export const NS = 'tunnels';
const ALSO_INVALIDATE = [ifaceKeys.state] as const;

function KindView({ kind }: { kind: KindKey }) {
  const { t } = useTranslation(NS);
  const state = useInterfacesState();
  const items = state.data?.items;
  const spec = useMemo(() => ({ domain: 'tunnels' as const, path: [kind], ns: NS }), [kind]);
  const columns = useMemo<GridColDef<CollectionRow<TunnelItem>>[]>(() => {
    const cols: GridColDef<CollectionRow<TunnelItem>>[] = [
      {
        field: 'engineName',
        headerName: t('col.engineName'),
        width: 150,
        valueGetter: (_v, row) => vppName(kind, row.value ?? row.running) ?? t('noInstance'),
      },
      { field: 'src', headerName: t('col.src'), width: 150 },
      { field: 'dst', headerName: t('col.dst'), width: 150 },
    ];
    if (kind === 'vxlan') cols.push({ field: 'vni', headerName: t('col.vni'), width: 100 });
    if (kind === 'gre') cols.push({ field: 'type', headerName: t('col.type'), width: 100 });
    if (kind === 'ipip') cols.push({ field: 'mode', headerName: t('col.mode'), width: 100 });
    return cols;
  }, [kind, t]);
  return (
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
        render: (row) => <StatusCell status={tunnelStatus(items, vppName(kind, row.value ?? row.running))} />,
      }}
      options={{ alsoInvalidate: ALSO_INVALIDATE }}
    />
  );
}

/**
 * VPN › Tunnels (F-tunnels): `tunnels.gre`, `tunnels.vxlan` and `tunnels.ipip` — one tab per kind, each the WEB-2
 * config screen kit (list + drawer with the SchemaForm of the one schema). The status column is the tunnel
 * interface's link state from `GET /api/v1/state/interfaces` (tunnels are interfaces there, named by `instance`).
 * Edits go to the candidate; the pending-change bar commits them.
 */
export function TunnelsPage() {
  const { t } = useTranslation(NS);
  const [kind, setKind] = useState<KindKey>('gre');
  return (
    <PageHeader title={t('title')}>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('intro')}
      </Typography>
      <Tabs value={kind} onChange={(_, v: KindKey) => setKind(v)} aria-label={t('kinds')} sx={{ mb: 2 }}>
        {KINDS.map((k) => (
          <Tab key={k.key} value={k.key} label={t(`kind.${k.key}`)} id={`tunnels-tab-${k.key}`} aria-controls={`tunnels-panel-${k.key}`} />
        ))}
      </Tabs>
      <Box role="tabpanel" id={`tunnels-panel-${kind}`} aria-labelledby={`tunnels-tab-${kind}`}>
        <KindView key={kind} kind={kind} />
      </Box>
    </PageHeader>
  );
}
