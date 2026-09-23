import { useTranslation } from 'react-i18next';
import { domainByKey, type RootKey } from '../schema/registry';
import { NotAvailablePage } from '../shell/NotAvailablePage';

/** Placeholder for a configuration domain screen; title and description come from the schema itself. */
export function DomainPlaceholderPage({ domainKey }: { domainKey: RootKey }) {
  const { t } = useTranslation(['common', 'nav']);
  const domain = domainByKey(domainKey);
  const title = t(`nav:domains.${domainKey}`, { defaultValue: domain?.title ?? domainKey });
  return (
    <NotAvailablePage
      title={title}
      subtitle={t('notAvailable.domain', { title: domain?.title ?? domainKey })}
      description={domain?.description}
    />
  );
}
