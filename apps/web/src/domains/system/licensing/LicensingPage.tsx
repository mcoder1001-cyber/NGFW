import { PgButton as Button } from '@amnafzar/common-ui-lib/forms';
import {
  ReactGeereedMaterial,
  type GeereedTablePassthrough,
} from '@amnafzar/common-ui-lib/geereed';
import { PgUploadFileIcon as UploadFileIcon } from '@amnafzar/common-ui-lib/icons';
import {
  PgAlert as Alert,
  PgChip as Chip,
  PgLinearProgress as LinearProgress,
  PgLink as Link,
  PgPaper as Paper,
  PgStack as Stack,
  PgTableContainer as TableContainer,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { useFormatters } from '@ngfw/ui-kit';
import type { ChangeEvent, ReactElement } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { serviceText } from '../../../product-text';
import { PageHeader } from '../../../shell/PageHeader';
import { useLicenseState, useUploadLicense, type LicenseState } from './queries';

const COLOR: Record<LicenseState['status'], 'success' | 'warning' | 'error' | 'default'> = {
  valid: 'success',
  grace: 'warning',
  expired: 'error',
  invalid: 'error',
  community: 'default',
};

/** Features the API gates (apps/api/src/features/licensing/entitlements.ts, sample table). */
export const GATED_FEATURES = ['ipsec', 'wireguard', 'bgp', 'ospf', 'isis', 'ha'] as const;

/** File contents as text (FileReader: also available where Blob.text() is not, e.g. jsdom). */
function readText(f: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const r = new FileReader();
    r.onload = () => resolve(String(r.result));
    r.onerror = () => reject(r.error ?? new Error('read failed'));
    r.readAsText(f);
  });
}

