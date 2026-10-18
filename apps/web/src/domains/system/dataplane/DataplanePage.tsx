import { PgButton as Button } from '@amnafzar/common-ui-lib/forms';
import {
  ReactGeereedMaterial,
  type GeereedTablePassthrough,
} from '@amnafzar/common-ui-lib/geereed';
import {
  PgAlert as Alert,
  PgChip as Chip,
  PgDialog as Dialog,
  PgDialogActions as DialogActions,
  PgDialogContent as DialogContent,
  PgDialogTitle as DialogTitle,
  PgLinearProgress as LinearProgress,
  PgPaper as Paper,
  PgStack as Stack,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { useProductWording } from '@ngfw/ui-kit';
import { SchemaForm, type ProblemDetails } from '@ngfw/ui-kit/schema-form';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiError } from '../../../api-problem';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { domainSchemas } from '../../../schema/registry';
import { PageHeader } from '../../../shell/PageHeader';
import { ApplyDataplane } from './ApplyDataplane';
import {
  useCandidateDataplane,
  useDataplaneState,
  usePatchDataplane,
  usePreviewDataplane,
  useRunningDataplane,
  type DataplaneConfig,
} from './queries';

export const NS = 'dataplane';
const BASE = '/dataplane';
const RUNTIME_FIELDS = ['loadedPlugins', 'nicQueues', 'runtimeMemory'] as const;

/** Server problem with pointers made relative to the form's root (`/dataplane/corelist` → `/corelist`). */
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

function switches(sw: Record<string, boolean>, on: string, off: string): string {
  return Object.entries(sw)
    .map(([f, v]) => `${f}: ${v ? on : off}`)
    .join(', ');
}

function rows(
  c: DataplaneConfig,
  r: DataplaneConfig,
  none: string,
  on: string,
  off: string,
): Row[] {
  const num = (v: number | undefined) => (v === undefined ? none : String(v));
  const list = (v: (string | number)[] | undefined) => (v && v.length > 0 ? v.join(', ') : none);
  const plugins = (v: DataplaneConfig['plugins']) =>
    v === undefined ? none : switches(v.switches ?? {}, on, off) || '{}';
  const devices = (v: DataplaneConfig['devices']) =>
    list(Object.entries(v ?? {}).map(([pci, d]) => (d.name ? `${pci}=${d.name}` : pci)));
  return [
    { key: 'workers', candidate: num(c.workers), running: num(r.workers) },
    { key: 'corelist', candidate: list(c.corelist), running: list(r.corelist) },
    { key: 'mainCore', candidate: num(c.mainCore), running: num(r.mainCore) },
    {
      key: 'queues',
      candidate: `${num(c.rxQueues)} / ${num(c.txQueues)}`,
      running: `${num(r.rxQueues)} / ${num(r.txQueues)}`,
    },
    { key: 'devices', candidate: devices(c.devices), running: devices(r.devices) },
    { key: 'hugepagesGb', candidate: num(c.hugepagesGb), running: num(r.hugepagesGb) },
    { key: 'buffersPerNuma', candidate: num(c.buffersPerNuma), running: num(r.buffersPerNuma) },
    { key: 'plugins', candidate: plugins(c.plugins), running: plugins(r.plugins) },
  ];
}

const gib = (bytes: string | undefined) => {
  const n = Number(bytes ?? '0');
  return `${(n / 1024 ** 3).toFixed(1)} GiB`;
};

/**
 * System › Dataplane (F-dataplane-ui, D-152): the `dataplane` domain — VPP workers/cores, DPDK NICs and queues,
 * hugepages/buffers, plugin switches — as a schema-driven form next to the running configuration and the installed
 * startup.conf. These settings only take effect after a VPP restart: the page stages and previews (read-only render +
 * diff by the agent); installing the file (apply-startup.sh) is disabled until TD-17's approval gate.
 */
