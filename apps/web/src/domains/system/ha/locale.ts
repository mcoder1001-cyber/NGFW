import enHa from '../../../locales/en/ha.json';
import faHa from '../../../locales/fa/ha.json';

/**
 * The `ha` namespace: F-vrrp-config-sync moved WEB-4b's strings to locales/{en,fa}/ha.json (registered in i18n.ts
 * like every feature namespace). `haEn` / `haFa` stay exported for the page's key-parity test.
 */
export const NS = 'ha';

export const haEn = enHa;
export const haFa: typeof haEn = faHa;
