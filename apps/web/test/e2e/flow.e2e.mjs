#!/usr/bin/env node
/**
 * P07b end-to-end flow in a real browser against a REAL stack (P06 API + P05 agent on a worker slot; nothing mocked):
 *   login → Users (edit) → pending-change bar → Commit… with auto-revert → countdown → Confirm → Revisions shows it →
 *   rollback (with auto-revert, confirmed) → sign out → readonly user sees disabled actions.
 *   --revert additionally commits with a 1-minute window and does NOT confirm: the device must revert and the UI must say so.
 *
 * Not part of `pnpm test` (unit-only, 00-CONTEXT): Playwright is not a workspace dependency and packages may not be
 * installed on the shared host, so the script loads `playwright-core` from VRX_PLAYWRIGHT_CORE and drives the
 * Chrome binary in VRX_CHROME. Environment:
 *   VRX_E2E_BASE                  web origin (vite dev/preview on the slot port, proxying /api to the slot API)
 *   VRX_E2E_ADMIN_USER            default "admin"
 *   VRX_E2E_ADMIN_PASSWORD_FILE   file holding the bootstrap admin password (never pass it on the command line)
 *   VRX_E2E_ARGON2_FROM           package dir that has @node-rs/argon2 (default apps/api) — hashes the readonly user's
 *                                 random per-run password the same way the API does (argon2id PHC)
 *   VRX_PLAYWRIGHT_CORE, VRX_CHROME
 *   --keyboard runs login → commit (with auto-revert) → confirm using the keyboard only (Tab / Enter / typing).
 *   --secret   changes ONLY the readonly user's password hash (write-only, redacted in every diff) → the bar must still
 *              appear with "password changed", the commit must go through and the new password must work (review H1).
 *   --blackhole silent-drop test (review M3/M2/M1). Needs VRX_E2E_PROXY_PORT (the port vite proxies /api to, e.g. the
 *              slot port 3100) and VRX_E2E_UPSTREAM (host:port of the API, e.g. 127.0.0.1:3101): this script runs a TCP
 *              proxy in-process between them and, mid-countdown, swallows all traffic without closing a socket (what a
 *              commit that cuts the operator's own route looks like). No iptables, no system change.
 * Options: --langs en,fa  --shots <dir>  --revert  --keyboard  --secret  --blackhole
 * Test users carry the slot prefix (VRX_TEST_PREFIX, default w1): `<prefix>ro`.
 */
import { randomBytes } from 'node:crypto';
import net from 'node:net';
import { mkdirSync, readFileSync, readdirSync } from 'node:fs';
import { createRequire } from 'node:module';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const webRoot = resolve(here, '../..');
const repo = resolve(webRoot, '../..');
const args = process.argv.slice(2);
const opt = (name, dflt) => {
  const i = args.indexOf(`--${name}`);
  return i < 0 ? dflt : args[i + 1];
};
const LANGS = opt('langs', 'en,fa').split(',');
const SHOTS = opt('shots', '');
const REVERT = args.includes('--revert');
const KEYBOARD = args.includes('--keyboard');
const SECRET = args.includes('--secret');
const BLACKHOLE = args.includes('--blackhole');
const BASE = process.env.VRX_E2E_BASE ?? 'http://127.0.0.1:5100';
const ADMIN = process.env.VRX_E2E_ADMIN_USER ?? 'admin';
const ADMIN_PW = readFileSync(process.env.VRX_E2E_ADMIN_PASSWORD_FILE ?? '/run/vrx-test/w1/admin.pw', 'utf8').trim();
const PREFIX = process.env.VRX_TEST_PREFIX ?? 'w1';
const RO_USER = `${PREFIX}ro`;
let RO_PW = randomBytes(18).toString('base64url'); // per run, memory only
const require = createRequire(import.meta.url);
const { chromium } = require(process.env.VRX_PLAYWRIGHT_CORE ?? 'playwright-core');
const argon2 = createRequire(join(resolve(repo, process.env.VRX_E2E_ARGON2_FROM ?? 'apps/api'), 'package.json'))('@node-rs/argon2');
const hashOf = (pw) => argon2.hash(pw, { memoryCost: 19456, timeCost: 2, parallelism: 1 });
let RO_HASH = await hashOf(RO_PW);

