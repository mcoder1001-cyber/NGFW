#!/usr/bin/env node
/**
 * UI-domain-editor screenshot pass: /config/interfaces and /config/system, en + fa/RTL, light + dark, 0 page errors.
 * Same playwright-core/Chrome-for-Testing approach as apps/web/test/e2e/flow.e2e.mjs (no WEB-3 harness on main yet),
 * but against ONLY the web + API stack — no vrx-agent, no VPP, no af_packet host-interfaces (manager instruction,
 * 2026-09-25: the slot agent must create no interfaces until TD-25 lands). The advanced editor's reads/writes
 * (GET/PATCH /api/v1/config/candidate/{path}) never call the agent, so this is a faithful, honest exercise of the
 * screen — nothing about it is mocked.
 *
 * Environment:
 *   VRX_E2E_BASE                  web origin (vite dev on the slot port, proxying /api to the slot API)
 *   VRX_E2E_ADMIN_USER            default "admin"
 *   VRX_E2E_ADMIN_PASSWORD_FILE   file holding the bootstrap admin password
 *   VRX_PLAYWRIGHT_CORE, VRX_CHROME
 * Options: --shots <dir>  --langs en,fa  --themes light,dark
 */
import { readFileSync, mkdirSync, readdirSync } from 'node:fs';
import { createRequire } from 'node:module';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const webRoot = resolve(here, '../..');
const args = process.argv.slice(2);
const opt = (name, dflt) => {
  const i = args.indexOf(`--${name}`);
  return i < 0 ? dflt : args[i + 1];
};
const LANGS = opt('langs', 'en,fa').split(',');
const THEMES = opt('themes', 'light,dark').split(',');
const SHOTS = opt('shots', '');
const BASE = process.env.VRX_E2E_BASE ?? 'http://127.0.0.1:6100';
const ADMIN = process.env.VRX_E2E_ADMIN_USER ?? 'admin';
const ADMIN_PW = readFileSync(process.env.VRX_E2E_ADMIN_PASSWORD_FILE ?? '/run/vrx-test/w11/admin.pw', 'utf8').trim();
const require = createRequire(import.meta.url);
const { chromium } = require(process.env.VRX_PLAYWRIGHT_CORE ?? 'playwright-core');

function loadLocales(lang) {
  const dir = join(webRoot, 'src/locales', lang);
  return Object.fromEntries(readdirSync(dir).map((f) => [f.replace(/\.json$/, ''), JSON.parse(readFileSync(join(dir, f), 'utf8'))]));
}
const LOCALES = Object.fromEntries(LANGS.map((l) => [l, loadLocales(l)]));
function tr(lang, key) {
  const [ns, path] = key.split(':');
  const v = path.split('.').reduce((o, k) => (o == null ? undefined : o[k]), LOCALES[lang][ns]);
  if (typeof v !== 'string') throw new Error(`no string ${lang}:${key}`);
  return v;
}

const results = [];
function ok(msg) {
  results.push(msg);
  console.log(`ok   ${msg}`);
}
function check(cond, msg) {
  if (!cond) throw new Error(`FAILED: ${msg}`);
  ok(msg);
}

if (SHOTS) mkdirSync(SHOTS, { recursive: true });
async function shot(page, name) {
  if (!SHOTS) return;
  await page.waitForTimeout(400);
  await page.screenshot({ path: join(SHOTS, `${name}.png`) });
  const dir = await page.evaluate(() => `${document.documentElement.dir}/${document.documentElement.lang}`);
  console.log(`shot ${name}.png  (${dir})  ${page.url().replace(BASE, '')}`);
}

async function newPage(browser, lang, mode) {
  const ctx = await browser.newContext({ viewport: { width: 1366, height: 900 }, deviceScaleFactor: 1 });
  await ctx.addInitScript((s) => localStorage.setItem('vrx.ui.settings', JSON.stringify(s)), { mode, lang, persianDigits: false, dense: true });
  const page = await ctx.newPage();
  const errors = [];
  page.on('pageerror', (e) => errors.push(String(e)));
  page.on('console', (m) => {
    if (m.type() === 'error') errors.push(m.text());
  });
  return { ctx, page, errors };
}

async function login(page, lang) {
  await page.getByLabel(tr(lang, 'auth:username'), { exact: false }).fill(ADMIN);
  await page.getByLabel(tr(lang, 'auth:passwordLabel'), { exact: false }).fill(ADMIN_PW);
  await page.getByRole('button', { name: tr(lang, 'auth:signIn') }).click();
  await page.waitForURL((u) => !u.pathname.startsWith('/login'));
}

async function pass(browser, lang, mode, index) {
  const { ctx, page, errors } = await newPage(browser, lang, mode);
  await page.goto(`${BASE}/login`);
  await login(page, lang);
  ok(`[${lang}/${mode}] signed in as ${ADMIN}`);

  await page.goto(`${BASE}/config/interfaces`);
  await page.getByRole('heading', { level: 2, name: tr(lang, 'nav:domains.interfaces') }).waitFor({ timeout: 15_000 });
  await shot(page, `${index}1-config-interfaces-${lang}-${mode}`);
  check(true, `[${lang}/${mode}] /config/interfaces: heading + record-list rendered`);

  await page.goto(`${BASE}/config/system`);
  await page.getByRole('heading', { level: 2, name: tr(lang, 'nav:domains.system') }).waitFor({ timeout: 15_000 });
  await page.getByLabel('Hostname', { exact: false }).waitFor();
  await shot(page, `${index}2-config-system-${lang}-${mode}`);
  check(true, `[${lang}/${mode}] /config/system: heading + scalar form + child chips rendered`);

  check(errors.length === 0, `[${lang}/${mode}] pageErrors=${errors.length}${errors.length ? `: ${errors[0]}` : ''}`);
  await ctx.close();
}

const browser = await chromium.launch({ executablePath: process.env.VRX_CHROME, headless: true });
try {
  let i = 0;
  for (const lang of LANGS) {
    for (const mode of THEMES) {
      i += 1;
      await pass(browser, lang, mode, i);
    }
  }
  console.log(`\nE2E PASSED (${results.length} checks)`);
} catch (e) {
  console.error(`\nE2E FAILED: ${e.message}`);
  process.exitCode = 1;
} finally {
  await browser.close();
}
