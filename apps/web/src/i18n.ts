import { UI_KIT_NS, uiKitResources } from '@ngfw/ui-kit';
import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import enCommon from './locales/en/common.json';
import enDev from './locales/en/dev.json';
import enNav from './locales/en/nav.json';
import faCommon from './locales/fa/common.json';
import faDev from './locales/fa/dev.json';
import faNav from './locales/fa/nav.json';
import { loadSettings } from './settings/storage';

export const NAMESPACES = ['common', 'nav', 'dev', UI_KIT_NS] as const;

export const resources = {
  en: { common: enCommon, nav: enNav, dev: enDev, [UI_KIT_NS]: uiKitResources.en },
  fa: { common: faCommon, nav: faNav, dev: faDev, [UI_KIT_NS]: uiKitResources.fa },
} as const;

void i18n.use(initReactI18next).init({
  resources,
  lng: loadSettings().lang,
  fallbackLng: 'en',
  supportedLngs: ['en', 'fa'],
  defaultNS: 'common',
  ns: [...NAMESPACES],
  interpolation: { escapeValue: false },
  returnNull: false,
  initImmediate: false,
});

export default i18n;
