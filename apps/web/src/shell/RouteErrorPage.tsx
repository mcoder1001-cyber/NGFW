import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Button from '@mui/material/Button';
import Container from '@mui/material/Container';
import { useTranslation } from 'react-i18next';
import { isRouteErrorResponse, useRouteError } from 'react-router';

export function RouteErrorPage() {
  const { t } = useTranslation();
  const error = useRouteError();
  const detail = isRouteErrorResponse(error)
    ? `${error.status} ${error.statusText}`
    : error instanceof Error
      ? error.message
      : String(error);
  return (
    <Container sx={{ py: 4 }}>
      <Alert severity="error" action={<Button color="inherit" onClick={() => window.location.reload()}>{t('error.reload')}</Button>}>
        <AlertTitle>{t('error.title')}</AlertTitle>
        {detail}
      </Alert>
    </Container>
  );
}
