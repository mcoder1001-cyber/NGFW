import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Typography from '@mui/material/Typography';
import { useTranslation } from 'react-i18next';
import { PageHeader } from './PageHeader';

export interface NotAvailablePageProps {
  title: string;
  /** Second line, e.g. the configuration domain this screen will edit. */
  subtitle?: string | undefined;
  description?: string | undefined;
}

/** The only thing a not-yet-built screen may show (00-CONTEXT "never ship a UI screen whose backend is stubbed"). */
export function NotAvailablePage({ title, subtitle, description }: NotAvailablePageProps) {
  const { t } = useTranslation();
  return (
    <PageHeader title={title}>
      {subtitle && (
        <Typography color="text.secondary" gutterBottom>
          {subtitle}
        </Typography>
      )}
      {description && <Typography gutterBottom>{description}</Typography>}
      <Alert severity="info" sx={{ maxInlineSize: 720 }}>
        <AlertTitle>{t('notAvailable.title')}</AlertTitle>
        {t('notAvailable.body')}
      </Alert>
    </PageHeader>
  );
}
