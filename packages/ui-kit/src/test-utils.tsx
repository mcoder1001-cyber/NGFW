import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, type RenderOptions, type RenderResult } from '@testing-library/react';
import i18next, { type i18n as I18n } from 'i18next';
import type { ReactElement, ReactNode } from 'react';
import { I18nextProvider, initReactI18next } from 'react-i18next';
import { directionFor, UI_KIT_NS, uiKitResources } from './i18n/index.js';
import { VrxThemeProvider } from './theme/VrxThemeProvider.js';

export function createTestI18n(lang: 'en' | 'fa' = 'en'): I18n {
  const inst = i18next.createInstance();
  void inst.use(initReactI18next).init({
    lng: lang,
    fallbackLng: 'en',
    resources: { en: { [UI_KIT_NS]: uiKitResources.en }, fa: { [UI_KIT_NS]: uiKitResources.fa } },
    interpolation: { escapeValue: false },
    initImmediate: false,
  });
  return inst;
}

export interface ProviderOptions {
  lang?: 'en' | 'fa';
  mode?: 'light' | 'dark';
}

export function Providers({ children, lang = 'en', mode = 'light' }: ProviderOptions & { children: ReactNode }) {
  const i18n = createTestI18n(lang);
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
  return (
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <VrxThemeProvider mode={mode} lang={lang} dir={directionFor(lang)}>
          {children}
        </VrxThemeProvider>
      </QueryClientProvider>
    </I18nextProvider>
  );
}

export function renderWithProviders(ui: ReactElement, opts: ProviderOptions & Omit<RenderOptions, 'wrapper'> = {}): RenderResult {
  const { lang, mode, ...rest } = opts;
  return render(ui, {
    ...rest,
    wrapper: ({ children }) => (
      <Providers {...(lang ? { lang } : {})} {...(mode ? { mode } : {})}>
        {children}
      </Providers>
    ),
  });
}
