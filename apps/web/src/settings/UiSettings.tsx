import { directionFor, FormatterSettingsProvider, VrxThemeProvider } from '@ngfw/ui-kit';
import useMediaQuery from '@mui/material/useMediaQuery';
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { loadSettings, saveSettings, type UiSettings } from './storage';

interface UiSettingsContextValue {
  settings: UiSettings;
  resolvedMode: 'light' | 'dark';
  update: (patch: Partial<UiSettings>) => void;
}

const UiSettingsContext = createContext<UiSettingsContextValue | null>(null);

/** Theme mode, language (→ `<html dir lang>` via VrxThemeProvider), Persian digits and density, persisted locally. */
export function UiSettingsProvider({ children, initial }: { children: ReactNode; initial?: UiSettings }) {
  const { i18n } = useTranslation();
  const [settings, setSettings] = useState<UiSettings>(() => initial ?? loadSettings());
  const prefersDark = useMediaQuery('(prefers-color-scheme: dark)');
  const resolvedMode = settings.mode === 'system' ? (prefersDark ? 'dark' : 'light') : settings.mode;

  useEffect(() => {
    saveSettings(settings);
  }, [settings]);
  useEffect(() => {
    if (i18n.resolvedLanguage !== settings.lang) void i18n.changeLanguage(settings.lang);
  }, [i18n, settings.lang]);

  const update = useCallback((patch: Partial<UiSettings>) => setSettings((s) => ({ ...s, ...patch })), []);
  const value = useMemo(() => ({ settings, resolvedMode, update }), [settings, resolvedMode, update]);
  const formatterSettings = useMemo(
    () => ({ persianDigits: settings.persianDigits, calendar: settings.lang === 'fa' ? ('persian' as const) : ('gregory' as const) }),
    [settings.persianDigits, settings.lang],
  );

  return (
    <UiSettingsContext.Provider value={value}>
      <VrxThemeProvider mode={resolvedMode} lang={settings.lang} dir={directionFor(settings.lang)} dense={settings.dense}>
        <FormatterSettingsProvider value={formatterSettings}>{children}</FormatterSettingsProvider>
      </VrxThemeProvider>
    </UiSettingsContext.Provider>
  );
}

export function useUiSettings(): UiSettingsContextValue {
  const ctx = useContext(UiSettingsContext);
  if (!ctx) throw new Error('useUiSettings must be used inside <UiSettingsProvider>');
  return ctx;
}