/** System → Licence: status, entitlements in force, upload (admin). */
export function LicensingPage(): ReactElement {
  const { t } = useTranslation('licensing');
  const fmt = useFormatters();
  const perms = usePermissions();
  const q = useLicenseState();
  const upload = useUploadLicense();

  const onFile = async (e: ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0];
    e.target.value = '';
    if (f) upload.mutate(await readText(f));
  };

  const st = q.data;
  const limits = Object.entries(st?.entitlements.limits ?? {});
  return (
    <>
      <PageHeader title={t('title')} />
      {q.isPending && <LinearProgress aria-label={t('loading')} />}
      {q.error && <ProblemAlert error={q.error} />}
      {st && (
        <Stack spacing={2}>
          <Paper sx={{ p: 2 }}>
            <Stack direction="row" spacing={1} sx={{ alignItems: 'center', mb: 1 }}>
              <Typography component="h3" variant="h6">
                {t('status.heading')}
              </Typography>
              <Chip
                label={t(`status.${st.status}`)}
                color={COLOR[st.status]}
                size="small"
                role="status"
                aria-label={t('status.aria', { status: t(`status.${st.status}`) })}
                data-testid="license-status"
              />
            </Stack>
            {st.reason && <Alert severity="error">{serviceText(st.reason)}</Alert>}
            <ReactGeereedMaterial
              tableProps={{ size: 'small', 'aria-label': t('status.heading') }}
              columns={[
                { key: 'label', filterable: false, renderer: (value) => <>{value}</> },
                { key: 'value', filterable: false, renderer: (value) => <>{value}</> },
              ]}
              items={[
                ...(st.customer !== undefined
                  ? [
                      {
                        id: 0,
                        label: t('field.customer'),
                        value: st.customer,
                        cellProps: { label: { component: 'th' } } satisfies Record<
                          string,
                          NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
                        >,
                      },
                    ]
                  : []),
                ...(st.licenseId !== undefined
                  ? [
                      {
                        id: 1,
                        label: t('field.licenseId'),
                        value: st.licenseId,
                        cellProps: {
                          label: { component: 'th' },
                          value: { dir: 'ltr' },
                        } satisfies Record<
                          string,
                          NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
                        >,
                      },
                    ]
                  : []),
                ...(st.expiresAt !== undefined
                  ? [
                      {
                        id: 2,
                        label: t('field.expiresAt'),
                        value: fmt.dateTime(new Date(st.expiresAt)),
                        cellProps: { label: { component: 'th' } } satisfies Record<
                          string,
                          NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
                        >,
                      },
                    ]
                  : []),
                ...(st.status === 'valid' || st.status === 'grace'
                  ? [
                      {
                        id: 3,
                        label: t(st.status === 'grace' ? 'field.graceLeft' : 'field.daysLeft'),
                        value: fmt.number(st.daysLeft),
                        cellProps: { label: { component: 'th' } } satisfies Record<
                          string,
                          NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
                        >,
                      },
                    ]
                  : []),
                ...(st.bound.machineId || st.bound.serial
                  ? [
                      {
                        id: 4,
                        label: t('field.binding'),
                        value: [
                          st.bound.machineId && t('binding.machineId'),
                          st.bound.serial && t('binding.serial'),
                        ]
                          .filter(Boolean)
                          .join(', '),
                        cellProps: { label: { component: 'th' } } satisfies Record<
                          string,
                          NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
                        >,
                      },
                    ]
                  : []),
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
            {st.status === 'community' && (
              <Alert severity="warning" sx={{ mt: 1 }} data-testid="license-community-help">
                {t('status.communityHelp')}
                {perms.manageUsers && (
                  <>
                    {' '}
                    <Link href="#license-upload">{t('upload.link')}</Link>
                  </>
                )}
              </Alert>
            )}
            {st.status === 'grace' && (
              <Alert severity="warning" sx={{ mt: 1 }}>
                {t('banner.grace', { days: st.daysLeft })}
              </Alert>
            )}
            {(st.status === 'expired' || st.status === 'invalid') && (
              <Alert severity="error" sx={{ mt: 1 }}>
                {t('status.expiredHelp')}
                {perms.manageUsers && (
                  <>
                    {' '}
                    <Link href="#license-upload">{t('upload.link')}</Link>
                  </>
                )}
              </Alert>
            )}
          </Paper>

          <Paper sx={{ p: 2 }}>
            <Typography component="h3" variant="h6" gutterBottom>
              {t('entitlements.heading')}
            </Typography>
            <TableContainer>
              <ReactGeereedMaterial
                tableProps={{ size: 'small', 'aria-label': t('entitlements.heading') }}
                columns={[
                  {
                    key: 'feature',
                    headerRenderer: () => t('entitlements.feature'),
                    filterable: false,
                    renderer: (value) => <>{value}</>,
                  },
                  {
                    key: 'licensed',
                    headerRenderer: () => t('entitlements.licensed'),
                    filterable: false,
                    renderer: (value) => <>{value}</>,
                  },
                ]}
                items={[
                  ...(GATED_FEATURES.map((f) => ({
                    id: f,
                    feature: t(`feature.${f}`),
                    licensed: st.entitlements.features.includes(f)
                      ? t('entitlements.yes')
                      : t('entitlements.no'),
                  })) ?? []),
                  ...(limits.map(([k, v]) => ({
                    id: k,
                    feature: t(`limit.${k}`, { defaultValue: k }),
                    licensed: t('entitlements.max', { n: fmt.number(v) }),
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
          </Paper>

          {perms.manageUsers && (
            <Paper
              sx={{ p: 2 }}
              id="license-upload"
              component="section"
              aria-labelledby="license-upload-heading"
            >
              <Typography component="h3" variant="h6" gutterBottom id="license-upload-heading">
                {t('upload.heading')}
              </Typography>
              <Typography sx={{ mb: 1 }}>{t('upload.help')}</Typography>
              <Button
                component="label"
                variant="contained"
                startIcon={<UploadFileIcon />}
                disabled={upload.isPending}
              >
                {t('upload.button')}
                <input
                  hidden
                  type="file"
                  accept=".ngfwlic,application/json"
                  aria-label={t('upload.button')}
                  onChange={(e) => void onFile(e)}
                />
              </Button>
              {upload.isSuccess && (
                <Alert severity="success" sx={{ mt: 1 }}>
                  {t('upload.done')}
                </Alert>
              )}
              {upload.error instanceof SyntaxError && (
                <Alert severity="error" sx={{ mt: 1 }}>
                  {t('upload.notJson')}
                </Alert>
              )}
              {upload.error && !(upload.error instanceof SyntaxError) && (
                <ProblemAlert error={upload.error} sx={{ mt: 1 }} />
              )}
            </Paper>
          )}
        </Stack>
      )}
    </>
  );
}