export function DataplanePage() {
  const { t } = useTranslation([NS, 'config']);
  const wording = useProductWording();
  const perms = usePermissions();
  const cand = useCandidateDataplane();
  const running = useRunningDataplane();
  const state = useDataplaneState();
  const patch = usePatchDataplane();
  const preview = usePreviewDataplane();
  const [open, setOpen] = useState(false);
  const readOnly = !perms.editConfig;
  const none = t('none');
  const st = state.data;

  return (
    <PageHeader title={t('title')}>
      <Alert severity="warning" sx={{ mb: 2 }} data-testid="dp-restart-banner">
        {t('restartBanner')}
      </Alert>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('intro')}
      </Typography>
      {(cand.isPending || running.isPending) && (
        <LinearProgress aria-label={t('config:loading')} sx={{ mb: 2 }} />
      )}
      {cand.isError && <ProblemAlert error={cand.error} sx={{ mb: 2 }} />}
      {running.isError && <ProblemAlert error={running.error} sx={{ mb: 2 }} />}
      <Stack
        direction="row"
        spacing={2}
        sx={{ mb: 2 }}
        alignItems="center"
        flexWrap="wrap"
        useFlexGap
      >
        <Button
          variant="outlined"
          onClick={() => {
            setOpen(true);
            preview.mutate();
          }}
        >
          {t('preview.button')}
        </Button>
        <ApplyDataplane />
      </Stack>
      <Stack direction={{ xs: 'column', lg: 'row' }} spacing={3} alignItems="flex-start">
        {cand.isSuccess && (
          <Paper variant="outlined" sx={{ p: 2, flex: 1, maxWidth: 640, width: '100%' }}>
            <SchemaForm
              id="dataplane-form"
              schema={domainSchemas.dataplane}
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
        <Stack spacing={3} sx={{ flex: 1, width: '100%' }}>
          <Paper variant="outlined" sx={{ p: 2 }} data-testid="dp-runtime">
            <Typography component="h3" variant="h6">
              {t('runtime.title')}
            </Typography>
            {state.isError && <ProblemAlert error={state.error} />}
            {st && (
              <>
                {st.runtimeErrors?.length > 0 && (
                  <Alert severity="warning">
                    {t('runtime.unavailable')}: {st.runtimeErrors.join(', ')}
                  </Alert>
                )}
                <Typography component="pre" sx={{ whiteSpace: 'pre-wrap' }} dir="ltr">
                  {st.runtimeThreads
                    ?.map((th) =>
                      t('runtime.thread', {
                        name: th.name,
                        type: th.type,
                        cpu: th.cpuId,
                        core: th.core,
                        numa: th.numaSocket,
                      }),
                    )
                    .join('\n') || none}
                </Typography>
                {RUNTIME_FIELDS.map((key) => (
                  <Stack key={key}>
                    <Typography variant="subtitle2">{t(`runtime.${key}`)}</Typography>
                    <Typography
                      component="pre"
                      dir="ltr"
                      sx={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}
                    >
                      {st[key] || none}
                    </Typography>
                  </Stack>
                ))}
              </>
            )}
          </Paper>
          <Paper variant="outlined" sx={{ p: 2 }}>
            <Typography component="h3" variant="h6" gutterBottom>
              {t('live.title')}
            </Typography>
            <ReactGeereedMaterial
              tableProps={{ size: 'small', 'aria-label': t('live.title') }}
              columns={[
                {
                  key: 'column0',
                  headerRenderer: () => t('live.setting'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
                {
                  key: 'column1',
                  headerRenderer: () => t('live.candidate'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
                {
                  key: 'column2',
                  headerRenderer: () => t('live.running'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
                {
                  key: 'column3',
                  headerRenderer: () => null,
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
              ]}
              items={[
                ...(rows(
                  cand.data ?? {},
                  running.data ?? {},
                  none,
                  t('switch.on'),
                  t('switch.off'),
                ).map((r) => ({
                  id: r.key,
                  column0: t(`live.${r.key}`),
                  column1: r.candidate,
                  column2: r.running,
                  column3: running.isSuccess && r.candidate !== r.running && (
                    <Chip size="small" color="warning" label={t('live.pending')} />
                  ),
                  rowProps: { 'data-testid': `dp-${r.key}` } satisfies ReturnType<
                    NonNullable<GeereedTablePassthrough['rowProps']>
                  > &
                    Record<`data-${string}`, unknown>,
                  cellProps: { column1: { dir: 'ltr' }, column2: { dir: 'ltr' } } satisfies Record<
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
            <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
              {t('live.note')}
            </Typography>
          </Paper>
          <Paper variant="outlined" sx={{ p: 2 }} data-testid="dp-installed">
            <Typography component="h3" variant="h6" gutterBottom>
              {t('installed.title')}
            </Typography>
            {state.isError && <ProblemAlert error={state.error} />}
            {st && (
              <ReactGeereedMaterial
                tableProps={{ size: 'small', 'aria-label': t('installed.title') }}
                columns={[
                  { key: 'label', filterable: false, renderer: (value) => <>{value}</> },
                  { key: 'value', filterable: false, renderer: (value) => <>{value}</> },
                ]}
                items={[
                  {
                    id: 0,
                    label: t('installed.file'),
                    value: t(st.startupPresent ? 'installed.present' : 'installed.missing'),
                    cellProps: { value: { dir: 'ltr' } } satisfies Record<
                      string,
                      NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
                    >,
                  },
                  {
                    id: 1,
                    label: t('live.workers'),
                    value: st.workers ?? none,
                    cellProps: { value: { dir: 'ltr' } } satisfies Record<
                      string,
                      NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
                    >,
                  },
                  {
                    id: 2,
                    label: t('live.corelist'),
                    value: st.corelistWorkers || none,
                    cellProps: { value: { dir: 'ltr' } } satisfies Record<
                      string,
                      NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
                    >,
                  },
                  {
                    id: 3,
                    label: t('live.mainCore'),
                    value: st.mainCore ?? none,
                    cellProps: { value: { dir: 'ltr' } } satisfies Record<
                      string,
                      NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
                    >,
                  },
                  {
                    id: 4,
                    label: t('installed.cpus'),
                    value: st.onlineCpus || none,
                    cellProps: { value: { dir: 'ltr' } } satisfies Record<
                      string,
                      NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
                    >,
                  },
                  {
                    id: 5,
                    label: t('installed.hugepages'),
                    value: (
                      <>
                        {gib(st.hugepagesFreeBytes)}/{gib(st.hugepagesTotalBytes)}
                      </>
                    ),
                    cellProps: { value: { dir: 'ltr' } } satisfies Record<
                      string,
                      NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
                    >,
                  },
                  {
                    id: 6,
                    label: t('live.plugins'),
                    value: switches(st.plugins, t('switch.on'), t('switch.off')) || none,
                    cellProps: { value: { dir: 'ltr' } } satisfies Record<
                      string,
                      NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
                    >,
                  },
                ]}
                rowKey={(item) => item.id}
                rowProps={(item) => item.rowProps ?? {}}
                cellProps={(item, column) => item.cellProps?.[column.key] ?? {}}
                headerRows={<></>}
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
            )}
            {st?.error && (
              <Alert severity="info" sx={{ mt: 1 }}>
                {wording(st.error)}
              </Alert>
            )}
          </Paper>
        </Stack>
      </Stack>
      <Dialog open={open} onClose={() => setOpen(false)} maxWidth="md" fullWidth>
        <DialogTitle>{t('preview.title')}</DialogTitle>
        <DialogContent>
          {preview.isPending && <LinearProgress aria-label={t('config:loading')} />}
          {preview.isError && <ProblemAlert error={preview.error} />}
          {preview.data && (
            <Stack spacing={2}>
              <Alert severity={preview.data.changed ? 'warning' : 'success'}>
                {preview.data.changed ? t('preview.changed') : t('preview.unchanged')}
              </Alert>
              {preview.data.warnings.map((w) => (
                <Alert key={w} severity="info" dir="ltr">
                  {wording(w)}
                </Alert>
              ))}
              <Typography variant="body2" data-testid="dp-preview-summary">
                {t('preview.summary')}
              </Typography>
              <Typography
                component="pre"
                dir="ltr"
                sx={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}
                data-testid="dp-preview-rendered"
              >
                {preview.data.rendered}
              </Typography>
              <Typography
                component="pre"
                dir="ltr"
                sx={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}
                data-testid="dp-preview-diff"
              >
                {preview.data.diff}
              </Typography>
            </Stack>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpen(false)}>{t('preview.close')}</Button>
        </DialogActions>
      </Dialog>
    </PageHeader>
  );
}
