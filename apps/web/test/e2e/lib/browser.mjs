// apps/web/test/e2e/lib/browser.mjs — headless Chrome, the way P07a/P07b/P08 already drove it. Playwright is
// deliberately NOT a workspace dependency (00-CONTEXT: `pnpm test` is unit-only and packages may not be installed on
// the shared host), so every script loads `playwright-core` from VRX_PLAYWRIGHT_CORE (an existing install, e.g. the
// npx cache) and a Chrome-for-Testing binary from VRX_CHROME (its shared libraries via LD_LIBRARY_PATH — nothing is
// installed system-wide). This file adds no dependency of its own.
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);

export function loadChromium() {
  return require(process.env.VRX_PLAYWRIGHT_CORE ?? 'playwright-core').chromium;
}

/** `opts` are forwarded to `chromium.launch()` (e.g. `{ args: ['--no-sandbox'] }`) on top of the fixed defaults. */
export async function launchBrowser(opts = {}) {
  const chromium = loadChromium();
  return chromium.launch({ executablePath: process.env.VRX_CHROME, headless: true, ...opts });
}

/**
 * A fresh browser context + page seeded with the app's own UI settings (`localStorage` key `vrx.ui.settings`,
 * `apps/web/src/settings/storage.ts`) BEFORE the first load, so the app never flashes its default language/theme and
 * a script never has to race the settings popover just to get to a starting state. `mode` is the initial theme
 * ('light' | 'dark' | 'system'); use `lib/theme.mjs`'s `setTheme`/`setLanguage` to change it at runtime without a
 * reload (cheaper than logging in again per combination).
 *
 * Also collects every `pageerror` (uncaught exception in the page) — a script must call `assertNoPageErrors()` before
 * declaring a run green; screenshots taken while the page silently threw are not evidence of anything.
 */
export async function newPage(browser, { lang, mode = 'light', persianDigits = false, dense = true, viewport = { width: 1366, height: 860 } } = {}) {
  const ctx = await browser.newContext({ viewport, deviceScaleFactor: 1 });
  await ctx.addInitScript((s) => localStorage.setItem('vrx.ui.settings', JSON.stringify(s)), { mode, lang, persianDigits, dense });
  const page = await ctx.newPage();
  const pageErrors = [];
  page.on('pageerror', (e) => {
    pageErrors.push(String(e));
    console.log(`pageerror ${e}`);
  });
  return {
    ctx,
    page,
    pageErrors,
    /** Throws (failing the run) if any `pageerror` fired on this page since it was created. */
    assertNoPageErrors(context = '') {
      if (pageErrors.length > 0) throw new Error(`${context ? `${context}: ` : ''}${pageErrors.length} page error(s): ${pageErrors.join(' | ')}`);
    },
    close: () => ctx.close(),
  };
}
