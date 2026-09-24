import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Box from '@mui/material/Box';
import CircularProgress from '@mui/material/CircularProgress';
import LinearProgress from '@mui/material/LinearProgress';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Navigate, useLocation } from 'react-router';
import { ConfirmBanner } from '../config/ConfirmBanner';
import { useAuth } from './AuthProvider';

/**
 * The device did not answer the session restore (page reloaded during an outage — typically right after a commit
 * that cut the operator's own access). Never the login page here (review M2): the countdown of a tracked confirmed
 * commit keeps running locally, and the session restore is retried until the device answers.
 */
function OfflineScreen() {
  const { t } = useTranslation('auth');
  return (
    <Box component="main" id="main" sx={{ p: 3, maxInlineSize: 960, marginInline: 'auto' }} data-testid="offline-screen">
      <ConfirmBanner offline />
      <Alert severity="warning">
        <AlertTitle>{t('offline.title')}</AlertTitle>
        {t('offline.body')}
      </Alert>
      <LinearProgress sx={{ mt: 1 }} aria-label={t('offline.title')} />
    </Box>
  );
}

/** Protected routes: wait for the session restore, send anonymous users to `/login?next=<where they were going>`. */
export function RequireAuth({ children }: { children: ReactNode }) {
  const { state } = useAuth();
  const location = useLocation();
  const { t } = useTranslation('auth');
  if (state.status === 'unknown') {
    return (
      <Box sx={{ display: 'grid', placeItems: 'center', minBlockSize: '100vh' }}>
        <CircularProgress aria-label={t('restoring')} />
      </Box>
    );
  }
  if (state.status === 'offline') return <OfflineScreen />;
  if (state.status === 'anonymous') {
    const next = `${location.pathname}${location.search}`;
    return <Navigate to={next === '/' ? '/login' : `/login?next=${encodeURIComponent(next)}`} replace />;
  }
  return <>{children}</>;
}
