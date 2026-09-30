import { useTranslation } from 'react-i18next';
import { useCallback } from 'react';

/** Presentation names only: never apply these to raw configuration, audit records or exported data. */
export function productWording(text: string, language = 'en'): string {
  const fa = language.split('-')[0] === 'fa';
  const names: Record<string, string> = {
    vpp: fa ? 'صفحهٔ داده' : 'dataplane',
    frr: fa ? 'سرویس مسیریابی' : 'routing service',
    frrouting: fa ? 'سرویس مسیریابی' : 'routing service',
    strongswan: fa ? 'سرویس IPsec' : 'IPsec service',
  };
  const alias = (name: string) => names[name.toLowerCase()] ?? name;
  // A diagnostic may include an implementation path. Describe that resource instead of displaying a fictional path.
  return text
    .replace(/(?:\/[\w.~+-]+)+/g, (path) => {
      const name = /(?:^|\/)(vpp|frr|strongswan)(?=\/|[.-]|$)/i.exec(path)?.[1];
      return name ? `${alias(name)} ${fa ? 'منبع' : 'resource'}` : path;
    })
    .replace(/\bkernel-vpp\b/gi, fa ? 'یکپارچه‌سازی IPsec' : 'IPsec integration')
    .replace(/\bvpp-ikev2\b/gi, fa ? 'IKEv2 داخلی' : 'native IKEv2')
    .replace(/\bfrr-linuxcp\b/gi, fa ? 'یکپارچه‌سازی مسیریابی' : 'routing integration')
    .replace(/\bvppctl\b/gi, fa ? 'فرمان صفحهٔ داده' : 'dataplane command')
    .replace(/\bfrr-reload(?:\.py)?\b/gi, fa ? 'بارگذاری سرویس مسیریابی' : 'routing reload')
    .replace(/\b(?:FRRouting|FRR|VPP|strongSwan)(?=$|[^\w]|_)(?:[._-][\w.-]+)*/gi, (name) => {
      const base = name.split(/[._-]/)[0] ?? name;
      return alias(base);
    });
}

/** Labels for persisted implementation choices; select values stay exactly as the API expects. */
export function productEngineLabels(language = 'en'): Readonly<Record<string, string>> {
  return language.split('-')[0] === 'fa'
    ? { vpp: 'صفحهٔ داده', strongswan: 'سرویس IPsec', 'vpp-ikev2': 'IKEv2 داخلی' }
    : { vpp: 'Dataplane', strongswan: 'IPsec service', 'vpp-ikev2': 'Native IKEv2' };
}

export function useProductWording(): (text: string) => string {
  const { i18n } = useTranslation();
  const language = i18n.language;
  return useCallback((text: string) => productWording(text, language), [language]);
}
