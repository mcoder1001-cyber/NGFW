import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import Grid from '@mui/material/Grid';
import Paper from '@mui/material/Paper';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
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
            <Table size="small" aria-label={t('ntp.sources')}>
              <TableHead>
                <TableRow>
                  <TableCell>{t('col.source')}</TableCell>
                  <TableCell>{t('col.state')}</TableCell>
                  <TableCell>{t('col.stratum')}</TableCell>
                  <TableCell>{t('col.reach')}</TableCell>
                  <TableCell>{t('col.offset')}</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {(st?.sources ?? []).length === 0 && (
                  <TableRow>
                    <TableCell colSpan={5}>{t('ntp.noSources')}</TableCell>
                  </TableRow>
                )}
                {(st?.sources ?? []).map((s) => (
                  <TableRow key={s.name}>
                    <TableCell sx={{ fontFamily: 'monospace' }}>{s.name}</TableCell>
                    <TableCell>
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
                    </TableCell>
                    <TableCell>{s.stratum}</TableCell>
                    <TableCell sx={{ fontFamily: 'monospace' }}>{s.reach}</TableCell>
                    <TableCell>{seconds(s.offset)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        </Grid>
      </Grid>
    </Box>
  );
}
