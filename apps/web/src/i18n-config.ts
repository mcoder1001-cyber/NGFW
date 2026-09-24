/** Languages the UI ships. Adding one means: locales/<lang>/*.json + ui-kit resources + RTL_LANGUAGES check. */
export const SUPPORTED_LANGUAGES = ['en', 'fa'] as const;
export type Language = (typeof SUPPORTED_LANGUAGES)[number];

/** Native names shown in the language switcher (never translated). */
export const LANGUAGE_NAMES: Record<Language, string> = { en: 'English', fa: 'فارسی' };

export function isLanguage(v: unknown): v is Language {
  return typeof v === 'string' && (SUPPORTED_LANGUAGES as readonly string[]).includes(v);
}
