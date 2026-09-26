import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import { useTranslation } from 'react-i18next';
import { Link as RouterLink } from 'react-router';
import { configPathTo } from '../domains/advanced/schemaPath';
import { domainByKey, type RootKey } from '../schema/registry';
import { NotAvailablePage } from '../shell/NotAvailablePage';

/**
 * Placeholder for a configuration domain screen; title and description come from the schema itself. Every domain,
 * built or not, can still be edited generically through the advanced editor (D-125, UI-domain-editor).
 */
export function DomainPlaceholderPage({ domainKey }: { domainKey: RootKey }) {
  const { t } = useTranslation(['common', 'nav', 'advanced']);
  const domain = domainByKey(domainKey);
  const title = t(`nav:domains.${domainKey}`, { defaultValue: domain?.title ?? domainKey });
  return (
    <>
      <NotAvailablePage
        title={title}
        subtitle={t('notAvailable.domain', { title: domain?.title ?? domainKey })}
        description={domain?.description}
      />
      <Box sx={{ mt: 2 }}>
        <Button component={RouterLink} to={`/${configPathTo(domainKey)}`} variant="outlined">
          {t('advanced:openInAdvanced')}
        </Button>
      </Box>
    </>
  );
}
