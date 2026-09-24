import Box from '@mui/material/Box';
import CircularProgress from '@mui/material/CircularProgress';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Navigate, useLocation } from 'react-router';
import { useAuth } from './AuthProvider';

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
  if (state.status === 'anonymous') {
    const next = `${location.pathname}${location.search}`;
    return <Navigate to={next === '/' ? '/login' : `/login?next=${encodeURIComponent(next)}`} replace />;
  }
  return <>{children}</>;
}
