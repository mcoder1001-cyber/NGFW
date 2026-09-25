import Box from '@mui/material/Box';
import { StatusChip } from '@ngfw/ui-kit';
import type { GridColDef } from '@ngfw/ui-kit/data-grid';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { localizeSchema } from '../vrf-static-ecmp/model';
import { Mono } from '../vrf-static-ecmp/common';
import { useBgpState, useCandidate } from './api';
import { lcpSchema, NS } from './model';
import { RecordEditor } from './RecordEditor';

/** Where the pairs live in the document: `interfaces.<name>.lcp`. */
const PAIRS = { k: 'pairs', root: 'interfaces', leaf: 'lcp', path: [] } as const;

const liveChip = (live: boolean) => (live ? ('up' as const) : ('down' as const));
const liveLabel = (live: boolean) => (live ? 'pairs.present' : 'pairs.absent');

interface PairRow {
  id: string;
  interface: string;
  hostIfName: string;
  hostIfType: string;
  netns: string;
  live: boolean;
}

/**
 * linux-cp pairs (`interfaces.<n>.lcp`, DF-8 lcp.itf-pair): the Linux interface FRR runs its sessions on for a VPP
 * interface, with the live pair from VPP (`GET /state/bgp`). The editor writes only the `lcp` leaf of the interface.
 */
export function PairsTab() {
  const { t } = useTranslation(NS);
  const ifs =
    useCandidate<
      Record<string, { lcp?: { hostIfName?: string; hostIfType?: string; netns?: string } }>
    >('interfaces');
  const live = useBgpState();
  const tr = (k: string, o?: Record<string, unknown>) => t(k, o ?? {});
  const schema = useMemo(() => localizeSchema(lcpSchema(), tr, 'lcp.'), [t]);
  const livePairs = useMemo(
    () => new Set((live.data?.lcpPairs ?? []).map((p) => p.interface)),
    [live.data],
  );
  const record = useMemo(
    () =>
      Object.fromEntries(
        Object.entries(ifs.data ?? {})
          .filter(([, v]) => v.lcp !== undefined)
          .map(([k, v]) => [k, v.lcp]),
      ),
    [ifs.data],
  );
  const rows = useMemo<PairRow[]>(
    () =>
      Object.entries(record).map(([name, l]) => ({
        id: name,
        interface: name,
        hostIfName: l?.hostIfName ?? name,
        hostIfType: l?.hostIfType ?? 'tap',
        netns: l?.netns ?? '',
        live: livePairs.has(name),
      })),
    [record, livePairs],
  );
  const columns = useMemo<GridColDef<PairRow>[]>(
    () => [
      {
        field: 'interface',
        headerName: t('pairs.col.interface'),
        flex: 1,
        renderCell: (p) => <Mono>{p.row.interface}</Mono>,
      },
      {
        field: 'hostIfName',
        headerName: t('pairs.col.hostIfName'),
        flex: 1,
        renderCell: (p) => <Mono>{p.row.hostIfName}</Mono>,
      },
      { field: 'hostIfType', headerName: t('pairs.col.hostIfType'), width: 100 },
      { field: 'netns', headerName: t('pairs.col.netns'), width: 140 },
      {
        field: 'live',
        headerName: t('pairs.col.live'),
        width: 150,
        renderCell: (p) => (
          <StatusChip size="small" status={liveChip(p.row.live)} label={t(liveLabel(p.row.live))} />
        ),
      },
    ],
    [t],
  );
  return (
    <Box>
      <RecordEditor<PairRow>
        {...PAIRS}
        record={record}
        schema={schema}
        rows={rows}
        columns={columns}
        checkKey={(k) =>
          ifs.data?.[k] === undefined
            ? t('pairs.unknownInterface')
            : record[k] !== undefined
              ? t('pairs.exists')
              : ''
        }
        version={ifs.dataUpdatedAt + live.dataUpdatedAt}
      />
    </Box>
  );
}
