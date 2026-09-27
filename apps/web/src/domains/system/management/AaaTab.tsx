import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import { useTranslation } from 'react-i18next';

/**
 * Management › AAA: RADIUS / TACACS+ login backends are built by F-aaa (not landed). Until then the tab says so
 * honestly (00-CONTEXT: never a screen over a stubbed backend); F-aaa replaces this entry in `tabs.ts`.
 */
export default function AaaTab() {
  const { t } = useTranslation(['management', 'common']);
  return (
    <Alert severity="info" sx={{ maxInlineSize: 720 }} data-testid="aaa-not-available">
      <AlertTitle>{t('common:notAvailable.title')}</AlertTitle>
      {t('aaa.notAvailable')}
    </Alert>
  );
}
