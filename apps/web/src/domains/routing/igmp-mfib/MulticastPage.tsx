import { PgButton as Button } from '@amnafzar/common-ui-lib/forms';
import { ReactGeereedMaterial } from '@amnafzar/common-ui-lib/geereed';
import { PgRefreshIcon as RefreshIcon } from '@amnafzar/common-ui-lib/icons';
import {
  PgAlert as Alert,
  PgBox as Box,
  PgLinearProgress as LinearProgress,
  PgPaper as Paper,
  PgStack as Stack,
  PgTableContainer as TableContainer,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { useInterfaceLabel } from '@ngfw/ui-kit';
import { serviceText } from '../../../product-text';
import { useQueryClient } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { serviceText } from '../../../product-text';
import { PageHeader } from '../../../shell/PageHeader';
import { multicastKeys, useMroutes, useMulticastGroups, usePimNeighbors } from './queries';

/** F-igmp-mfib: `/routing/multicast` — live IGMP groups, the VPP mFIB and PIM neighbours. Config is under Config → Routing. */
const ANY_SOURCE = '*';

export function MulticastPage() {
  const interfaceLabel = useInterfaceLabel();
  const { t } = useTranslation('igmp-mfib');
  const qc = useQueryClient();
  const groups = useMulticastGroups();
  const mroutes = useMroutes();
  const pim = usePimNeighbors();
  const agentError =
    groups.data?.agentError ?? mroutes.data?.agentError ?? pim.data?.agentError ?? null;

  return (
    <Box>
      <PageHeader title={t('title')}>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2, maxInlineSize: 900 }}>
          {t('intro')}
        </Typography>
      </PageHeader>

      <Stack direction="row" sx={{ mb: 1 }}>
        <Box sx={{ flex: 1 }} />
        <Button
          startIcon={<RefreshIcon />}
          onClick={() => void qc.invalidateQueries({ queryKey: multicastKeys.all })}
        >
          {t('refresh')}
        </Button>
      </Stack>

      {(groups.isPending || mroutes.isPending || pim.isPending) && (
        <LinearProgress aria-label={t('loading')} sx={{ mb: 1 }} />
      )}
      {groups.isError && <ProblemAlert error={groups.error} sx={{ mb: 1 }} />}
      {agentError && (
        <Alert severity="info" sx={{ mb: 2 }}>
          {t('agentError', { error: serviceText(agentError) })}
        </Alert>
      )}

      <Section title={t('groups.title')}>
        {(groups.data?.groups ?? []).length === 0 ? (
          <Alert severity="info">{t('groups.empty')}</Alert>
        ) : (
          <TableContainer component={Paper} variant="outlined">
            {(() => {
              const gridItems = groups.data?.groups ?? [];
              type GridRow = (typeof gridItems)[number];
              return (
                <ReactGeereedMaterial
                  hideFilterBuilderComponent
                  noFormWrapper
                  embedded
                  memoEnable={false}
                  hidePagination
                  hideToolbar
                  search={false}
                  exportToCSV={false}
                  sortable={false}
                  footer={false}
                  items={gridItems}
                  columns={[
                    {
                      key: '0',
                      sortable: false,
                      filterable: false,
                      headerRenderer: () => <>{t('groups.col.interface')}</>,
                      renderer: (_value: unknown, gridRow: GridRow) => {
                        const g = gridRow;
                        return <>{g.interface}</>;
                      },
                    },
                    {
                      key: '1',
                      sortable: false,
                      filterable: false,
                      headerRenderer: () => <>{t('groups.col.group')}</>,
                      renderer: (_value: unknown, gridRow: GridRow) => {
                        const g = gridRow;
                        return <>{g.group}</>;
                      },
                    },
                    {
                      key: '2',
                      sortable: false,
                      filterable: false,
                      headerRenderer: () => <>{t('groups.col.sources')}</>,
                      renderer: (_value: unknown, gridRow: GridRow) => {
                        const g = gridRow;
                        return <>{g.sources.join(', ')}</>;
                      },
                    },
                  ]}
                  tableProps={{
                    size: 'small',
                    'aria-label': t('groups.title'),
                  }}
                  rowKey={(nativeRow, gridIndex) => {
                    const gridRow = nativeRow as unknown as GridRow;
                    const g = gridRow;
                    const i = gridIndex;
                    return `${g.interface}-${g.group}-${i}`;
                  }}
                  rowProps={(_nativeRow, _gridIndex) => {
                    return {};
                  }}
                  cellProps={(_nativeRow, column, _gridIndex) => {
                    switch (column.key) {
                      case '0':
                        return { dir: 'ltr' };
                      case '1':
                        return { dir: 'ltr' };
                      case '2':
                        return { dir: 'ltr' };
                      default:
                        return {};
                    }
                  }}
                  emptyContent={null}
                />
              );
            })()}
          </TableContainer>
        )}
      </Section>

      <Section title={t('mroutes.title')}>
        {(mroutes.data?.mroutes ?? []).length === 0 ? (
          <Alert severity="info">{t('mroutes.empty')}</Alert>
        ) : (
          <TableContainer component={Paper} variant="outlined">
            {(() => {
              const gridItems = mroutes.data?.mroutes ?? [];
              type GridRow = (typeof gridItems)[number];
              return (
                <ReactGeereedMaterial
                  hideFilterBuilderComponent
                  noFormWrapper
                  embedded
                  memoEnable={false}
                  hidePagination
                  hideToolbar
                  search={false}
                  exportToCSV={false}
                  sortable={false}
                  footer={false}
                  items={gridItems}
                  columns={[
                    {
                      key: '0',
                      sortable: false,
                      filterable: false,
                      headerRenderer: () => <>{t('mroutes.col.vrf')}</>,
                      renderer: (_value: unknown, gridRow: GridRow) => {
                        const m = gridRow;
                        return <>{m.vrf}</>;
                      },
                    },
                    {
                      key: '1',
                      sortable: false,
                      filterable: false,
                      headerRenderer: () => <>{t('mroutes.col.group')}</>,
                      renderer: (_value: unknown, gridRow: GridRow) => {
                        const m = gridRow;
                        return <>{m.group}</>;
                      },
                    },
                    {
                      key: '2',
                      sortable: false,
                      filterable: false,
                      headerRenderer: () => <>{t('mroutes.col.source')}</>,
                      renderer: (_value: unknown, gridRow: GridRow) => {
                        const m = gridRow;
                        return <>{m.source || ANY_SOURCE}</>;
                      },
                    },
                    {
                      key: '3',
                      sortable: false,
                      filterable: false,
                      headerRenderer: () => <>{t('mroutes.col.accept')}</>,
                      renderer: (_value: unknown, gridRow: GridRow) => {
                        const m = gridRow;
                        return <>{m.accept}</>;
                      },
                    },
                    {
                      key: '4',
                      sortable: false,
                      filterable: false,
                      headerRenderer: () => <>{t('mroutes.col.forward')}</>,
                      renderer: (_value: unknown, gridRow: GridRow) => {
                        const m = gridRow;
                        return <>{m.forward.join(', ')}</>;
                      },
                    },
                  ]}
                  tableProps={{
                    size: 'small',
                    'aria-label': t('mroutes.title'),
                  }}
                  rowKey={(nativeRow, gridIndex) => {
                    const gridRow = nativeRow as unknown as GridRow;
                    const m = gridRow;
                    const i = gridIndex;
                    return `${m.vrf}-${m.group}-${m.source}-${i}`;
                  }}
                  rowProps={(_nativeRow, _gridIndex) => {
                    return {};
                  }}
                  cellProps={(_nativeRow, column, _gridIndex) => {
                    switch (column.key) {
                      case '0':
                        return { dir: 'ltr' };
                      case '1':
                        return { dir: 'ltr' };
                      case '2':
                        return { dir: 'ltr' };
                      case '3':
                        return { dir: 'ltr' };
                      case '4':
                        return { dir: 'ltr' };
                      default:
                        return {};
                    }
                  }}
                  emptyContent={null}
                />
              );
            })()}
          </TableContainer>
        )}
      </Section>

      <Section title={t('pim.title')}>
        {(pim.data?.neighbors ?? []).length === 0 ? (
          <Alert severity="info">{t('pim.empty')}</Alert>
        ) : (
          <TableContainer component={Paper} variant="outlined">
            {(() => {
              const gridItems = pim.data?.neighbors ?? [];
              type GridRow = (typeof gridItems)[number];
              return (
                <ReactGeereedMaterial
                  hideFilterBuilderComponent
                  noFormWrapper
                  embedded
                  memoEnable={false}
                  hidePagination
                  hideToolbar
                  search={false}
                  exportToCSV={false}
                  sortable={false}
                  footer={false}
                  items={gridItems}
                  columns={[
                    {
                      key: '0',
                      sortable: false,
                      filterable: false,
                      headerRenderer: () => <>{t('pim.col.interface')}</>,
                      renderer: (_value: unknown, gridRow: GridRow) => {
                        const n = gridRow;
                        return <>{n.interface}</>;
                      },
                    },
                    {
                      key: '1',
                      sortable: false,
                      filterable: false,
                      headerRenderer: () => <>{t('pim.col.address')}</>,
                      renderer: (_value: unknown, gridRow: GridRow) => {
                        const n = gridRow;
                        return <>{n.address}</>;
                      },
                    },
                    {
                      key: '2',
                      sortable: false,
                      filterable: false,
                      headerRenderer: () => <>{t('pim.col.uptime')}</>,
                      renderer: (_value: unknown, gridRow: GridRow) => {
                        const n = gridRow;
                        return <>{n.uptimeSec}</>;
                      },
                    },
                  ]}
                  tableProps={{ size: 'small', 'aria-label': t('pim.title') }}
                  rowKey={(nativeRow, gridIndex) => {
                    const gridRow = nativeRow as unknown as GridRow;
                    const n = gridRow;
                    const i = gridIndex;
                    return `${n.interface}-${n.address}-${i}`;
                  }}
                  rowProps={(_nativeRow, _gridIndex) => {
                    return {};
                  }}
                  cellProps={(_nativeRow, column, _gridIndex) => {
                    switch (column.key) {
                      case '0':
                        return { dir: 'ltr' };
                      case '1':
                        return { dir: 'ltr' };
                      case '2':
                        return { dir: 'ltr', sx: { textAlign: 'end' } };
                      default:
                        return {};
                    }
                  }}
                  emptyContent={null}
                />
              );
            })()}
          </TableContainer>
        )}
      </Section>
    </Box>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Box sx={{ mb: 3 }}>
      <Typography component="h3" variant="subtitle1" sx={{ mb: 1 }}>
        {title}
      </Typography>
      {children}
    </Box>
  );
}
