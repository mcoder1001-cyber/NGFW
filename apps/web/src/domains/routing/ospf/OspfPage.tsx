import { ReactGeereedMaterial } from '@amnafzar/common-ui-lib/geereed';
import {
  PgAlert as Alert,
  PgBox as Box,
  PgChip as Chip,
  PgLinearProgress as LinearProgress,
  PgPaper as Paper,
  PgStack as Stack,
  PgTab as Tab,
  PgTabs as Tabs,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { useInterfaceLabel } from '@ngfw/ui-kit';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { PageHeader } from '../../../shell/PageHeader';
import { NS, registerIgpLocale } from './locale';
import { ProtocolForm } from './ProtocolForm';
import { useCandidateRouting, useOspfNeighbors, useRunningRouting } from './queries';

registerIgpLocale();

const DEFAULT_AREA_TYPE = 'normal';
const V2 = 'ospf' as const;
const V3 = 'ospf6' as const;
const NEIGHBOR_COLUMNS = ['vrf', 'routerId', 'interface', 'state'] as const;

/**
 * Routing › OSPF (WEB-4a, D-123): schema-driven `routing.ospf` form plus area / interface summaries. Merged UNROUTED —
 * F-ospf adds the route/nav entry and live neighbours / LSDB (no state endpoint yet).
 */
export function OspfPage() {
  const interfaceLabel = useInterfaceLabel();
  const { t } = useTranslation([NS, 'config']);
  const cand = useCandidateRouting();
  const running = useRunningRouting();
  const [proto, setProto] = useState<'ospf' | 'ospf6'>('ospf');
  const observed = useOspfNeighbors(proto === 'ospf' ? '2' : '3');
  const ospf = cand.data?.[proto];
  const areas = Object.entries(ospf?.areas ?? {});
  const ifaces = Object.entries(ospf?.interfaces ?? {});
  const yn = (v: boolean | undefined) => (v ? t('yes') : t('no'));
  return (
    <PageHeader title={t('ospf.title')}>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('ospf.intro')}
      </Typography>
      {proto === 'ospf' && <Alert severity="info">{t('ospf.authBoundary')}</Alert>}
      {(cand.isPending || running.isPending) && (
        <LinearProgress aria-label={t('config:loading')} sx={{ mb: 2 }} />
      )}
      {cand.isError && <ProblemAlert error={cand.error} sx={{ mb: 2 }} />}
      {running.isError && <ProblemAlert error={running.error} sx={{ mb: 2 }} />}
      <Tabs
        value={proto}
        onChange={(_, value: 'ospf' | 'ospf6') => setProto(value)}
        aria-label={t('ospf.versions')}
      >
        <Tab value={V2} label={t('ospf.v2')} />
        <Tab value={V3} label={t('ospf.v3')} />
      </Tabs>
      <Paper variant="outlined" sx={{ p: 2, mb: 2 }}>
        <Typography component="h3" variant="h6">
          {t('ospf.neighbors')}
        </Typography>
        {observed.isPending && <LinearProgress aria-label={t('config:loading')} />}
        {observed.isError && <ProblemAlert error={observed.error} />}
        {observed.data && (
          <>
            {(observed.data.unavailable || observed.data.warning || observed.data.truncated) && (
              <Alert severity="warning">{t('ospf.observationWarning')}</Alert>
            )}
            {!observed.data.unavailable && observed.data.neighbors.length === 0 && (
              <Typography>{t('ospf.noNeighbors')}</Typography>
            )}
            {(() => {
              const gridItems = observed.data.neighbors;
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
                      headerRenderer: () => <>{t(`ospf.${NEIGHBOR_COLUMNS[0]}`)}</>,
                      renderer: (_value: unknown, gridRow: GridRow) => {
                        const n = gridRow;
                        return <>{n.vrf}</>;
                      },
                    },
                    {
                      key: '1',
                      sortable: false,
                      filterable: false,
                      headerRenderer: () => <>{t(`ospf.${NEIGHBOR_COLUMNS[1]}`)}</>,
                      renderer: (_value: unknown, gridRow: GridRow) => {
                        const n = gridRow;
                        return <>{n.routerId}</>;
                      },
                    },
                    {
                      key: '2',
                      sortable: false,
                      filterable: false,
                      headerRenderer: () => <>{t(`ospf.${NEIGHBOR_COLUMNS[2]}`)}</>,
                      renderer: (_value: unknown, gridRow: GridRow) => {
                        const n = gridRow;
                        return <>{interfaceLabel(n.interface ?? '—')}</>;
                      },
                    },
                    {
                      key: '3',
                      sortable: false,
                      filterable: false,
                      headerRenderer: () => <>{t(`ospf.${NEIGHBOR_COLUMNS[3]}`)}</>,
                      renderer: (_value: unknown, gridRow: GridRow) => {
                        const n = gridRow;
                        return (
                          <>
                            <Chip
                              size="small"
                              color={n.state.startsWith('Full') ? 'success' : 'warning'}
                              label={n.state}
                            />
                          </>
                        );
                      },
                    },
                  ]}
                  tableProps={{
                    size: 'small',
                    'aria-label': t('ospf.neighbors'),
                  }}
                  rowKey={(nativeRow, gridIndex) => {
                    const gridRow = nativeRow as unknown as GridRow;
                    const n = gridRow;
                    const i = gridIndex;
                    return `${n.vrf}-${n.routerId}-${n.interface}-${i}`;
                  }}
                  rowProps={(_nativeRow, _gridIndex) => {
                    return {};
                  }}
                  cellProps={(_nativeRow, column, _gridIndex) => {
                    switch (column.key) {
                      case '0':
                        return {};
                      case '1':
                        return { dir: 'ltr' };
                      case '2':
                        return { dir: 'ltr' };
                      case '3':
                        return {};
                      default:
                        return {};
                    }
                  }}
                  emptyContent={null}
                />
              );
            })()}
          </>
        )}
      </Paper>
      <Stack direction={{ xs: 'column', lg: 'row' }} spacing={3} alignItems="flex-start">
        <ProtocolForm key={proto} proto={proto} label={t(proto === V2 ? 'ospf.v2' : 'ospf.v3')} />
        <Paper variant="outlined" sx={{ p: 2, flex: 1, width: '100%' }}>
          <Typography component="h3" variant="h6" gutterBottom>
            {t('ospf.areas')}
          </Typography>
          {areas.length === 0 ? (
            <Typography color="text.secondary">{t('ospf.empty')}</Typography>
          ) : (
            (() => {
              const gridItems = areas;
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
                      headerRenderer: () => <>{t('ospf.area')}</>,
                      renderer: (_value: unknown, gridRow: GridRow) => {
                        const { 0: id } = gridRow;
                        return <>{id}</>;
                      },
                    },
                    {
                      key: '1',
                      sortable: false,
                      filterable: false,
                      headerRenderer: () => <>{t('ospf.type')}</>,
                      renderer: (_value: unknown, gridRow: GridRow) => {
                        const { 1: a } = gridRow;
                        return <>{a.type ?? DEFAULT_AREA_TYPE}</>;
                      },
                    },
                    {
                      key: '2',
                      sortable: false,
                      filterable: false,
                      headerRenderer: () => <>{t('ospf.noSummary')}</>,
                      renderer: (_value: unknown, gridRow: GridRow) => {
                        const { 1: a } = gridRow;
                        return <>{yn(a.noSummary)}</>;
                      },
                    },
                  ]}
                  tableProps={{ size: 'small', 'aria-label': t('ospf.areas') }}
                  rowKey={(nativeRow, _gridIndex) => {
                    const gridRow = nativeRow as unknown as GridRow;
                    const { 0: id } = gridRow;
                    return id;
                  }}
                  rowProps={(nativeRow, _gridIndex) => {
                    const gridRow = nativeRow as unknown as GridRow;
                    const { 0: id } = gridRow;
                    return { role: 'row', 'data-testid': `ospf-area-${id}` };
                  }}
                  cellProps={(_nativeRow, column, _gridIndex) => {
                    switch (column.key) {
                      case '0':
                        return { dir: 'ltr' };
                      case '1':
                        return { dir: 'ltr' };
                      case '2':
                        return {};
                      default:
                        return {};
                    }
                  }}
                  emptyContent={null}
                />
              );
            })()
          )}
          <Typography component="h3" variant="h6" gutterBottom sx={{ mt: 3 }}>
            {t('ospf.interfaces')}
          </Typography>
          {ifaces.length === 0 ? (
            <Typography color="text.secondary">{t('ospf.empty')}</Typography>
          ) : (
            <Box sx={{ overflowX: 'auto' }}>
              {(() => {
                const gridItems = ifaces;
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
                        headerRenderer: () => <>{t('ospf.interface')}</>,
                        renderer: (_value: unknown, gridRow: GridRow) => {
                          const { 0: name } = gridRow;
                          return <>{interfaceLabel(name)}</>;
                        },
                      },
                      {
                        key: '1',
                        sortable: false,
                        filterable: false,
                        headerRenderer: () => <>{t('ospf.area')}</>,
                        renderer: (_value: unknown, gridRow: GridRow) => {
                          const { 1: i } = gridRow;
                          return <>{i.area}</>;
                        },
                      },
                      {
                        key: '2',
                        sortable: false,
                        filterable: false,
                        headerRenderer: () => <>{t('ospf.cost')}</>,
                        renderer: (_value: unknown, gridRow: GridRow) => {
                          const { 1: i } = gridRow;
                          return <>{i.cost ?? t('none')}</>;
                        },
                      },
                      {
                        key: '3',
                        sortable: false,
                        filterable: false,
                        headerRenderer: () => <>{t('ospf.passive')}</>,
                        renderer: (_value: unknown, gridRow: GridRow) => {
                          const { 1: i } = gridRow;
                          return <>{yn(i.passive)}</>;
                        },
                      },
                      ...(proto === 'ospf'
                        ? [
                            {
                              key: '4',
                              sortable: false,
                              filterable: false,
                              headerRenderer: () => <>{t('ospf.bfd')}</>,
                              renderer: (_value: unknown, gridRow: GridRow) => {
                                const { 1: i } = gridRow;
                                return <>{yn(i.bfd)}</>;
                              },
                            },
                          ]
                        : []),
                    ]}
                    tableProps={{
                      size: 'small',
                      'aria-label': t('ospf.interfaces'),
                    }}
                    rowKey={(nativeRow, _gridIndex) => {
                      const gridRow = nativeRow as unknown as GridRow;
                      const { 0: name } = gridRow;
                      return name;
                    }}
                    rowProps={(nativeRow, _gridIndex) => {
                      const gridRow = nativeRow as unknown as GridRow;
                      const { 0: name } = gridRow;
                      return { role: 'row', 'data-testid': `ospf-if-${name}` };
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
                          return {};
                        case '4':
                          return {};
                        default:
                          return {};
                      }
                    }}
                    emptyContent={null}
                  />
                );
              })()}
            </Box>
          )}
        </Paper>
      </Stack>
    </PageHeader>
  );
}
