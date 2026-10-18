import { ReactGeereedMaterial } from '@amnafzar/common-ui-lib/geereed';
import {
  PgBox as Box,
  PgChip as Chip,
  PgGrid as Grid,
  PgPaper as Paper,
  PgTableContainer as TableContainer,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { DaemonStatus, PendingActions, StateProblem } from './common';
import { fieldKey, NS, ntpSchema, problemUnder, seconds, SOURCE_STATES } from './model';
import {
  replacePatch,
  useCandidateNode,
  useNtpState,
  usePatchDomain,
  type ServicesNtpConfig,
} from './queries';

const DAEMON = 'chronyd';
const NTP_POINTER = '/services/ntp';

/**
 * Services › NTP (F-unbound-chrony-syslog; D-050: NTP lives only in `services.ntp`): the chrony client/server form
 * (schema-driven) and the live synchronisation status (`chronyc -c tracking | sources`), with the pending
 * start/restart request when chrony.conf changed (chrony reloads only its sources and keys at run time).
 */
export default function NtpTab() {
  const { t } = useTranslation([NS, 'config']);
  const perms = usePermissions();
  const cand = useCandidateNode<ServicesNtpConfig>('services', 'ntp');
  const state = useNtpState();
  const put = usePatchDomain('services');
  const schema = useMemo(() => ntpSchema(), []);
  const st = state.data;
  const tr = st?.tracking;

  return (
    <Box>
      <DaemonStatus
        daemon={DAEMON}
        running={st?.running}
        query={state}
        configPath={st?.configPath}
      />
      <StateProblem error={state.error} partial={st?.error} />
      <PendingActions actions={st?.pendingActions} />
      <Grid container spacing={2}>
        <Grid size={{ xs: 12, md: 6 }}>
          <Paper variant="outlined" sx={{ p: 2 }}>
            <Typography variant="h6" gutterBottom>
              {t('ntp.config')}
            </Typography>
            {cand.data && (
              <SchemaForm
                id="ntp-form"
                schema={schema}
                value={cand.data}
                readOnly={!perms.editConfig}
                onSubmit={async (value) => {
                  await put
                    .mutateAsync({ ntp: replacePatch(cand.data, value) })
                    .catch(() => undefined);
                }}
                problem={problemUnder(put.error, NTP_POINTER)}
                translateLabel={(p, f) => t(fieldKey('ntp', p), { defaultValue: f })}
                submitLabel={t('saveToCandidate')}
              />
            )}
          </Paper>
        </Grid>
        <Grid size={{ xs: 12, md: 6 }}>
          <Paper variant="outlined" sx={{ p: 2, mb: 2 }}>
            <Typography variant="h6" gutterBottom>
              {t('ntp.sync')}
            </Typography>
            {tr ? (
              <Typography variant="body2" component="div">
                {t('ntp.tracking', {
                  ref: tr.refName || tr.refId,
                  stratum: tr.stratum,
                  offset: seconds(tr.systemTime),
                  leap: tr.leap,
                })}
              </Typography>
            ) : (
              <Typography variant="body2" color="text.secondary">
                {t('ntp.noTracking')}
              </Typography>
            )}
          </Paper>
          <TableContainer component={Paper} variant="outlined">
            {(() => {
              const nativeRows = (st?.sources ?? []).map((s) => {
                return { s };
              });
              return (
                <ReactGeereedMaterial
                  footer={false}
                  hideFilterBuilderComponent
                  items={nativeRows}
                  columns={[
                    {
                      key: 'source',
                      headerRenderer: () => <>{t('col.source')}</>,
                      sortable: false,
                      filterable: false,
                      renderer: (_value: unknown, { s }: (typeof nativeRows)[number]) => (
                        <>{s.name}</>
                      ),
                    },
                    {
                      key: 'state',
                      headerRenderer: () => <>{t('col.state')}</>,
                      sortable: false,
                      filterable: false,
                      renderer: (_value: unknown, { s }: (typeof nativeRows)[number]) => (
                        <>
                          <Chip
                            size="small"
                            color={
                              s.state === '*'
                                ? 'success'
                                : s.state === '+'
                                  ? 'info'
                                  : s.state === 'x' || s.state === '?'
                                    ? 'warning'
                                    : 'default'
                            }
                            label={t(`ntp.sourceState.${SOURCE_STATES[s.state] ?? 'unknown'}`)}
                          />
                        </>
                      ),
                    },
                    {
                      key: 'stratum',
                      headerRenderer: () => <>{t('col.stratum')}</>,
                      sortable: false,
                      filterable: false,
                      renderer: (_value: unknown, { s }: (typeof nativeRows)[number]) => (
                        <>{s.stratum}</>
                      ),
                    },
                    {
                      key: 'reach',
                      headerRenderer: () => <>{t('col.reach')}</>,
                      sortable: false,
                      filterable: false,
                      renderer: (_value: unknown, { s }: (typeof nativeRows)[number]) => (
                        <>{s.reach}</>
                      ),
                    },
                    {
                      key: 'offset',
                      headerRenderer: () => <>{t('col.offset')}</>,
                      sortable: false,
                      filterable: false,
                      renderer: (_value: unknown, { s }: (typeof nativeRows)[number]) => (
                        <>{seconds(s.offset)}</>
                      ),
                    },
                  ]}
                  rowKey={(item) => {
                    const { s } = item as (typeof nativeRows)[number];
                    return s.name;
                  }}
                  cellProps={(_item, column) => {
                    switch (column.key) {
                      case 'source':
                        return { sx: { fontFamily: 'monospace' } };
                      case 'reach':
                        return { sx: { fontFamily: 'monospace' } };
                      default:
                        return {};
                    }
                  }}
                  tableProps={{ size: 'small', 'aria-label': t('ntp.sources') }}
                  emptyContent={<>{t('ntp.noSources')}</>}
                  noFormWrapper
                  embedded
                  memoEnable={false}
                  hidePagination
                  hideToolbar
                  search={false}
                  exportToCSV={false}
                  sortable={false}
                />
              );
            })()}
          </TableContainer>
        </Grid>
      </Grid>
    </Box>
  );
}
