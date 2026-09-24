import { createContext, useContext, useMemo, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { createFormatters, type Formatters, type VrxCalendar } from './formatters.js';

export interface FormatterSettings {
  persianDigits: boolean;
  calendar?: VrxCalendar;
  timeZone?: string;
}

const FormatterSettingsContext = createContext<FormatterSettings>({ persianDigits: false });

export function FormatterSettingsProvider({
  value,
  children,
}: {
  value: FormatterSettings;
  children: ReactNode;
}) {
  return <FormatterSettingsContext.Provider value={value}>{children}</FormatterSettingsContext.Provider>;
}

/** Formatters bound to the current i18next language and the app's digit/calendar settings. */
export function useFormatters(): Formatters {
  const { i18n } = useTranslation();
  const settings = useContext(FormatterSettingsContext);
  const lang = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  return useMemo(
    () =>
      createFormatters({
        locale: lang,
        persianDigits: settings.persianDigits,
        ...(settings.calendar ? { calendar: settings.calendar } : {}),
        ...(settings.timeZone ? { timeZone: settings.timeZone } : {}),
      }),
    [lang, settings.persianDigits, settings.calendar, settings.timeZone],
  );
}
