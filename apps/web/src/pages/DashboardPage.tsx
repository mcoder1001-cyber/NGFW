import Alert from '@mui/material/Alert';
import Stack from '@mui/material/Stack';
import { useTranslation } from 'react-i18next';
import { HealthCard } from '../HealthCard';
import { PageHeader } from '../shell/PageHeader';

export function DashboardPage() {
  const { t } = useTranslation();
  return (
    <PageHeader title={t('dashboard.title')}>
      <Stack gap={2} sx={{ maxInlineSize: 720 }}>
        <Alert severity="info">{t('dashboard.widgetsNotAvailable')}</Alert>
        <HealthCard />
      </Stack>
    </PageHeader>
  );
}
