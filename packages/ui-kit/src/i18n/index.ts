import { en } from './locales/en.js';
import { fa } from './locales/fa.js';

/** i18next namespace used by every ui-kit component. The app registers `uiKitResources` under it. */
export const UI_KIT_NS = 'ui-kit' as const;

/** Languages the ui-kit ships strings for; the app must provide the same set for its own namespaces. */
export const uiKitResources = { en, fa } as const;

/** Languages rendered right-to-left. */
export const RTL_LANGUAGES: ReadonlySet<string> = new Set(['fa', 'ar', 'he', 'ur']);

export function directionFor(lang: string): 'rtl' | 'ltr' {
  return RTL_LANGUAGES.has(lang.split('-')[0]!) ? 'rtl' : 'ltr';
}

export * from './formatters.js';
export * from './useFormatters.js';
export type { UiKitResource } from './locales/en.js';
