import Alert from '@mui/material/Alert';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '../../../api';
import { call } from '../../../api-problem';

/** Public configured text is rendered through React escaping, never as HTML.
 * Failure cannot prevent login; no credentials or candidate data are requested. */
export function LoginBanner() {
  const { t } = useTranslation('system-identity');
  const banner = useQuery({
    queryKey: ['auth', 'login-banner'],
    queryFn: async ({ signal }) => call(api.GET('/api/v1/auth/banner', { signal })),
    retry: false,
    staleTime: 0,
  });
  if (!banner.data?.banner) return null;
  return (
    <Alert
      severity="info"
      aria-label={t('loginNotice')}
      sx={{ mb: 2, whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}
    >
      {banner.data.banner}
    </Alert>
  );
}
