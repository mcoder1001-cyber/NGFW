import Alert from '@mui/material/Alert';
import Link from '@mui/material/Link';
import { useTranslation } from 'react-i18next';
import { Link as RouterLink } from 'react-router';
import { useLicenseState } from './queries';

/** Shell banner (AppShell): shown while no licence is installed (D-151: gated features refused), or in grace, expired or invalid. Errors render nothing. */
export function LicenseBanner() {
  const { t } = useTranslation('licensing');
  const q = useLicenseState();
  const st = q.data;
  if (!st || st.status === 'valid') return null;
  return (
    <Alert severity={st.status === 'grace' || st.status === 'community' ? 'warning' : 'error'} sx={{ mb: 2 }} data-testid="license-banner">
      {t(`banner.${st.status}`, { days: st.daysLeft })}{' '}
      <Link component={RouterLink} to="/system/licensing#license-upload">
        {t('banner.link')}
      </Link>
    </Alert>
  );
}
