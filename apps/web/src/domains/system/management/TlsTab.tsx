import {
  ReactGeereedMaterial,
  type GeereedTablePassthrough,
} from '@amnafzar/common-ui-lib/geereed';
import {
  PgAlert as Alert,
  PgChip as Chip,
  PgLinearProgress as LinearProgress,
  PgPaper as Paper,
  PgStack as Stack,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { useFormatters } from '@ngfw/ui-kit';
import { SchemaForm, type JsonSchema, type ProblemDetails } from '@ngfw/ui-kit/schema-form';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiError } from '../../../api-problem';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { serviceText } from '../../../product-text';
import { domainSchemas } from '../../../schema/registry';
import {
  useCandidateTls,
  usePatchTls,
  useRunningTls,
  useTlsState,
  type TlsConfig,
} from './queries';

export const NS = 'management';
const BASE = '/management/tls';

/** `management.tls` sub-schema — the one schema, never a hand-written form. */
export function tlsSchema(): JsonSchema {
  const mgmt = domainSchemas.management as { properties?: { tls?: JsonSchema } };
  const tls = mgmt.properties?.tls;
  if (!tls) throw new Error('management.tls schema not found');
  return tls;
}

/** Server problem with pointers made relative to the form (`/management/tls/privateKeyRef` → `/privateKeyRef`). */
export function problemUnder(error: unknown): ProblemDetails | null {
  if (!(error instanceof ApiError)) return null;
  const p = error.toFormProblem();
  return {
    ...p,
    errors: (p.errors ?? []).map((e) => ({
      ...e,
      pointer: e.pointer.startsWith(BASE) ? e.pointer.slice(BASE.length) || '/' : e.pointer,
    })),
  };
}

type TlsState = NonNullable<ReturnType<typeof useTlsState>['data']>;

/** Label key suffix (`tls.<key>`) and value of each row of the loaded-certificate table. */
function certRows(st: TlsState, fmt: ReturnType<typeof useFormatters>): [string, string][] {
  const a = st.active;
  if (!a) return [];
  return [
    ['subject', a.subject],
    ['issuer', a.issuer],
    ['sans', a.subjectAltNames.join(', ') || '—'],
    ['notBefore', fmt.dateTime(a.notBefore)],
    ['notAfter', fmt.dateTime(a.notAfter)],
    ['daysLeft', fmt.number(a.daysLeft)],
    ['fingerprint', a.fingerprintSha256],
    ['minVersion', `TLS ${st.minVersion}`],
    ['revision', st.loadedRevision === null ? '—' : fmt.number(st.loadedRevision)],
  ];
}

const same = (a: TlsConfig, b: TlsConfig) =>
  (a.certificateRef ?? '') === (b.certificateRef ?? '') &&
  (a.privateKeyRef ?? '') === (b.privateKeyRef ?? '') &&
  (a.minVersion ?? '1.2') === (b.minVersion ?? '1.2');

/**
 * Management › API TLS (F-management-ui): `management.tls` (certificate + key references into the secret store, minimum
 * version) and the certificate the API has actually loaded. A bad pair is refused at commit with a pointer; a good one is
 * swapped in without a restart.
 */
export default function TlsTab() {
  const { t } = useTranslation([NS, 'config']);
  const fmt = useFormatters();
  const perms = usePermissions();
  const cand = useCandidateTls();
  const running = useRunningTls();
  const state = useTlsState();
  const patch = usePatchTls();
  const schema = useMemo(() => tlsSchema(), []);
  const st = state.data;
  const pending = cand.isSuccess && running.isSuccess && !same(cand.data, running.data);

  return (
    <Stack direction={{ xs: 'column', lg: 'row' }} spacing={3} alignItems="flex-start">
      <Paper variant="outlined" sx={{ p: 2, flex: 1, maxWidth: 640, width: '100%' }}>
        <Typography component="h3" variant="h6" gutterBottom>
          {t('tls.configTitle')}
          {pending && (
            <Chip
              size="small"
              color="warning"
              label={t('tls.pending')}
              sx={{ marginInlineStart: 1 }}
            />
          )}
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          {t('tls.intro')}
        </Typography>
        {cand.isPending && <LinearProgress aria-label={t('config:loading')} />}
        {cand.isError && <ProblemAlert error={cand.error} />}
        {cand.isSuccess && (
          <SchemaForm
            id="management-tls-form"
            schema={schema}
            value={cand.data}
            readOnly={!perms.editConfig}
            i18nPrefix={`${NS}:field.tls`}
            onSubmit={async (v) => {
              await patch.mutateAsync(v as TlsConfig).catch(() => undefined);
            }}
            problem={patch.error ? problemUnder(patch.error) : null}
            submitLabel={t('tls.save')}
          />
        )}
      </Paper>
      <Paper variant="outlined" sx={{ p: 2, flex: 1, width: '100%' }}>
        <Typography component="h3" variant="h6" gutterBottom>
          {t('tls.activeTitle')}
        </Typography>
        {state.isPending && <LinearProgress aria-label={t('config:loading')} />}
        {state.isError && <ProblemAlert error={state.error} />}
        {st && !st.listener.enabled && (
          <Alert severity="info" sx={{ mb: 2 }}>
            {t('tls.noListener')}
          </Alert>
        )}
        {st?.error && (
          <Alert severity="error" sx={{ mb: 2 }} data-testid="tls-error">
            {t('tls.loadError', { error: serviceText(st.error) })}
          </Alert>
        )}
        {st && !st.configured && !st.active && <Typography>{t('tls.none')}</Typography>}
        {st?.active && (
          <ReactGeereedMaterial
            tableProps={{ size: 'small', 'aria-label': t('tls.activeTitle') }}
            columns={[
              { key: 'label', filterable: false, renderer: (value) => <>{value}</> },
              { key: 'value', filterable: false, renderer: (value) => <>{value}</> },
            ]}
            items={[
              ...(certRows(st, fmt).map(([k, v]) => ({
                id: k,
                label: t(`tls.${k}`),
                value: v,
                rowProps: { 'data-testid': `tls-${k}` } satisfies ReturnType<
                  NonNullable<GeereedTablePassthrough['rowProps']>
                > &
                  Record<`data-${string}`, unknown>,
                cellProps: {
                  label: { component: 'th' },
                  value: { dir: 'ltr', sx: { wordBreak: 'break-all' } },
                } satisfies Record<
                  string,
                  NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
                >,
              })) ?? []),
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
      </Paper>
    </Stack>
  );
}
