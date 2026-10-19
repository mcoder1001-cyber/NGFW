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
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { useInterfaceLabel, StatusChip } from '@ngfw/ui-kit';
import { serviceText } from '../../../product-text';
import { useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { PageHeader } from '../../../shell/PageHeader';
import { useWanState, type WanGroupState } from './queries';

const UP = 'up' as const;
const DOWN = 'down' as const;
const WAN_KEY = ['state', 'wan'];

/** F-multiwan: `/routing/wan` — live WAN group member health. Groups are configured under Config → Routing. */
export function WanGroupsPage() {
  const { t } = useTranslation('multiwan');
  const qc = useQueryClient();
  const q = useWanState();
  const groups = q.data?.groups ?? [];
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
          onClick={() => void qc.invalidateQueries({ queryKey: WAN_KEY })}
        >
          {t('refresh')}
        </Button>
      </Stack>
      {q.isPending && <LinearProgress aria-label={t('loading')} sx={{ mb: 1 }} />}
      {q.isError && <ProblemAlert error={q.error} sx={{ mb: 1 }} />}
      {q.data?.agentError && (
        <Alert severity="info" sx={{ mb: 1 }}>
          {t('agentError', { error: serviceText(q.data.agentError) })}
        </Alert>
      )}
      {q.data && groups.length === 0 && !q.data.agentError && (
        <Alert severity="info">{t('empty')}</Alert>
      )}
      <Stack gap={3}>
        {groups.map((g) => (
          <WanGroup key={g.name} g={g} />
        ))}
      </Stack>
    </Box>
  );
}

function WanGroup({ g }: { g: WanGroupState }) {
  const interfaceLabel = useInterfaceLabel();
  const { t } = useTranslation('multiwan');
  return (
    <Box>
      <Stack direction="row" gap={1} alignItems="center" sx={{ mb: 1 }}>
        <Typography component="h3" variant="subtitle1">
          <bdi>{g.name}</bdi>
        </Typography>
        <Chip
          size="small"
          variant="outlined"
          label={t(`mode.${g.mode}`, { defaultValue: g.mode })}
        />
        {g.mode === 'failover' && g.active && (
          <Typography variant="body2" color="text.secondary">
            {t('activeMember', { iface: interfaceLabel(g.active) })}
          </Typography>
        )}
      </Stack>
      <TableContainer component={Paper} variant="outlined">
        {(() => {
          const gridItems = g.members;
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
                  headerRenderer: () => <>{t('col.interface')}</>,
                  renderer: (_value: unknown, gridRow: GridRow) => {
                    const m = gridRow;
                    return (
                      <>
                        <bdi>{m.interface}</bdi>
                        {g.mode === 'failover' && m.interface === g.active && (
                          <Chip
                            size="small"
                            color="success"
                            variant="outlined"
                            label={t('active')}
                            sx={{ marginInlineStart: 1 }}
                          />
                        )}
                      </>
                    );
                  },
                },
                {
                  key: '1',
                  sortable: false,
                  filterable: false,
                  headerRenderer: () => <>{t('col.health')}</>,
                  renderer: (_value: unknown, gridRow: GridRow) => {
                    const m = gridRow;
                    return (
                      <>
                        <StatusChip
                          size="small"
                          status={m.up ? UP : DOWN}
                          label={m.up ? t('up') : t('down')}
                        />
                      </>
                    );
                  },
                },
                {
                  key: '2',
                  sortable: false,
                  filterable: false,
                  headerRenderer: () => <>{t('col.loss')}</>,
                  renderer: (_value: unknown, gridRow: GridRow) => {
                    const m = gridRow;
                    return <>{m.lossPct}%</>;
                  },
                },
                {
                  key: '3',
                  sortable: false,
                  filterable: false,
                  headerRenderer: () => <>{t('col.latency')}</>,
                  renderer: (_value: unknown, gridRow: GridRow) => {
                    const m = gridRow;
                    return <>{m.latencyMs ? `${m.latencyMs} ms` : '—'}</>;
                  },
                },
                {
                  key: '4',
                  sortable: false,
                  filterable: false,
                  headerRenderer: () => <>{t('col.weight')}</>,
                  renderer: (_value: unknown, gridRow: GridRow) => {
                    const m = gridRow;
                    return <>{m.weight}</>;
                  },
                },
                {
                  key: '5',
                  sortable: false,
                  filterable: false,
                  headerRenderer: () => <>{t('col.priority')}</>,
                  renderer: (_value: unknown, gridRow: GridRow) => {
                    const m = gridRow;
                    return <>{m.priority}</>;
                  },
                },
              ]}
              tableProps={{
                size: 'small',
                'aria-label': t('membersOf', { name: g.name }),
              }}
              rowKey={(nativeRow, _gridIndex) => {
                const gridRow = nativeRow as unknown as GridRow;
                const m = gridRow;
                return m.interface;
              }}
              rowProps={(_nativeRow, _gridIndex) => {
                return {};
              }}
              cellProps={(_nativeRow, column, _gridIndex) => {
                switch (column.key) {
                  case '0':
                    return { dir: 'ltr' };
                  case '1':
                    return {};
                  case '2':
                    return { sx: { textAlign: 'end' } };
                  case '3':
                    return { sx: { textAlign: 'end' } };
                  case '4':
                    return { sx: { textAlign: 'end' } };
                  case '5':
                    return { sx: { textAlign: 'end' } };
                  default:
                    return {};
                }
              }}
              emptyContent={null}
            />
          );
        })()}
      </TableContainer>
    </Box>
  );
}
