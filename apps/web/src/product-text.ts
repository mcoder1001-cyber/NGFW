import { productWording } from '@ngfw/ui-kit';
import i18n from './i18n';

/** Translate implementation names only at the presentation boundary; API data stays intact. */
export function serviceText(text: string): string {
  return productWording(text, i18n.language);
}

/** Descriptor keys are identities, not JSON pointers. Only their known service prefix is presentation metadata. */
export function resultKeyText(key: string): string {
  return key.replace(/^(?:frr|vpp|strongswan)(?=[./_-]|$)/i, (service) => serviceText(service));
}
