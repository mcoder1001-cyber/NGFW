import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import LinearProgress from '@mui/material/LinearProgress';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
import { SchemaForm, type JsonSchema } from '@ngfw/ui-kit/schema-form';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { usePermissions } from '../../../auth/AuthProvider';
import { invalidateConfig, qk } from '../../../config/queries';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { domainSchemas } from '../../../schema/registry';
import type { NotificationsConfig } from '@ngfw/schema';

const PATH = 'management';
const NS = 'management';
export default function NotificationsTab() {
  const { t } = useTranslation(['management', 'config']);
  const perms = usePermissions();
  const qc = useQueryClient();
  const candidate = useQuery({
    queryKey: qk.candidate(PATH),
    queryFn: async ({ signal }) =>
      ((
        await call(
          api.GET('/api/v1/config/candidate/{path}', { params: { path: { path: PATH } }, signal }),
        )
      ).data ?? {}) as { notifications?: NotificationsConfig },
  });
  const state = useQuery({
    queryKey: ['state', 'management', 'notifications'],
    refetchInterval: 5000,
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/management/notifications', { signal }))).data,
  });
  const patch = useMutation({
    mutationFn: async (config: NotificationsConfig) =>
      call(
        api.PATCH('/api/v1/config/{path}', {
          params: { path: { path: PATH } },
          body: { notifications: config },
        }),
      ),
    onSettled: () => invalidateConfig(qc),
  });
  const test = useMutation({
    mutationFn: async (name: string) =>
      call(
        api.POST('/api/v1/actions/management/notifications/{name}/test', {
          params: { path: { name } },
        }),
      ),
    onSettled: () => qc.invalidateQueries({ queryKey: ['state', 'management', 'notifications'] }),
  });
  const schema = (domainSchemas.management as { properties?: { notifications?: JsonSchema } })
    .properties?.notifications;
  const config = candidate.data?.notifications ?? { channels: [], rules: [] };
  return (
    <Stack spacing={2}>
      <Typography variant="h6" component="h3">
        {t('notifications.title')}
      </Typography>
      <Typography>{t('notifications.intro')}</Typography>
      {candidate.isPending && <LinearProgress aria-label={t('config:loading')} />}
      {candidate.isError && <ProblemAlert error={candidate.error} />}
      {patch.isError && <ProblemAlert error={patch.error} />}
      {candidate.isSuccess && schema && (
        <Paper variant="outlined" sx={{ p: 2 }}>
          <SchemaForm
            id="notifications-form"
            schema={schema}
            value={config}
            readOnly={perms.role !== 'admin'}
            i18nPrefix={`${NS}:field.notifications`}
            onSubmit={async (v) => {
              await patch.mutateAsync(v as NotificationsConfig).catch(() => undefined);
            }}
          />
        </Paper>
      )}
      <Typography component="h3" variant="h6">
        {t('notifications.deliveryTitle')}
      </Typography>
      {state.isPending && <LinearProgress aria-label={t('config:loading')} />}
      {state.isError && <ProblemAlert error={state.error} />}
      {state.data?.error && (
        <Alert severity="error">
          {t('notifications.degraded')}: {state.data.error}
        </Alert>
      )}
      {state.data && (
        <Typography>{t('notifications.queue', { count: state.data.queued })}</Typography>
      )}
      <Typography variant="body2">{t('notifications.runningOnly')}</Typography>
      {config.channels.map((c) => (
        <Button
          key={c.name}
          disabled={perms.role !== 'admin' || test.isPending}
          onClick={() => test.mutate(c.name)}
        >
          {t('notifications.test', { name: c.name })}
        </Button>
      ))}
      {test.isError && <ProblemAlert error={test.error} />}
      {test.isSuccess && <Alert severity="info">{t('notifications.testQueued')}</Alert>}
      <Table aria-label={t('notifications.deliveryTitle')}>
        <TableBody>
          {state.data?.deliveries.map((d, i) => (
            <TableRow key={`${d.at}-${i}`}>
              <TableCell>{d.channel}</TableCell>
              <TableCell>{d.rule ?? t('notifications.testLabel')}</TableCell>
              <TableCell>{d.at}</TableCell>
              <TableCell>{t(`notifications.result.${d.result}`)}</TableCell>
              <TableCell>{d.error ?? '—'}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Stack>
  );
}
