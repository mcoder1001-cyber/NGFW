import { PgButton as Button } from '@amnafzar/common-ui-lib/forms';
import { ReactGeereedMaterial } from '@amnafzar/common-ui-lib/geereed';
import {
  PgAlert as Alert,
  PgLinearProgress as LinearProgress,
  PgPaper as Paper,
  PgStack as Stack,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import type { NotificationsConfig } from '@ngfw/schema';
import { useFormatters } from '@ngfw/ui-kit';
import { SchemaForm, type JsonSchema, type ProblemDetails } from '@ngfw/ui-kit/schema-form';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '../../../api';
import { ApiError, call } from '../../../api-problem';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { invalidateConfig, qk } from '../../../config/queries';
import { domainSchemas } from '../../../schema/registry';

const PATH = 'management';
const NS = 'management';
export function notificationProblem(error: unknown): ProblemDetails | null {
  if (!(error instanceof ApiError)) return null;
  const problem = error.toFormProblem();
  const base = '/management/notifications';
  const relative = (pointer: string) =>
    pointer === base ? '/' : pointer.startsWith(`${base}/`) ? pointer.slice(base.length) : pointer;
  return {
    ...problem,
    ...(typeof problem.pointer === 'string' ? { pointer: relative(problem.pointer) } : {}),
    errors: (problem.errors ?? []).map((e) => ({ ...e, pointer: relative(e.pointer) })),
  };
}

export default function NotificationsTab() {
  const { t } = useTranslation(['management', 'config']);
  const perms = usePermissions();
  const fmt = useFormatters();
  const qc = useQueryClient();
  const candidate = useQuery({
    // Other management observers cache projections (users/alarms), not this document.
    queryKey: [...qk.candidate(PATH), 'notifications'],
    queryFn: async ({ signal }) =>
      ((
        await call(
          api.GET('/api/v1/config/candidate/{path}', { params: { path: { path: PATH } }, signal }),
        )
      ).data ?? {}) as { notifications?: NotificationsConfig },
  });
  const running = useQuery({
    queryKey: ['config', 'running', PATH, 'notifications'],
    queryFn: async ({ signal }) =>
      ((
        await call(
          api.GET('/api/v1/config/{path}', {
            params: { path: { path: PATH } },
            signal,
          }),
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
      {candidate.isPending && <LinearProgress aria-label={t('notifications.loadingCandidate')} />}
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
            problem={notificationProblem(patch.error)}
            onSubmit={async (v) => {
              await patch.mutateAsync(v as NotificationsConfig).catch(() => undefined);
            }}
          />
        </Paper>
      )}
      <Typography component="h3" variant="h6">
        {t('notifications.deliveryTitle')}
      </Typography>
      {state.isPending && <LinearProgress aria-label={t('notifications.loadingState')} />}
      {state.isError && <ProblemAlert error={state.error} />}
      {state.data?.error && (
        <Alert severity="error">
          {t('notifications.degraded')}: {state.data.error}
        </Alert>
      )}
      {state.data && (
        <Typography>
          {t('notifications.queue', { count: fmt.number(state.data.queued) })}
        </Typography>
      )}
      <Typography variant="body2">{t('notifications.runningOnly')}</Typography>
      {running.isPending && <LinearProgress aria-label={t('notifications.loadingRunning')} />}
      {running.isError && <ProblemAlert error={running.error} />}
      {(running.data?.notifications?.channels ?? [])
        .filter((c) => c.enabled)
        .map((c) => (
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
      <ReactGeereedMaterial
        tableProps={{ 'aria-label': t('notifications.deliveryTitle') }}
        columns={[
          {
            key: 'column0',
            headerRenderer: () => t('notifications.columns.channel'),
            filterable: false,
            renderer: (value) => <>{value}</>,
          },
          {
            key: 'column1',
            headerRenderer: () => t('notifications.columns.rule'),
            filterable: false,
            renderer: (value) => <>{value}</>,
          },
          {
            key: 'column2',
            headerRenderer: () => t('notifications.columns.at'),
            filterable: false,
            renderer: (value) => <>{value}</>,
          },
          {
            key: 'column3',
            headerRenderer: () => t('notifications.columns.result'),
            filterable: false,
            renderer: (value) => <>{value}</>,
          },
          {
            key: 'column4',
            headerRenderer: () => t('notifications.columns.error'),
            filterable: false,
            renderer: (value) => <>{value}</>,
          },
        ]}
        items={[
          ...(state.data?.deliveries.map((d, i) => ({
            id: `${d.at}-${i}`,
            column0: d.channel,
            column1: d.rule ?? t('notifications.testLabel'),
            column2: fmt.dateTime(d.at),
            column3: t(`notifications.result.${d.result}`),
            column4: d.error ?? '—',
          })) ?? []),
        ]}
        rowKey={(item) => item.id}
        rowProps={(item) => item.rowProps ?? {}}
        cellProps={(item, column) => item.cellProps?.[column.key] ?? {}}
        emptyContent={
          state.isSuccess && state.data?.deliveries.length === 0 ? t('notifications.empty') : null
        }
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
    </Stack>
  );
}
