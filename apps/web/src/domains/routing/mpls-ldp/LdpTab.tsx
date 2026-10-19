import { PgButton as Button } from '@amnafzar/common-ui-lib/forms';
import { ReactGeereedMaterial } from '@amnafzar/common-ui-lib/geereed';
import { PgRefreshIcon as RefreshIcon } from '@amnafzar/common-ui-lib/icons';
import {
  PgAlert as Alert,
  PgBox as Box,
  PgChip as Chip,
  PgLinearProgress as LinearProgress,
  PgPaper as Paper,
  PgStack as Stack,
  PgTableContainer as TableContainer,
  PgTablePagination as TablePagination,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { serviceText } from '../../../product-text';
import { ldpKeys, useLdpBindings, useLdpNeighbors, useLdpSync } from './queries';

const NS = 'mpls-ldp';

/** F-mpls-ldp: the LDP tab of the Routing → MPLS page — live neighbours, LIB bindings and the FRR→VPP sync status. */
export function LdpTab() {
  const { t } = useTranslation(NS);
  const qc = useQueryClient();
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(50);
  const neighbors = useLdpNeighbors();
  const bindings = useLdpBindings(page + 1, pageSize);
  const sync = useLdpSync();
  const agentError = neighbors.data?.agentError ?? sync.data?.agentError ?? null;
  const s = sync.data;

  return (
    <Box>
      <Stack direction="row" alignItems="center" gap={1} sx={{ mb: 2 }}>
        <Typography variant="body2" color="text.secondary" sx={{ flex: 1, maxInlineSize: 800 }}>
          {t('intro')}
        </Typography>
        <Button
          startIcon={<RefreshIcon />}
          onClick={() => void qc.invalidateQueries({ queryKey: ldpKeys.all })}
        >
          {t('refresh')}
        </Button>
      </Stack>

      {(neighbors.isPending || sync.isPending) && (
        <LinearProgress aria-label={t('loading')} sx={{ mb: 1 }} />
      )}
      {neighbors.isError && <ProblemAlert error={neighbors.error} sx={{ mb: 1 }} />}
      {agentError && (
        <Alert severity="info" sx={{ mb: 2 }}>
          {t('agentError', { error: serviceText(agentError) })}
        </Alert>
      )}

      {s && !agentError && (
        <Paper variant="outlined" sx={{ p: 2, mb: 3, maxInlineSize: 800 }}>
          <Stack direction="row" gap={1} alignItems="center" flexWrap="wrap">
            <Typography component="h3" variant="subtitle2" sx={{ flex: 1 }}>
              {t('sync.title')}
            </Typography>
            <Chip
              size="small"
              color={s.lastError ? 'error' : s.source ? 'success' : 'default'}
              label={s.lastError ? t('sync.error') : s.source ? t('sync.ok') : t('sync.idle')}
            />
          </Stack>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
            {t('sync.detail', {
              installed: s.installed,
              conflicts: s.conflicts,
              source: serviceText(s.source || '—'),
            })}
          </Typography>
          {s.lastError && (
            <Typography variant="body2" color="error" sx={{ mt: 0.5 }}>
              {serviceText(s.lastError)}
            </Typography>
          )}
        </Paper>
      )}

      <Typography component="h3" variant="subtitle1" sx={{ mb: 1 }}>
        {t('neighbors.title')}
      </Typography>
      {(neighbors.data?.neighbors ?? []).length === 0 ? (
        <Alert severity="info" sx={{ mb: 3 }}>
          {t('neighbors.empty')}
        </Alert>
      ) : (
        <TableContainer component={Paper} variant="outlined" sx={{ mb: 3 }}>
          {(() => {
            const gridItems = neighbors.data?.neighbors ?? [];
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
                    headerRenderer: () => <>{t('neighbors.col.lsrId')}</>,
                    renderer: (_value: unknown, gridRow: GridRow) => {
                      const n = gridRow;
                      return <>{n.lsrId}</>;
                    },
                  },
                  {
                    key: '1',
                    sortable: false,
                    filterable: false,
                    headerRenderer: () => <>{t('neighbors.col.address')}</>,
                    renderer: (_value: unknown, gridRow: GridRow) => {
                      const n = gridRow;
                      return <>{n.address}</>;
                    },
                  },
                  {
                    key: '2',
                    sortable: false,
                    filterable: false,
                    headerRenderer: () => <>{t('neighbors.col.state')}</>,
                    renderer: (_value: unknown, gridRow: GridRow) => {
                      const n = gridRow;
                      return (
                        <>
                          <Chip size="small" variant="outlined" label={n.state} />
                        </>
                      );
                    },
                  },
                  {
                    key: '3',
                    sortable: false,
                    filterable: false,
                    headerRenderer: () => <>{t('neighbors.col.uptime')}</>,
                    renderer: (_value: unknown, gridRow: GridRow) => {
                      const n = gridRow;
                      return <>{n.uptimeSec}</>;
                    },
                  },
                ]}
                tableProps={{
                  size: 'small',
                  'aria-label': t('neighbors.title'),
                }}
                rowKey={(nativeRow, _gridIndex) => {
                  const gridRow = nativeRow as unknown as GridRow;
                  const n = gridRow;
                  return n.lsrId;
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
                      return {};
                    case '3':
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

      <Typography component="h3" variant="subtitle1" sx={{ mb: 1 }}>
        {t('bindings.title')}
      </Typography>
      {(bindings.data?.total ?? 0) === 0 ? (
        <Alert severity="info">{t('bindings.empty')}</Alert>
      ) : (
        <TableContainer component={Paper} variant="outlined">
          {(() => {
            const gridItems = bindings.data?.bindings ?? [];
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
                    headerRenderer: () => <>{t('bindings.col.prefix')}</>,
                    renderer: (_value: unknown, gridRow: GridRow) => {
                      const b = gridRow;
                      return <>{b.prefix}</>;
                    },
                  },
                  {
                    key: '1',
                    sortable: false,
                    filterable: false,
                    headerRenderer: () => <>{t('bindings.col.localLabel')}</>,
                    renderer: (_value: unknown, gridRow: GridRow) => {
                      const b = gridRow;
                      return <>{b.localLabel}</>;
                    },
                  },
                  {
                    key: '2',
                    sortable: false,
                    filterable: false,
                    headerRenderer: () => <>{t('bindings.col.peer')}</>,
                    renderer: (_value: unknown, gridRow: GridRow) => {
                      const b = gridRow;
                      return <>{b.peer}</>;
                    },
                  },
                  {
                    key: '3',
                    sortable: false,
                    filterable: false,
                    headerRenderer: () => <>{t('bindings.col.remoteLabel')}</>,
                    renderer: (_value: unknown, gridRow: GridRow) => {
                      const b = gridRow;
                      return <>{b.remoteLabel}</>;
                    },
                  },
                  {
                    key: '4',
                    sortable: false,
                    filterable: false,
                    headerRenderer: () => <>{t('bindings.col.inUse')}</>,
                    renderer: (_value: unknown, gridRow: GridRow) => {
                      const b = gridRow;
                      return <>{b.inUse ? t('bindings.yes') : t('bindings.no')}</>;
                    },
                  },
                ]}
                tableProps={{
                  size: 'small',
                  'aria-label': t('bindings.title'),
                }}
                rowKey={(nativeRow, gridIndex) => {
                  const gridRow = nativeRow as unknown as GridRow;
                  const b = gridRow;
                  const i = gridIndex;
                  return `${b.prefix}-${b.localLabel}-${i}`;
                }}
                rowProps={(_nativeRow, _gridIndex) => {
                  return {};
                }}
                cellProps={(_nativeRow, column, _gridIndex) => {
                  switch (column.key) {
                    case '0':
                      return { dir: 'ltr' };
                    case '1':
                      return { dir: 'ltr', sx: { textAlign: 'end' } };
                    case '2':
                      return { dir: 'ltr' };
                    case '3':
                      return { dir: 'ltr', sx: { textAlign: 'end' } };
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
          <TablePagination
            component="div"
            count={bindings.data?.total ?? 0}
            page={page}
            onPageChange={(_e, p) => setPage(p)}
            rowsPerPage={pageSize}
            onRowsPerPageChange={(e) => {
              setPageSize(Number(e.target.value));
              setPage(0);
            }}
            rowsPerPageOptions={[50, 100, 200]}
          />
        </TableContainer>
      )}
    </Box>
  );
}
