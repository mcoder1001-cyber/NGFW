import Button from '@mui/material/Button';
import { Link as RouterLink } from 'react-router';
import Chip from '@mui/material/Chip';
import LinearProgress from '@mui/material/LinearProgress';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
import { useFormatters } from '@ngfw/ui-kit';
import { SchemaForm, type ProblemDetails } from '@ngfw/ui-kit/schema-form';
import { useTranslation } from 'react-i18next';
import { ApiError } from '../../../api-problem';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { domainSchemas } from '../../../schema/registry';
import { PageHeader } from '../../../shell/PageHeader';
import {
  useCandidateSystem,
  usePatchSystem,
  useRunningSystem,
  type SystemConfig,
  useSystemIdentityState,
} from './queries';

export const NS = 'system-identity';
const BASE = '/system';

/** Server problem with pointers made relative to the form's root (`/system/timezone` → `/timezone`). */
function problemUnder(error: unknown): ProblemDetails | null {
  if (!(error instanceof ApiError)) return null;
  const p = error.toFormProblem();
  return {
    ...p,
    errors: (p.errors ?? []).map((e) => ({
      ...e,
      pointer: e.pointer.startsWith(BASE) ? e.pointer.slice(BASE.length) : e.pointer,
    })),
  };
}

type Row = { key: string; candidate: string; running: string };

function rows(c: SystemConfig, r: SystemConfig, none: string): Row[] {
  const list = (v: string[] | undefined) => (v && v.length > 0 ? v.join(', ') : none);
  const text = (v: string | undefined) => (v ? v : none);
  const lines = (v: string | undefined) =>
    v ? v.split('\n')[0] + (v.includes('\n') ? ' …' : '') : none;
  return [
    { key: 'hostname', candidate: text(c.hostname), running: text(r.hostname) },
    { key: 'timezone', candidate: text(c.timezone), running: text(r.timezone) },
    { key: 'loginBanner', candidate: lines(c.banner?.login), running: lines(r.banner?.login) },
    { key: 'motd', candidate: lines(c.banner?.motd), running: lines(r.banner?.motd) },
    { key: 'servers', candidate: list(c.dns?.servers), running: list(r.dns?.servers) },
    {
      key: 'searchDomains',
      candidate: list(c.dns?.searchDomains),
      running: list(r.dns?.searchDomains),
    },
  ];
}

/**
 * System › Identity (F-system-identity, DEC-system-identity): the `system` domain — hostname, time zone, login/MOTD
 * banners and the router's own DNS client — as one schema-driven form, next to what the router runs now. Saving writes a
 * merge patch to the candidate; the pending-change bar commits it and the agent renders /etc/hostname, /etc/localtime,
 * /etc/issue, /etc/motd and the resolver drop-in.
 */
export function SystemIdentityPage() {
  const { t } = useTranslation([NS, 'config']);
  const perms = usePermissions();
  const fmt = useFormatters();
  const cand = useCandidateSystem();
  const running = useRunningSystem();
  const observed = useSystemIdentityState();
  const patch = usePatchSystem();
  const readOnly = !perms.editConfig;
  const none = t('none');

  return (
    <PageHeader title={t('title')}>
      <Button component={RouterLink} to="/system/setup">
        {t('setup:run')}
      </Button>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('intro')}
      </Typography>
      {(cand.isPending || running.isPending) && (
        <LinearProgress aria-label={t('config:loading')} sx={{ mb: 2 }} />
      )}
      {cand.isError && <ProblemAlert error={cand.error} sx={{ mb: 2 }} />}
      {running.isError && <ProblemAlert error={running.error} sx={{ mb: 2 }} />}
      <Paper
        component="section"
        aria-label={t('observed.title')}
        variant="outlined"
        sx={{ p: 2, mb: 2 }}
      >
        <Typography component="h3" variant="h6">
          {t('observed.title')}
        </Typography>
        {observed.isPending && <LinearProgress aria-label={t('config:loading')} />}
        {observed.isError && <ProblemAlert error={observed.error} />}
        {observed.isSuccess && !observed.data && (
          <Typography>{t('observed.unavailable')}</Typography>
        )}
        {observed.data && (
          <>
            <Typography>
              {t('observed.hostname')}: {observed.data.hostname || none}
            </Typography>
            <Typography>
              {t('observed.timezone')}: {observed.data.timezone || none}
            </Typography>
            <Typography>
              {t('observed.uptime')}:{' '}
              {observed.data.uptimeSeconds === undefined
                ? none
                : fmt.number(Math.floor(observed.data.uptimeSeconds))}
            </Typography>
            <Typography>
              {t('observed.resolver')}:{' '}
              {t(`observed.status.${observed.data.resolverStatus}`, {
                defaultValue: t('observed.unavailable'),
              })}
            </Typography>
            <Typography>
              {t('observed.servers')}: {observed.data.observedNameServers.join(', ') || none}
            </Typography>
            {observed.data.errors.length > 0 && (
              <Typography color="warning.main">{t('observed.partial')}</Typography>
            )}
            <Typography variant="body2" color="text.secondary">
              {t('observed.note')}
            </Typography>
          </>
        )}
      </Paper>
      <Stack direction={{ xs: 'column', lg: 'row' }} spacing={3} alignItems="flex-start">
        {cand.isSuccess && (
          <Paper variant="outlined" sx={{ p: 2, flex: 1, maxWidth: 640, width: '100%' }}>
            <SchemaForm
              id="system-identity-form"
              schema={domainSchemas.system}
              value={cand.data}
              readOnly={readOnly}
              i18nPrefix={`${NS}:field`}
              onSubmit={async (v) => {
                await patch.mutateAsync(v as Record<string, unknown>).catch(() => undefined);
              }}
              problem={patch.error ? problemUnder(patch.error) : null}
              submitLabel={t('save')}
            />
          </Paper>
        )}
        <Paper variant="outlined" sx={{ p: 2, flex: 1, width: '100%' }}>
          <Typography component="h3" variant="h6" gutterBottom>
            {t('live.title')}
          </Typography>
          <Table size="small" aria-label={t('live.title')}>
            <TableHead>
              <TableRow>
                <TableCell>{t('live.setting')}</TableCell>
                <TableCell>{t('live.candidate')}</TableCell>
                <TableCell>{t('live.running')}</TableCell>
                <TableCell />
              </TableRow>
            </TableHead>
            <TableBody>
              {rows(cand.data ?? {}, running.data ?? {}, none).map((r) => (
                <TableRow key={r.key} data-testid={`sys-${r.key}`}>
                  <TableCell>{t(`live.${r.key}`)}</TableCell>
                  <TableCell dir="ltr">{r.candidate}</TableCell>
                  <TableCell dir="ltr">{r.running}</TableCell>
                  <TableCell>
                    {running.isSuccess && r.candidate !== r.running && (
                      <Chip size="small" color="warning" label={t('live.pending')} />
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
            {t('live.note')}
          </Typography>
        </Paper>
      </Stack>
    </PageHeader>
  );
}
