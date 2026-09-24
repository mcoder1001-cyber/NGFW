import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';

/** Localised, language-correct list of configuration domain names (`interfaces, acl` → "Interfaces and ACLs"). */
export function useDomainList(): (keys: readonly string[]) => string {
  const { t, i18n } = useTranslation('nav');
  return useCallback(
    (keys) => new Intl.ListFormat(i18n.language, { type: 'conjunction' }).format(keys.map((k) => t(`domains.${k}`, { defaultValue: k }))),
    [t, i18n.language],
  );
}
