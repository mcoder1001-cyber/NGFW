// test/topology/nat44-ei-64-66-nptv6/shots.mjs — F-nat44-ei-64-66-nptv6 evidence: screenshots (en, fa/RTL) of the NAT
// page's EI, NAT64, NAT66 and NPTv6 tabs against the real stack (TestNatEI6466Screenshots starts it: API + agent + VPP
// with live sessions).
//
//   node shots.mjs <baseUrl> <outDir> <adminPasswordFile> <tab,tab,…>
//
// Nothing is installed: VRX_PLAYWRIGHT_CORE is the path of an existing playwright-core package (the npx cache) and
// VRX_CHROME a Chrome-for-Testing headless shell (its libraries via LD_LIBRARY_PATH), as in P07a/P07b/P08 and
// F-nat44-ed-sessions. Test code only — not part of the product and not in any pnpm package.
import { mkdirSync, readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const require = createRequire(import.meta.url);
const { chromium } = require(process.env.VRX_PLAYWRIGHT_CORE);
const [BASE, OUT, PW_FILE, TABS] = process.argv.slice(2);
const PW = readFileSync(PW_FILE, 'utf8').trim();
const LOCALES = join(dirname(fileURLToPath(import.meta.url)), '../../../apps/web/src/locales');
const L = (lang, ns) => JSON.parse(readFileSync(join(LOCALES, lang, `${ns}.json`), 'utf8'));
const flat = (s) => s.replace(/\s+/g, ' ').trim();
mkdirSync(OUT, { recursive: true });

// the element that proves a tab shows live data (its text must hold a digit and not be the loading text)
const LIVE = { ei: 'nat-ei-sessions-total', nat64: 'nat64-sessions-total' };

const browser = await chromium.launch({ executablePath: process.env.VRX_CHROME, args: ['--no-sandbox'] });
let failures = 0;
for (const lang of ['en', 'fa']) {
  const auth = L(lang, 'auth');
  const ns = L(lang, 'nat44-ei-64-66-nptv6');
  const ctx = await browser.newContext({ viewport: { width: 1680, height: 1100 } });
  await ctx.addInitScript((s) => localStorage.setItem('vrx.ui.settings', JSON.stringify(s)), {
    mode: 'light',
    lang,
    persianDigits: false,
    dense: true,
  });
  const page = await ctx.newPage();
  const errors = [];
  page.on('pageerror', (e) => errors.push(String(e)));
  await page.goto(`${BASE}/login`);
  await page.getByLabel(auth.username, { exact: false }).fill('admin');
  await page.getByLabel(auth.passwordLabel, { exact: false }).fill(PW);
  await page.getByRole('button', { name: auth.signIn }).click();
  await page.waitForURL((u) => !u.pathname.startsWith('/login'));
  const rtl = lang === 'fa' ? '-rtl' : '';
  const dir = async () => page.evaluate(() => `${document.documentElement.dir}/${document.documentElement.lang}`);

  for (const tab of TABS.split(',')) {
    await page.goto(`${BASE}/firewall/nat?tab=${tab}`);
    await page.getByRole('tab', { name: ns.tab[tab], selected: true }).waitFor({ timeout: 30000 });
    let live = '-';
    if (LIVE[tab]) {
      await page.waitForFunction(
        ([id, loading]) => {
          const el = document.querySelector(`[data-testid="${id}"]`);
          return el && el.textContent && !el.textContent.includes(loading) && /\d/.test(el.textContent);
        },
        [LIVE[tab], ns.loading],
        { timeout: 30000 },
      );
      live = flat(await page.getByTestId(LIVE[tab]).innerText());
    }
    if (tab === 'nptv6') {
      await page.getByRole('table', { name: ns.nptv6.running }).waitFor({ timeout: 30000 });
      live = flat(await page.getByRole('table', { name: ns.nptv6.running }).innerText());
    }
    if (tab === 'nat66') {
      const t = page.getByRole('table', { name: ns.nat66.mappings });
      await t.waitFor({ timeout: 30000 });
      await page.waitForTimeout(1500);
      live = flat(await t.innerText());
    }
    await page.waitForTimeout(1500);
    const shot = `nat-${tab}-${lang}${rtl}.png`;
    await page.screenshot({ path: join(OUT, shot), fullPage: true });
    console.log(`${shot}  html dir/lang=${await dir()}  live=${JSON.stringify(live)}  pageErrors=${errors.length}`);

    if (tab === 'ei' && lang === 'en') {
      // the kill's confirm dialog (cancelled — the topology test kills through the API)
      const grid = page.getByRole('grid', { name: ns.ei.sessions });
      await grid.getByRole('button', { name: /^Delete session / }).first().click();
      const dialog = page.getByRole('dialog', { name: ns.kill.title });
      await dialog.waitFor();
      await page.waitForTimeout(500);
      const k = 'nat-ei-kill-dialog-en.png';
      await page.screenshot({ path: join(OUT, k) });
      console.log(`${k}  tuple=${JSON.stringify(flat(await page.getByTestId('nat-ei-kill-tuple').innerText()))}  pageErrors=${errors.length}`);
      await dialog.getByRole('button', { name: ns.cancel }).click();
    }
  }
  if (errors.length > 0) {
    console.log(`[${lang}] page errors: ${JSON.stringify(errors)}`);
    failures++;
  }
  await ctx.close();
}
await browser.close();
process.exit(failures === 0 ? 0 : 1);
