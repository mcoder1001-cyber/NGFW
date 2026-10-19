import { PgButton as Button } from '@amnafzar/common-ui-lib/forms';
import {
  ReactGeereedMaterial,
  type GeereedTablePassthrough,
} from '@amnafzar/common-ui-lib/geereed';
import { PgRefreshIcon as RefreshIcon } from '@amnafzar/common-ui-lib/icons';
import {
  PgAlert as Alert,
  PgBox as Box,
  PgChip as Chip,
  PgLinearProgress as LinearProgress,
  PgPaper as Paper,
  PgStack as Stack,
  PgTab as Tab,
  PgTableContainer as TableContainer,
  PgTabs as Tabs,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { useFormatters } from '@ngfw/ui-kit';
import { useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { serviceText } from '../../../product-text';
import { PageHeader } from '../../../shell/PageHeader';
import { useAckAlarm, useAlarms, type Alarm } from './queries';

const ALARMS_KEY = ['state', 'alarms'];

const SEV_COLOR: Record<string, 'error' | 'warning' | 'info' | 'default'> = {
  critical: 'error',
  warning: 'warning',
  info: 'info',
};

type TabKey = 'active' | 'all';
const ACTIVE: TabKey = 'active';
const ALL: TabKey = 'all';
function isTab(v: string | null): v is TabKey {
  return v === 'active' || v === 'all';
}

/** F-dashboard-prom-alarms: the alarms list with acknowledge. Rules and targets are edited under Config (management). */
export function AlarmsPage() {
  const { t } = useTranslation('dashboard-prom-alarms');
  const fmt = useFormatters();
  const perms = usePermissions();
  const qc = useQueryClient();
  const [params, setParams] = useSearchParams();
  const tab: TabKey = isTab(params.get('tab')) ? (params.get('tab') as TabKey) : 'active';
  const q = useAlarms(tab === 'active' ? true : undefined);
  const ack = useAckAlarm();
  const [error, setError] = useState<unknown>(null);

  const doAck = async (id: number) => {
    setError(null);
    try {
      await ack.mutateAsync(id);
    } catch (e) {
      setError(e);
    }
  };

  return (
    <Box>
      <PageHeader title={t('title')}>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2, maxInlineSize: 900 }}>
          {t('intro')}
        </Typography>
      </PageHeader>
      <Stack direction="row" alignItems="center" sx={{ mb: 1 }}>
        <Tabs
          value={tab}
          onChange={(_e, v: TabKey) => setParams({ tab: v })}
          sx={{ flex: 1 }}
          aria-label={t('tabsLabel')}
        >
          <Tab value={ACTIVE} label={t('tabs.active')} />
          <Tab value={ALL} label={t('tabs.all')} />
        </Tabs>
        <Button
          startIcon={<RefreshIcon />}
          onClick={() => void qc.invalidateQueries({ queryKey: ALARMS_KEY })}
        >
          {t('refresh')}
        </Button>
      </Stack>
      {q.isPending && <LinearProgress aria-label={t('loading')} sx={{ mb: 1 }} />}
      {q.isError && <ProblemAlert error={q.error} sx={{ mb: 1 }} />}
      {error !== null && <ProblemAlert error={error} sx={{ mb: 1 }} />}
      {q.data && q.data.items.length === 0 && <Alert severity="success">{t('empty')}</Alert>}
      {q.data && q.data.items.length > 0 && (
        <TableContainer component={Paper} variant="outlined">
          <ReactGeereedMaterial
            tableProps={{ size: 'small', 'aria-label': t('title') }}
            columns={[
              {
                key: 'severity',
                headerRenderer: () => t('col.severity'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'rule',
                headerRenderer: () => t('col.rule'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'message',
                headerRenderer: () => t('col.message'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'raised',
                headerRenderer: () => t('col.raised'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'state',
                headerRenderer: () => t('col.state'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'ack',
                headerRenderer: () => t('col.ack'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'column6',
                headerRenderer: () => null,
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
            ]}
            items={[
              ...(q.data.items.map((a: Alarm) => ({
                id: a.id,
                severity: (
                  <>
                    <Chip
                      size="small"
                      color={SEV_COLOR[a.severity] ?? 'default'}
                      variant="outlined"
                      label={t(`severity.${a.severity}`, { defaultValue: a.severity })}
                    />
                  </>
                ),
                rule: (
                  <>
                    <bdi>{a.rule}</bdi>
                    {a.instance && (
                      <Typography
                        variant="caption"
                        color="text.secondary"
                        sx={{ display: 'block' }}
                        dir="ltr"
                      >
                        {a.instance}
                      </Typography>
                    )}
                  </>
                ),
                message: serviceText(a.message),
                raised: fmt.dateTime(new Date(a.raisedAt)),
                state: (
                  <>
                    <Chip
                      size="small"
                      variant="outlined"
                      color={a.state === 'active' ? 'warning' : 'default'}
                      label={t(`state.${a.state}`, { defaultValue: a.state })}
                    />
                  </>
                ),
                ack: a.ackedBy ? <bdi>{a.ackedBy}</bdi> : '—',
                column6: a.ackedAt === null && (
                  <Button
                    size="small"
                    disabled={!perms.editConfig || ack.isPending}
                    onClick={() => void doAck(a.id)}
                  >
                    {t('ack')}
                  </Button>
                ),
                cellProps: { raised: { dir: 'ltr' } } satisfies Record<
                  string,
                  NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
                >,
              })) ?? []),
            ]}
            rowKey={(item) => item.id}
            rowProps={(item) => item.rowProps ?? {}}
            cellProps={(item, column) => item.cellProps?.[column.key] ?? {}}
            emptyContent={null}
            noFormWrapper
            embedded
            hideToolbar
            hidePagination
            hideFilterBuilderComponent
            search={false}
            sortable={false}
            exportToCSV={false}
            footer={false}
            memoEnable={false}
          />
        </TableContainer>
      )}
    </Box>
  );
}
