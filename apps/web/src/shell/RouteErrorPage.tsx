import { serviceText } from '../product-text';
import { PgAlert as Alert } from '@amnafzar/common-ui-lib/primitives';
import { PgAlertTitle as AlertTitle } from '@amnafzar/common-ui-lib/primitives';
import { PgButton as Button } from '@amnafzar/common-ui-lib/forms';
import { PgContainer as Container } from '@amnafzar/common-ui-lib/primitives';
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
        {serviceText(detail)}
      </Alert>
    </Container>
  );
}
