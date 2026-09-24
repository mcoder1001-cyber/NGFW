// test/topology/nat44-ed-sessions/shots.mjs — F-nat44-ed-sessions evidence: screenshots (en, fa/RTL) of the NAT page
// against the real stack (TestNat44EdScreenshots starts it: API + agent + VPP with ≥ 2 000 live sessions).
//
//   node shots.mjs <baseUrl> <outDir> <adminPasswordFile>
//
// Nothing is installed: VRX_PLAYWRIGHT_CORE is the path of an existing playwright-core package (the npx cache) and
// VRX_CHROME a Chrome-for-Testing headless shell (its libraries via LD_LIBRARY_PATH), as in P07a/P07b/P08. Test code
// only — not part of the product and not in any pnpm package.
import { mkdirSync, readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const require = createRequire(import.meta.url);
const { chromium } = require(process.env.VRX_PLAYWRIGHT_CORE);
const [BASE, OUT, PW_FILE] = process.argv.slice(2);
const PW = readFileSync(PW_FILE, 'utf8').trim();
const LOCALES = join(dirname(fileURLToPath(import.meta.url)), '../../../apps/web/src/locales');
const L = (lang, ns) => JSON.parse(readFileSync(join(LOCALES, lang, `${ns}.json`), 'utf8'));
const flat = (s) => s.replace(/\s+/g, ' ').trim();
mkdirSync(OUT, { recursive: true });

const browser = await chromium.launch({ executablePath: process.env.VRX_CHROME, args: ['--no-sandbox'] });
let failures = 0;
for (const lang of ['en', 'fa']) {
  const auth = L(lang, 'auth');
  const nat = L(lang, 'nat44-ed-sessions');
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 1100 } });
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

  // Sessions: server-side paged grid with ≥ 2 000 sessions
  await page.goto(`${BASE}/firewall/nat?tab=sessions`);
  const total = page.getByTestId('nat-sessions-total');
  await page.waitForFunction(
    (loading) => {
      const el = document.querySelector('[data-testid="nat-sessions-total"]');
      return el && el.textContent && !el.textContent.includes(loading) && /\d/.test(el.textContent);
    },
    nat.loading,
    { timeout: 30000 },
  );
  const grid = page.getByRole('grid', { name: nat.sessions.title });
  await grid.waitFor();
  await page.waitForTimeout(1200);
  const rows = await grid.getByRole('row').count();
  let shot = `nat-sessions-${lang}${rtl}.png`;
  await page.screenshot({ path: join(OUT, shot) });
  console.log(`${shot}  html dir/lang=${await dir()}  total=${JSON.stringify(flat(await total.innerText()))}  gridRows=${rows}  pageErrors=${errors.length}`);

  if (lang === 'en') {
    // kill: the confirm dialog with the 5-tuple (cancelled — the topology test kills through the API)
    await grid.getByRole('button', { name: /^Delete session / }).first().click();
    const dialog = page.getByRole('dialog', { name: nat.kill.title });
    await dialog.waitFor();
    await page.waitForTimeout(500);
    shot = 'nat-sessions-kill-dialog-en.png';
    await page.screenshot({ path: join(OUT, shot) });
    console.log(`${shot}  tuple=${JSON.stringify(flat(await page.getByTestId('nat-kill-tuple').innerText()))}  pageErrors=${errors.length}`);
    await dialog.getByRole('button', { name: nat.cancel }).click();
  }

  // Pools: utilisation bar
  await page.goto(`${BASE}/firewall/nat?tab=pools`);
  const bar = page.getByTestId('pool-utilisation-pat');
  await bar.waitFor({ timeout: 30000 });
  await page.waitForTimeout(1200);
  shot = `nat-pools-${lang}${rtl}.png`;
  await page.screenshot({ path: join(OUT, shot) });
  console.log(`${shot}  html dir/lang=${await dir()}  bar=${JSON.stringify(await bar.getAttribute('aria-valuenow'))}  pageErrors=${errors.length}`);

  // Outbound and Static & port forwards
  for (const tab of ['outbound', 'static']) {
    await page.goto(`${BASE}/firewall/nat?tab=${tab}`);
    await page.getByRole('tab', { name: nat.tab[tab] }).waitFor();
    await page.waitForTimeout(1500);
    shot = `nat-${tab}-${lang}${rtl}.png`;
    await page.screenshot({ path: join(OUT, shot) });
    console.log(`${shot}  html dir/lang=${await dir()}  status=${JSON.stringify(flat(await page.getByTestId('nat-status').first().innerText()))}  pageErrors=${errors.length}`);
  }
  if (errors.length > 0) {
    console.log(`[${lang}] page errors: ${JSON.stringify(errors)}`);
    failures++;
  }
  await ctx.close();
}
await browser.close();
process.exit(failures === 0 ? 0 : 1);
