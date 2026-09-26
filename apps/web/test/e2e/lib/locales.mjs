// apps/web/test/e2e/lib/locales.mjs — translations straight from the app's own i18n files, so every lookup in a
// script matches exactly what the UI renders in that language (never a hand-typed English/Persian guess).
import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';

/** Every namespace JSON for one language (`apps/web/src/locales/<lang>/*.json`), keyed by filename without `.json`. */
export function loadLocaleNamespaces(webRoot, lang) {
  const dir = join(webRoot, 'src/locales', lang);
  return Object.fromEntries(readdirSync(dir).map((f) => [f.replace(/\.json$/, ''), JSON.parse(readFileSync(join(dir, f), 'utf8'))]));
}

/**
 * A `tr(lang, key, vars)` translator over the app's own strings for every language in `langs` (loaded once, eagerly —
 * a missing key throws immediately rather than silently rendering `undefined` into a selector). `key` is
 * `namespace:dotted.path` (e.g. `"users:field.role.title"`), matching the `t()` calls in the source.
 */
export function createTranslator(webRoot, langs) {
  const locales = Object.fromEntries(langs.map((lang) => [lang, loadLocaleNamespaces(webRoot, lang)]));
  return function tr(lang, key, vars = {}) {
    const [ns, path] = key.split(':');
    let v = path.split('.').reduce((o, k) => (o == null ? undefined : o[k]), locales[lang]?.[ns]);
    if (typeof v !== 'string') throw new Error(`no string ${lang}:${key}`);
    for (const [k, val] of Object.entries(vars)) v = v.replaceAll(`{{${k}}}`, String(val));
    return v;
  };
}
