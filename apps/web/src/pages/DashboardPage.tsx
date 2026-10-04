import { useQuery } from '@tanstack/react-query';
import { Navigate } from 'react-router';
import { api } from '../api';
import { call } from '../api-problem';
import { RootConfig } from '@ngfw/schema';
import { usePermissions } from '../auth/AuthProvider';
import { useTranslation } from 'react-i18next';
import { DashboardOverview } from '../domains/dashboard/overview/DashboardOverview';
import { PageHeader } from '../shell/PageHeader';

/** The landing screen (WEB-dashboard; feature cards plug in through domains/dashboard/overview/cards.ts). */
export function DashboardPage() {
  const { t } = useTranslation();
  const { manageUsers } = usePermissions();
  const setup = useQuery({
    queryKey: ['config', 'setup-status'],
    queryFn: async () =>
      RootConfig.parse((await call(api.GET('/api/v1/config'))).data).system.setup,
    enabled: manageUsers,
  });
  if (manageUsers && setup.data && !setup.data.completed)
    return <Navigate to="/system/setup" replace />;
  return (
    <PageHeader title={t('dashboard.title')}>
      <DashboardOverview />
    </PageHeader>
  );
}
