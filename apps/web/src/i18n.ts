import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import en from './locales/en/common.json';
import fa from './locales/fa/common.json';

export const RTL_LANGS = new Set(['fa']);

void i18n.use(initReactI18next).init({
  resources: { en: { common: en }, fa: { common: fa } },
  lng: 'en',
  fallbackLng: 'en',
  defaultNS: 'common',
  interpolation: { escapeValue: false },
});

export default i18n;