/** The app's own locale files, so every lookup matches what the UI renders in that language. */
function loadLocales(lang) {
  const dir = join(webRoot, 'src/locales', lang);
  return Object.fromEntries(readdirSync(dir).map((f) => [f.replace(/\.json$/, ''), JSON.parse(readFileSync(join(dir, f), 'utf8'))]));
}
const LOCALES = Object.fromEntries(LANGS.map((l) => [l, loadLocales(l)]));
function tr(lang, key, vars = {}) {
  const [ns, path] = key.split(':');
  let v = path.split('.').reduce((o, k) => (o == null ? undefined : o[k]), LOCALES[lang][ns]);
  if (typeof v !== 'string') throw new Error(`no string ${lang}:${key}`);
  for (const [k, val] of Object.entries(vars)) v = v.replaceAll(`{{${k}}}`, String(val));
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
  await page.waitForTimeout(400); // let transitions settle
  await page.screenshot({ path: join(SHOTS, `${name}.png`) });
  const dir = await page.evaluate(() => `${document.documentElement.dir}/${document.documentElement.lang}`);
  console.log(`shot ${name}.png  (${dir})  ${page.url().replace(BASE, '')}`);
}

async function newPage(browser, lang) {
  const ctx = await browser.newContext({ viewport: { width: 1366, height: 860 }, deviceScaleFactor: 1 });
  await ctx.addInitScript((s) => {
    localStorage.setItem('vrx.ui.settings', JSON.stringify(s));
  }, { mode: 'light', lang, persianDigits: false, dense: true });
  const page = await ctx.newPage();
  page.on('pageerror', (e) => console.log(`pageerror ${e}`));
  return { ctx, page };
}

async function login(page, lang, user, pw) {
  await page.getByLabel(tr(lang, 'auth:username'), { exact: false }).fill(user);
  await page.getByLabel(tr(lang, 'auth:passwordLabel'), { exact: false }).fill(pw);
  await page.getByRole('button', { name: tr(lang, 'auth:signIn') }).click();
}

async function signOut(page, lang) {
  await page.getByTestId('user-menu').click();
  await page.getByRole('menuitem', { name: tr(lang, 'auth:signOut') }).click();
  await page.waitForURL(/\/login/);
}

/** Nav group of each entry the flow visits: groups start collapsed, so the owning group is opened first (D-117). */
const NAV_GROUP_OF = { users: 'system', revisions: 'system' };

async function nav(page, lang, key) {
  const menu = page.getByRole('navigation', { name: tr(lang, 'common:menu.navigation') });
  const group = NAV_GROUP_OF[key];
  if (group) {
    const header = menu.getByRole('button', { name: tr(lang, `nav:groups.${group}`) });
    if ((await header.getAttribute('aria-expanded')) === 'false') {
      await header.click();
      check((await header.getAttribute('aria-expanded')) === 'true', `[${lang}] nav: opened the collapsed "${tr(lang, `nav:groups.${group}`)}" group to reach ${key}`);
    }
  }
  const link = menu.getByRole('link', { name: tr(lang, `nav:${key}`) });
  await link.click();
  // the router navigates asynchronously: return only once this entry is the current page
  await link.and(page.locator('[aria-current="page"]')).waitFor();
}

