import { UI_KIT_NS, uiKitResources } from '@ngfw/ui-kit';
import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import { DEV_ROUTES } from './build-flags';
import enAuth from './locales/en/auth.json';
import enCommon from './locales/en/common.json';
import enConfig from './locales/en/config.json';
import enDev from './locales/en/dev.json';
import enNav from './locales/en/nav.json';
import enRevisions from './locales/en/revisions.json';
import enUsers from './locales/en/users.json';
import faAuth from './locales/fa/auth.json';
import faCommon from './locales/fa/common.json';
import faConfig from './locales/fa/config.json';
import faDev from './locales/fa/dev.json';
import faNav from './locales/fa/nav.json';
import faRevisions from './locales/fa/revisions.json';
import faUsers from './locales/fa/users.json';
import { loadSettings } from './settings/storage';

export const NAMESPACES = ['common', 'nav', 'auth', 'config', 'revisions', 'users', 'dev', UI_KIT_NS] as const;

const en = { common: enCommon, nav: enNav, auth: enAuth, config: enConfig, revisions: enRevisions, users: enUsers };
const fa = { common: faCommon, nav: faNav, auth: faAuth, config: faConfig, revisions: faRevisions, users: faUsers };

/** The `dev` namespace (developer demo pages) is loaded only when the demo routes are built in (review P07a M1). */
export const resources = DEV_ROUTES
  ? {
      en: { ...en, dev: enDev, [UI_KIT_NS]: uiKitResources.en },
      fa: { ...fa, dev: faDev, [UI_KIT_NS]: uiKitResources.fa },
    }
  : {
      en: { ...en, [UI_KIT_NS]: uiKitResources.en },
      fa: { ...fa, [UI_KIT_NS]: uiKitResources.fa },
    };

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
