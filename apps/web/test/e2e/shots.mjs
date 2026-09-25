#!/usr/bin/env node
// apps/web/test/e2e/shots.mjs — WEB-3 committed browser harness: one CLI every feature worker runs against their own
// slot stack instead of hand-rolling a shots.mjs (P08's script was lost; F-vlan-qinq and F-nat44-ed-sessions each
// committed a private one — see review F6). Screens live one file per screen under `screens/` (copy
// `screens/_example.mjs`); this file drives the browser, logs in once per language, and calls each screen once per
// (language × theme) combination with a ready-made `ctx` (already signed in, at the right nav entry's disposal).
//
//   node apps/web/test/e2e/shots.mjs --base http://127.0.0.1:<port> --screens <slug>[,<slug>...] --out <dir>
//       [--langs en,fa] [--themes light] [--admin-user admin] [--admin-password-file <file>]
//
// tools/app is NOT this harness's target — only a worker's own slot stack (docs/lab/shared-host-rules.md): point
// --base at your slot's vite dev/preview port, never 3000/5173. Nothing here is a workspace dependency (Playwright
// loads from VRX_PLAYWRIGHT_CORE, Chrome from VRX_CHROME) — see README.md.
//
// Exit code is non-zero on any failed check OR any browser `pageerror` — a run that took screenshots while the page
// silently threw is not evidence of anything working.
import { mkdirSync, readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { login } from './lib/auth.mjs';
import { launchBrowser, newPage } from './lib/browser.mjs';
import { createChecklist } from './lib/checklist.mjs';
import { createTranslator } from './lib/locales.mjs';
import { nav } from './lib/nav.mjs';
import { shot } from './lib/shot.mjs';
import { setTheme } from './lib/theme.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const webRoot = resolve(here, '../..');
const args = process.argv.slice(2);
const opt = (name, dflt) => {
  const i = args.indexOf(`--${name}`);
  return i < 0 ? dflt : args[i + 1];
};
const BASE = opt('base', 'http://127.0.0.1:5100');
const SCREENS = opt('screens', '').split(',').filter(Boolean);
const OUT = opt('out', '');
const LANGS = opt('langs', 'en,fa').split(',');
const THEMES = opt('themes', 'light').split(',');
const ADMIN_USER = opt('admin-user', process.env.VRX_E2E_ADMIN_USER ?? 'admin');
const ADMIN_PW_FILE = opt('admin-password-file', process.env.VRX_E2E_ADMIN_PASSWORD_FILE ?? '/run/vrx-test/w1/admin.pw');

if (SCREENS.length === 0 || !OUT) {
  console.error('usage: shots.mjs --base <url> --screens <slug>[,<slug>...] --out <dir> [--langs en,fa] [--themes light,dark]');
  process.exit(2);
}

const ADMIN_PW = readFileSync(ADMIN_PW_FILE, 'utf8').trim();
mkdirSync(OUT, { recursive: true });
const tr = createTranslator(webRoot, LANGS);
const { ok, check, results } = createChecklist();

const browser = await launchBrowser({ args: ['--no-sandbox'] });
let failed = false;
try {
  for (const slug of SCREENS) {
    const mod = await import(pathToFileURL(resolve(here, 'screens', `${slug}.mjs`)).href);
    const run = mod.default ?? mod[slug];
    if (typeof run !== 'function') throw new Error(`screens/${slug}.mjs must export a default (or "${slug}") async function — see screens/_example.mjs`);
    for (const lang of LANGS) {
      const { page, close, assertNoPageErrors } = await newPage(browser, { lang, mode: THEMES[0] });
      await page.goto(`${BASE}/login`);
      await login(page, tr, lang, ADMIN_USER, ADMIN_PW);
      await page.waitForURL((u) => !u.pathname.startsWith('/login'));
      ok(`[${slug}/${lang}] signed in as ${ADMIN_USER}`);
      for (const theme of THEMES) {
        if (theme !== THEMES[0]) await setTheme(page, tr, lang, theme);
        const ctx = {
          page,
          lang,
          theme,
          base: BASE,
          t: (key, vars) => tr(lang, key, vars),
          nav: (key) => nav(page, tr, lang, key, { check: (c, m) => check(c, `[${slug}/${lang}] ${m}`) }),
          shot: (step) => shot(page, OUT, `${slug}-${lang}-${theme}-${step}`, { base: BASE }),
          ok: (msg) => ok(`[${slug}/${lang}/${theme}] ${msg}`),
          check: (cond, msg) => check(cond, `[${slug}/${lang}/${theme}] ${msg}`),
        };
        await run(ctx);
      }
      assertNoPageErrors(`[${slug}/${lang}]`);
      await close();
    }
  }
  console.log(`\nSHOTS OK (${results.length} checks, ${SCREENS.length} screen(s) x ${LANGS.length} lang(s) x ${THEMES.length} theme(s))`);
} catch (e) {
  console.error(`\nSHOTS FAILED: ${e.message}`);
  failed = true;
} finally {
  await browser.close();
}
process.exit(failed ? 1 : 0);