/** Commit through the bar's dialog; `confirmMinutes` null = without auto-revert. Returns after the dialog closed or showed its result. */
async function commitViaDialog(page, lang, comment, confirmMinutes, shotName) {
  await page.getByTestId('pending-bar').getByRole('button', { name: tr(lang, 'config:bar.commit') }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByTestId('validate-ok').waitFor({ timeout: 30_000 });
  ok(`[${lang}] commit dialog: server validation passed (schema → semantic → agent DryRun)`);
  await dialog.getByLabel(tr(lang, 'config:commit.comment')).fill(comment);
  const box = dialog.getByRole('checkbox');
  check((await box.isChecked()) === true, `[${lang}] auto-revert checkbox is ON by default`);
  if (confirmMinutes === null) await box.uncheck();
  else await dialog.getByLabel(tr(lang, 'config:commit.minutesLabel')).fill(String(confirmMinutes));
  if (shotName) await shot(page, shotName);
  await dialog.getByRole('button', { name: tr(lang, confirmMinutes === null ? 'config:commit.submit' : 'config:commit.submitConfirm') }).click();
}

async function editRoUser(page, lang, fullName) {
  await nav(page, lang, 'users');
  await page.getByRole('heading', { level: 2, name: tr(lang, 'users:title') }).waitFor();
  const addMe = page.getByRole('button', { name: tr(lang, 'users:empty.addMe', { user: ADMIN }) });
  await page.waitForTimeout(800);
  if (await addMe.isVisible()) {
    await addMe.click();
    await page.getByRole('row', { name: new RegExp(ADMIN) }).waitFor();
    ok(`[${lang}] users: added the bootstrap admin to the configuration (its hash is kept by the API)`);
  }
  const exists = (await page.getByRole('row').filter({ hasText: RO_USER }).count()) > 0;
  if (exists) await page.getByRole('button', { name: tr(lang, 'users:edit', { user: RO_USER }) }).click();
  else await page.getByRole('button', { name: tr(lang, 'users:add') }).click();
  const dialog = page.getByRole('dialog');
  if (!exists) {
    await dialog.getByLabel(tr(lang, 'users:field.username.title'), { exact: false }).fill(RO_USER);
    await dialog.getByLabel(tr(lang, 'users:field.role.title'), { exact: false }).click();
    await page.getByRole('option', { name: tr(lang, 'users:field.role.enum.readonly') }).click();
  }
  await dialog.getByLabel(tr(lang, 'users:field.fullName.title'), { exact: false }).fill(fullName);
  await dialog.getByLabel(tr(lang, 'users:field.passwordHash.title'), { exact: false }).fill(RO_HASH);
  await dialog.getByRole('button', { name: tr(lang, 'users:saveToCandidate') }).click();
  await dialog.waitFor({ state: 'hidden' });
  ok(`[${lang}] users: ${exists ? 'edited' : 'added'} ${RO_USER} in the candidate (PATCH /config/management)`);
}

async function confirmBanner(page, lang, shotName) {
  const banner = page.getByTestId('confirm-banner');
  await banner.waitFor({ timeout: 30_000 });
  const t1 = await banner.innerText();
  await page.waitForTimeout(2200);
  const t2 = await banner.innerText();
  check(t1 !== t2, `[${lang}] countdown banner is ticking ("${t1.split('\n')[1] ?? t1}" → "${t2.split('\n')[1] ?? t2}")`);
  if (shotName) await shot(page, shotName);
  await banner.getByRole('button', { name: tr(lang, 'config:confirm.button') }).click();
  const outcome = page.getByTestId('confirm-outcome');
  await outcome.waitFor({ timeout: 30_000 });
  const text = await outcome.innerText();
  check(text.includes(tr(lang, 'config:confirm.outcome.confirmed.title')), `[${lang}] confirmed: "${text.replace(/\n/g, ' ')}"`);
  return text;
}

async function adminPass(browser, lang, index) {
  const { ctx, page } = await newPage(browser, lang);
  const n = (i) => `${String(index * 10 + i).padStart(2, '0')}`;
  await page.goto(`${BASE}/system/users`);
  await page.waitForURL(/\/login\?next=/);
  check(true, `[${lang}] protected route redirects to ${page.url().replace(BASE, '')}`);
  await shot(page, `${n(1)}-login-${lang}`);
  await login(page, lang, ADMIN, ADMIN_PW);
  await page.waitForURL(/\/system\/users$/);
  ok(`[${lang}] signed in as ${ADMIN}, returned to /system/users`);

  // baseline revision (only on an empty box): the admin + the readonly user, committed without auto-revert
  let revisions = await apiRevisions(page);
  if (revisions.total === 0) {
    await editRoUser(page, lang, `E2E baseline ${lang}`);
    await commitViaDialog(page, lang, 'e2e: baseline users', null, null);
    const result = page.getByRole('dialog').getByTestId('commit-result');
    await result.waitFor({ timeout: 30_000 });
    ok(`[${lang}] commit without auto-revert → "${(await result.innerText()).replace(/\n/g, ' ')}"`);
    await page.getByRole('dialog').getByRole('button', { name: tr(lang, 'config:close') }).click();
  }

  await editRoUser(page, lang, `E2E ${lang} ${new Date().toISOString().slice(11, 19)}`);
  const bar = page.getByTestId('pending-bar');
  await bar.waitFor();
  const count = await page.getByTestId('pending-count').innerText();
  const owner = await page.getByTestId('lock-owner').innerText();
  check(count.length > 0 && owner.length > 0, `[${lang}] pending-change bar: "${count}", "${owner}"`);
  await nav(page, lang, 'users');
  await shot(page, `${n(2)}-pending-bar-users-admin-${lang}`);

  await commitViaDialog(page, lang, `e2e ${lang}: rename ${RO_USER}`, 2, `${n(3)}-commit-dialog-${lang}`);
  await confirmBanner(page, lang, `${n(4)}-confirm-countdown-${lang}`);

  await nav(page, lang, 'revisions');
  await page.getByRole('heading', { level: 2, name: tr(lang, 'revisions:title') }).waitFor();
  revisions = await apiRevisions(page);
  const latest = revisions.items[0];
  check(latest.comment === `e2e ${lang}: rename ${RO_USER}`, `[${lang}] revisions: newest is #${latest.id} "${latest.comment}" by ${latest.author}`);
  const rows = page.getByRole('row');
  await rows.filter({ hasText: latest.comment }).first().waitFor();
  await rows.filter({ hasText: latest.comment }).first().getByRole('button', { name: tr(lang, 'revisions:changes') }).click();
  await page.getByTestId('revision-diff').waitFor();
  await page.getByTestId('revision-diff').getByText('/management/users', { exact: false }).first().waitFor();
  ok(`[${lang}] revisions: diff #${latest.parentId} → #${latest.id} shows /management/users changes`);
  await shot(page, `${n(5)}-revisions-diff-${lang}`);

  const target = revisions.items[1];
  await rows.filter({ hasText: target.comment || `#${target.id}` }).first().getByRole('button', { name: tr(lang, 'revisions:rollback.button') }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByText('/management/users', { exact: false }).first().waitFor();
  check((await dialog.getByRole('checkbox').isChecked()) === true, `[${lang}] rollback dialog: auto-revert ON by default, shows what changes`);
  await shot(page, `${n(6)}-rollback-dialog-${lang}`);
  await dialog.getByRole('button', { name: tr(lang, 'revisions:rollback.submit', { id: target.id }) }).click();
  await confirmBanner(page, lang, null);
  revisions = await apiRevisions(page);
  check(revisions.items[0].kind === 'rollback', `[${lang}] rollback confirmed → revision #${revisions.items[0].id} kind=${revisions.items[0].kind} "${revisions.items[0].comment}"`);
  await page.getByRole('row').filter({ hasText: revisions.items[0].comment }).first().waitFor();
  await shot(page, `${n(7)}-revisions-after-rollback-${lang}`);
  await nav(page, lang, 'users');
  await page.getByRole('row').filter({ hasText: RO_USER }).first().waitFor();
  await shot(page, `${n(8)}-users-admin-${lang}`);

  await signOut(page, lang);
  await login(page, lang, RO_USER, RO_PW);
  await page.waitForURL((u) => !u.pathname.startsWith('/login'));
  ok(`[${lang}] signed in as readonly ${RO_USER} (argon2id hash set through the Users form)`);
  // through the menu from the dashboard, not a URL: sign-in returns to /system/users (`next`), so start at '/', where System
  // is collapsed — this walks the collapsed-group path of nav()
  await page.goto(`${BASE}/`);
  await page.getByRole('heading', { level: 2, name: tr(lang, 'nav:dashboard') }).waitFor();
  await nav(page, lang, 'users');
  await page.getByTestId('users-readonly').waitFor();
  check(await page.getByRole('button', { name: tr(lang, 'users:add') }).isDisabled(), `[${lang}] readonly: "Add user" disabled`);
  check(await page.getByRole('button', { name: tr(lang, 'users:edit', { user: RO_USER }) }).isDisabled(), `[${lang}] readonly: edit disabled`);
  await shot(page, `${n(9)}-users-readonly-${lang}`);
  await nav(page, lang, 'revisions');
  await page.getByRole('heading', { level: 2, name: tr(lang, 'revisions:title') }).waitFor();
  await page.getByRole('row').filter({ hasText: 'rollback to revision' }).first().waitFor();
  await shot(page, `${n(10)}-revisions-readonly-${lang}`);
  const rb = page.getByRole('button', { name: tr(lang, 'revisions:rollback.button') });
  const all = await rb.count();
  let disabled = 0;
  for (let i = 0; i < all; i++) if (await rb.nth(i).isDisabled()) disabled++;
  check(all > 0 && disabled === all, `[${lang}] readonly: all ${all} rollback buttons disabled`);
  await ctx.close();
}

async function revertPass(browser, lang) {
  const { ctx, page } = await newPage(browser, lang);
  await page.goto(`${BASE}/login`);
  await login(page, lang, ADMIN, ADMIN_PW);
  await page.waitForURL((u) => !u.pathname.startsWith('/login'));
  await editRoUser(page, lang, `E2E revert ${new Date().toISOString().slice(11, 19)}`);
  const before = await apiRevisions(page);
  await commitViaDialog(page, lang, 'e2e: not confirmed on purpose', 1, null);
  await page.getByTestId('confirm-banner').waitFor({ timeout: 30_000 });
  ok(`[${lang}] committed with a 1-minute window; not confirming`);
  const outcome = page.getByTestId('confirm-outcome');
  await outcome.waitFor({ timeout: 120_000 });
  const text = await outcome.innerText();
  check(text.includes(tr(lang, 'config:confirm.outcome.reverted.title')), `[${lang}] after the deadline the UI reports "${text.replace(/\n/g, ' ')}"`);
  const after = await apiRevisions(page);
  check(after.total === before.total, `[${lang}] no revision was written for the unconfirmed commit (${before.total} → ${after.total})`);
  await shot(page, `30-auto-reverted-${lang}`);
  await page.getByTestId('pending-bar').getByRole('button', { name: tr(lang, 'config:bar.discard') }).click();
  await page.getByRole('dialog').getByRole('button', { name: tr(lang, 'config:discard.confirm') }).click();
  await page.getByTestId('pending-bar').waitFor({ state: 'hidden' });
  ok(`[${lang}] the kept candidate was discarded from the bar`);
  await ctx.close();
}

/** Press Tab until the focused element's accessible text matches, like a keyboard user would. */
async function tabTo(page, re, max = 60) {
  for (let i = 0; i < max; i++) {
    await page.keyboard.press('Tab');
    const label = await page.evaluate(() => {
      const el = document.activeElement;
      return el ? `${el.getAttribute('aria-label') ?? ''} ${el.textContent ?? ''}`.trim() : '';
    });
    if (re.test(label)) return i + 1;
  }
  throw new Error(`FAILED: no focusable element matching ${re} within ${max} Tab presses`);
}

async function keyboardPass(browser, lang) {
  const { ctx, page } = await newPage(browser, lang);
  await page.goto(`${BASE}/login`);
  await page.getByRole('button', { name: tr(lang, 'auth:signIn') }).waitFor();
  await page.keyboard.type(ADMIN); // the username field has focus on load
  await page.keyboard.press('Tab');
  await page.keyboard.type(ADMIN_PW);
  await page.keyboard.press('Enter');
  await page.waitForURL((u) => !u.pathname.startsWith('/login'));
  ok(`[${lang}] keyboard: signed in with typing + Tab + Enter only`);
  // setup (not part of the keyboard path): one uncommitted change through the API, as another screen would make it
  await page.evaluate(async (ro) => {
    const { accessToken } = await (await fetch('/api/v1/auth/refresh', { method: 'POST', credentials: 'include' })).json();
    const h = { authorization: `Bearer ${accessToken}`, 'content-type': 'application/json' };
    const users = (await (await fetch('/api/v1/config/candidate/management', { headers: h })).json()).users;
    for (const u of users) if (u.username === ro) u.fullName = `E2E keyboard ${Date.now()}`;
    await fetch('/api/v1/config/management', { method: 'PATCH', headers: h, body: JSON.stringify({ users }) });
  }, RO_USER);
  await page.getByTestId('pending-bar').waitFor();
  const t1 = await tabTo(page, new RegExp(`^${tr(lang, 'config:bar.commit')}$`));
  await page.keyboard.press('Enter');
  const dialog = page.getByRole('dialog');
  await dialog.getByTestId('validate-ok').waitFor({ timeout: 30_000 });
  const t2 = await tabTo(page, new RegExp(`^${tr(lang, 'config:commit.submitConfirm')}$`));
  await page.keyboard.press('Enter');
  await page.getByTestId('confirm-banner').waitFor({ timeout: 30_000 });
  await dialog.waitFor({ state: 'hidden' });
  const t3 = await tabTo(page, new RegExp(`^${tr(lang, 'config:confirm.button')}$`));
  await page.keyboard.press('Enter');
  const outcome = page.getByTestId('confirm-outcome');
  await outcome.waitFor({ timeout: 30_000 });
  check((await outcome.innerText()).includes(tr(lang, 'config:confirm.outcome.confirmed.title')), `[${lang}] keyboard: Commit… (${t1} Tabs) → Commit with auto-revert (${t2}) → Confirm (${t3}) → confirmed`);
  await ctx.close();
}

/**
 * TCP proxy that can go silent: in `drop` mode it keeps every socket open but forwards nothing (and accepts new
 * connections without ever answering) — no RST, no FIN, exactly like packets vanishing on a cut route.
 */
function silentProxy(listenPort, upstream) {
  const [upHost, upPort] = upstream.split(':');
  let dropping = false;
  const sockets = new Set();
  const server = net.createServer((client) => {
    sockets.add(client);
    client.on('error', () => {});
    client.on('close', () => sockets.delete(client));
    if (dropping) return; // accepted, never answered
    const up = net.connect(Number(upPort), upHost);
    sockets.add(up);
    up.on('error', () => client.destroy());
    up.on('close', () => {
      sockets.delete(up);
      if (!dropping) client.destroy();
    });
    client.on('close', () => {
      if (!dropping) up.destroy();
    });
    client.on('data', (d) => {
      if (!dropping) up.write(d);
    });
    up.on('data', (d) => {
      if (!dropping) client.write(d);
    });
  });
  return new Promise((resolveListen, rejectListen) => {
    server.once('error', rejectListen);
    server.listen(Number(listenPort), '127.0.0.1', () =>
      resolveListen({
        drop() {
          dropping = true;
        },
        /** The route is back: the dead connections are gone (as after a TCP timeout), new ones work. */
        restore() {
          dropping = false;
          for (const sock of sockets) sock.destroy();
          sockets.clear();
        },
        close: () => new Promise((r) => {
          for (const sock of sockets) sock.destroy();
          server.close(() => r());
        }),
      }),
    );
  });
}

async function secretPass(browser, lang) {
  const { ctx, page } = await newPage(browser, lang);
  await page.goto(`${BASE}/login`);
  await login(page, lang, ADMIN, ADMIN_PW);
  await page.waitForURL((u) => !u.pathname.startsWith('/login'));
  await nav(page, lang, 'users');
  await page.getByRole('button', { name: tr(lang, 'users:edit', { user: RO_USER }) }).click();
  const dialog = page.getByRole('dialog');
  const newPw = randomBytes(18).toString('base64url');
  await dialog.getByLabel(tr(lang, 'users:field.passwordHash.title'), { exact: false }).fill(await hashOf(newPw));
  await dialog.getByRole('button', { name: tr(lang, 'users:saveToCandidate') }).click();
  await dialog.waitFor({ state: 'hidden' });
  const serverDiff = await apiGet(page, '/api/v1/config/diff');
  ok(`[${lang}] secret: only ${RO_USER}'s password hash changed → server diff has ${serverDiff.changes.length} visible change(s)${serverDiff.changes.some((c) => c.redacted) ? ' (redacted entry from the API)' : ''}`);
  await page.getByTestId('pending-bar').waitFor({ timeout: 15_000 });
  await page.getByTestId('password-changed').first().waitFor();
  check(true, `[${lang}] secret: the bar appears ("${await page.getByTestId('pending-count').innerText()}") and the user row says "${tr(lang, 'users:pending.password')}"`);
  await shot(page, `40-password-only-bar-${lang}`);
  await page.getByTestId('pending-bar').getByRole('button', { name: tr(lang, 'config:bar.review') }).click();
  const review = page.getByRole('dialog');
  const text = await review.getByTestId('redacted-change').first().innerText();
  check(!text.includes('$argon2'), `[${lang}] secret: review shows "${text.replace(/\n/g, ' ')}" — no value`);
  await shot(page, `41-password-only-review-${lang}`);
  await review.getByRole('button', { name: tr(lang, 'config:close') }).click();
  await commitViaDialog(page, lang, `e2e: new password for ${RO_USER}`, null, null);
  const result = page.getByRole('dialog').getByTestId('commit-result');
  await result.waitFor({ timeout: 30_000 });
  ok(`[${lang}] secret: committed → "${(await result.innerText()).split('\n')[0]}"`);
  await page.getByRole('dialog').getByRole('button', { name: tr(lang, 'config:close') }).click();
  await page.getByTestId('pending-bar').waitFor({ state: 'hidden', timeout: 15_000 });
  await signOut(page, lang);
  await login(page, lang, RO_USER, newPw);
  await page.waitForURL((u) => !u.pathname.startsWith('/login'));
  check(true, `[${lang}] secret: ${RO_USER} signs in with the NEW password`);
  RO_PW = newPw;
  await ctx.close();
}

async function blackholePass(browser, lang, proxy) {
  const { ctx, page } = await newPage(browser, lang);
  await page.goto(`${BASE}/login`);
  await login(page, lang, ADMIN, ADMIN_PW);
  await page.waitForURL((u) => !u.pathname.startsWith('/login'));
  await editRoUser(page, lang, `E2E blackhole ${new Date().toISOString().slice(11, 19)}`);
  await commitViaDialog(page, lang, `e2e ${lang}: silent drop`, 2, null);
  const banner = page.getByTestId('confirm-banner');
  await banner.waitFor({ timeout: 30_000 });
  await page.getByRole('dialog').waitFor({ state: 'hidden' });
  proxy.drop();
  const t0 = Date.now();
  ok(`[${lang}] blackhole: traffic to the API now silently dropped (sockets stay open, nothing answers)`);
  await banner.getByText(tr(lang, 'config:confirm.reconnecting')).first().waitFor({ timeout: 30_000 });
  const t1 = await banner.innerText();
  check(true, `[${lang}] blackhole: banner says "${tr(lang, 'config:confirm.reconnecting')}" after ${((Date.now() - t0) / 1000).toFixed(1)} s`);
  await page.waitForTimeout(2200);
  const t2 = await banner.innerText();
  check(t1 !== t2 && (await banner.getByRole('button', { name: tr(lang, 'config:confirm.button') }).isDisabled()), `[${lang}] blackhole: countdown keeps running locally ("${t1.split('\n')[1]}" → "${t2.split('\n')[1]}"), Confirm disabled`);
  await shot(page, `50-silent-drop-reconnecting-${lang}`);
  await page.reload();
  const offline = page.getByTestId('offline-screen');
  await offline.waitFor({ timeout: 30_000 });
  await offline.getByTestId('confirm-banner').waitFor();
  check(!page.url().includes('/login'), `[${lang}] blackhole: reload during the drop → offline screen with the countdown ("${(await offline.getByTestId('confirm-banner').innerText()).split('\n')[1]}"), not /login`);
  await shot(page, `51-reload-during-drop-${lang}`);
  proxy.restore();
  ok(`[${lang}] blackhole: route restored`);
  const back = page.getByTestId('confirm-banner');
  await back.getByText(tr(lang, 'config:confirm.titleMine')).first().waitFor({ timeout: 45_000 });
  check(true, `[${lang}] blackhole: session restored without a login, banner: "${(await back.innerText()).split('\n')[1]}"`);
  await back.getByRole('button', { name: tr(lang, 'config:confirm.button') }).click();
  const outcome = page.getByTestId('confirm-outcome');
  await outcome.waitFor({ timeout: 30_000 });
  const applied = await outcome.getByTestId('commit-result').innerText();
  check(applied.includes(tr(lang, 'config:confirm.applyResult')), `[${lang}] blackhole: confirmed; apply result shown: "${applied.split('\n').slice(0, 2).join(' | ')}"`);
  await shot(page, `52-confirmed-with-apply-result-${lang}`);
  await ctx.close();
}

async function apiGet(page, path) {
  return page.evaluate(async (p) => {
    const { accessToken } = await (await fetch('/api/v1/auth/refresh', { method: 'POST', credentials: 'include' })).json();
    return (await fetch(p, { headers: { authorization: `Bearer ${accessToken}` } })).json();
  }, path);
}

/** Ground truth straight from the API (through the page's own session) — not from what the UI rendered. */
async function apiRevisions(page) {
  return page.evaluate(async () => {
    const r = await fetch('/api/v1/auth/refresh', { method: 'POST', credentials: 'include' });
    const { accessToken } = await r.json();
    const res = await fetch('/api/v1/config/revisions?limit=10', { headers: { authorization: `Bearer ${accessToken}` } });
    return res.json();
  });
}

const proxy = process.env.VRX_E2E_PROXY_PORT ? await silentProxy(process.env.VRX_E2E_PROXY_PORT, process.env.VRX_E2E_UPSTREAM ?? '127.0.0.1:3101') : null;
if (BLACKHOLE && !proxy) throw new Error('--blackhole needs VRX_E2E_PROXY_PORT and VRX_E2E_UPSTREAM');
const browser = await chromium.launch({ executablePath: process.env.VRX_CHROME, headless: true });
try {
  for (const [i, lang] of LANGS.entries()) await adminPass(browser, lang, i);
  if (SECRET) await secretPass(browser, LANGS[0]);
  if (BLACKHOLE) for (const lang of LANGS) await blackholePass(browser, lang, proxy);
  if (KEYBOARD) await keyboardPass(browser, LANGS[0]);
  if (REVERT) await revertPass(browser, LANGS[0]);
  console.log(`\nE2E PASSED (${results.length} checks)`);
} catch (e) {
  console.error(`\nE2E FAILED: ${e.message}`);
  process.exitCode = 1;
} finally {
  await browser.close();
  await proxy?.close();
}
