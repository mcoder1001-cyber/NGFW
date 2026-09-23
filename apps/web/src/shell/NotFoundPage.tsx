import Button from '@mui/material/Button';
import Typography from '@mui/material/Typography';
import { useTranslation } from 'react-i18next';
import { Link, useLocation } from 'react-router';
import { PageHeader } from './PageHeader';

export function NotFoundPage() {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  return (
    <PageHeader title={t('notFound.title')}>
      <Typography gutterBottom>{t('notFound.body', { path: pathname })}</Typography>
      <Button component={Link} to="/" variant="outlined">
        {t('notFound.home')}
      </Button>
    </PageHeader>
  );
}
