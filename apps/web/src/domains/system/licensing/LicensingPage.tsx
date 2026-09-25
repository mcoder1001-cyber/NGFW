import UploadFileIcon from '@mui/icons-material/UploadFile';
import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import LinearProgress from '@mui/material/LinearProgress';
import Link from '@mui/material/Link';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
import { useFormatters } from '@ngfw/ui-kit';
import type { ChangeEvent, ReactElement } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
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
            {st.reason && <Alert severity="error">{st.reason}</Alert>}
            <Table size="small" aria-label={t('status.heading')}>
              <TableBody>
                {st.customer !== undefined && (
                  <TableRow>
                    <TableCell component="th">{t('field.customer')}</TableCell>
                    <TableCell>{st.customer}</TableCell>
                  </TableRow>
                )}
                {st.licenseId !== undefined && (
                  <TableRow>
                    <TableCell component="th">{t('field.licenseId')}</TableCell>
                    <TableCell dir="ltr">{st.licenseId}</TableCell>
                  </TableRow>
                )}
                {st.expiresAt !== undefined && (
                  <TableRow>
                    <TableCell component="th">{t('field.expiresAt')}</TableCell>
                    <TableCell>{fmt.dateTime(new Date(st.expiresAt))}</TableCell>
                  </TableRow>
                )}
                {(st.status === 'valid' || st.status === 'grace') && (
                  <TableRow>
                    <TableCell component="th">{t(st.status === 'grace' ? 'field.graceLeft' : 'field.daysLeft')}</TableCell>
                    <TableCell>{fmt.number(st.daysLeft)}</TableCell>
                  </TableRow>
                )}
                {(st.bound.machineId || st.bound.serial) && (
                  <TableRow>
                    <TableCell component="th">{t('field.binding')}</TableCell>
                    <TableCell>{[st.bound.machineId && t('binding.machineId'), st.bound.serial && t('binding.serial')].filter(Boolean).join(', ')}</TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
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
            {st.status === 'grace' && <Alert severity="warning" sx={{ mt: 1 }}>{t('banner.grace', { days: st.daysLeft })}</Alert>}
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
              <Table size="small" aria-label={t('entitlements.heading')}>
                <TableHead>
                  <TableRow>
                    <TableCell>{t('entitlements.feature')}</TableCell>
                    <TableCell>{t('entitlements.licensed')}</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {GATED_FEATURES.map((f) => (
                    <TableRow key={f}>
                      <TableCell>{t(`feature.${f}`)}</TableCell>
                      <TableCell>{st.entitlements.features.includes(f) ? t('entitlements.yes') : t('entitlements.no')}</TableCell>
                    </TableRow>
                  ))}
                  {limits.map(([k, v]) => (
                    <TableRow key={k}>
                      <TableCell>{t(`limit.${k}`, { defaultValue: k })}</TableCell>
                      <TableCell>{t('entitlements.max', { n: fmt.number(v) })}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          </Paper>

          {perms.manageUsers && (
            <Paper sx={{ p: 2 }} id="license-upload" component="section" aria-labelledby="license-upload-heading">
              <Typography component="h3" variant="h6" gutterBottom id="license-upload-heading">
                {t('upload.heading')}
              </Typography>
              <Typography sx={{ mb: 1 }}>{t('upload.help')}</Typography>
              <Button component="label" variant="contained" startIcon={<UploadFileIcon />} disabled={upload.isPending}>
                {t('upload.button')}
                <input hidden type="file" accept=".vrxlic,application/json" aria-label={t('upload.button')} onChange={(e) => void onFile(e)} />
              </Button>
              {upload.isSuccess && <Alert severity="success" sx={{ mt: 1 }}>{t('upload.done')}</Alert>}
              {upload.error instanceof SyntaxError && <Alert severity="error" sx={{ mt: 1 }}>{t('upload.notJson')}</Alert>}
              {upload.error && !(upload.error instanceof SyntaxError) && <ProblemAlert error={upload.error} sx={{ mt: 1 }} />}
            </Paper>
          )}
        </Stack>
      )}
    </>
  );
}
