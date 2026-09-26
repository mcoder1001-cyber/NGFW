import { useTranslation } from 'react-i18next';
import { DashboardOverview } from '../domains/dashboard/overview/DashboardOverview';
import { PageHeader } from '../shell/PageHeader';

/** The landing screen (WEB-dashboard; feature cards plug in through domains/dashboard/overview/cards.ts). */
export function DashboardPage() {
  const { t } = useTranslation();
  return (
    <PageHeader title={t('dashboard.title')}>
      <DashboardOverview />
    </PageHeader>
  );
}
