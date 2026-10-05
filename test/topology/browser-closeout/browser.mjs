import { spawn } from 'node:child_process';
import { readFile, writeFile, mkdir, open } from 'node:fs/promises';
import { join } from 'node:path';
import assert from 'node:assert/strict';

const out = process.env.NGFW_BROWSER_OUTPUT;
await mkdir(out, { recursive: true });
const profile = process.env.NGFW_BROWSER_PROFILE;
assert(profile, 'owned profile required');
const log = await open(join(out, 'chrome.log'), 'w', 0o600);
// ALLOW: finite private browser acceptance, fixed arguments, no runtime/product execution.
const chrome = spawn(process.env.NGFW_BROWSER_BINARY, ['--headless=new', '--no-sandbox',
  '--disable-gpu', '--disable-background-networking', '--disable-dev-shm-usage', '--no-first-run', '--no-default-browser-check',
  '--remote-debugging-port=0', `--user-data-dir=${profile}`, 'about:blank'],
  { stdio: ['ignore', log.fd, log.fd] });
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
async function until(check, seconds = 30) {
  const end = Date.now() + seconds * 1000;
  while (Date.now() < end) { const value = await check(); if (value) return value; await pause(100); }
  throw new Error('browser condition timeout');
}
let ws, captureFailure;
const results = [], responses = [], exceptions = [], lbRequests = [];
try {
  const port = await until(async () => {
    try { return (await readFile(join(profile, 'DevToolsActivePort'), 'utf8')).split('\n')[0]; }
    catch { return null; }
  });
  const tabs = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
  const tab = tabs.find(t => t.type === 'page'); assert(tab);
  ws = new WebSocket(tab.webSocketDebuggerUrl);
  await new Promise((resolve, reject) => { ws.onopen = resolve; ws.onerror = reject; });
  const pending = new Map(); let id = 0;
  ws.onmessage = event => {
    const message = JSON.parse(event.data);
    if (message.id) {
      const request = pending.get(message.id); if (!request) return;
      pending.delete(message.id); clearTimeout(request.timer);
      if (message.error) request.reject(new Error(`CDP ${request.method} failed`));
      else request.resolve(message.result);
    } else if (message.method === 'Network.responseReceived') {
      const response = message.params.response, url = new URL(response.url);
      if (url.pathname.startsWith('/api/')) responses.push({ path: url.pathname, status: response.status });
      if (url.pathname === '/api/v1/state/lb/vips' && response.status === 200) lbRequests.push(message.params.requestId);
    } else if (message.method === 'Runtime.exceptionThrown') exceptions.push('runtime exception');
  };
  function send(method, params = {}) {
    return new Promise((resolve, reject) => {
      const key = ++id;
      const timer = setTimeout(() => { pending.delete(key); reject(new Error(`CDP ${method} timeout`)); }, 30000);
      pending.set(key, { method, resolve, reject, timer });
      ws.send(JSON.stringify({ id: key, method, params }));
    });
  }
  async function evaluate(expression) {
    const result = await send('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true });
    assert(!result.exceptionDetails, 'page evaluation failed'); return result.result.value;
  }
  captureFailure = async () => {
    const state = await evaluate('({path:location.href,title:document.title,lang:document.documentElement.lang,dir:document.documentElement.dir})');
    const screenshot = await send('Page.captureScreenshot', {format:'png'});
    await writeFile(join(out,'failed-page.png'),Buffer.from(screenshot.data,'base64'));
    await writeFile(join(out,'failed-observations.json'),JSON.stringify({state,responses,exceptions},null,2));
  };
  await send('Page.enable'); await send('Runtime.enable'); await send('Network.enable');
  await send('Emulation.setDeviceMetricsOverride', { width: 1440, height: 1100, deviceScaleFactor: 1, mobile: false });
  const base = 'http://127.0.0.1:15400';
  const restricted = await send('Page.navigate', {url:'http://127.0.0.1:6000/login'});
  assert.equal(restricted.errorText, 'net::ERR_UNSAFE_PORT');
  await writeFile(join(out,'slot10-port-negative.json'),JSON.stringify({error:restricted.errorText}));
  const navigation = await send('Page.navigate', { url: base + '/login' });
  assert(!navigation.errorText, 'login navigation refused');
  await until(() => evaluate('!!document.querySelector("input[type=password]")'));
  const credentials = JSON.stringify({ username: 'admin', password: process.env.NGFW_BROWSER_PASSWORD });
  await evaluate(`(() => { const c=${credentials}; const set=Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set; for(const [selector,value] of [['input[autocomplete=username]',c.username],['input[type=password]',c.password]]) { const e=document.querySelector(selector); set.call(e,value); e.dispatchEvent(new Event('input',{bubbles:true})); } return true; })()`);
  await until(() => evaluate('!document.querySelector("button[type=submit]").disabled'));
  await evaluate('document.querySelector("button[type=submit]").click()');
  await until(() => evaluate('location.pathname!=="/login" && !!document.querySelector("main")'));
  const routes = ['/system/management?tab=tls', '/system/management?tab=users', '/services?tab=lb',
    '/routing/mpls', '/routing/ospf', '/routing/isis-rip', '/routing/wan', '/firewall/nat', '/system/dataplane'];
  for (const lang of ['en', 'fa']) {
    await evaluate(`localStorage.setItem('ngfw.ui.settings',JSON.stringify({mode:'light',lang:${JSON.stringify(lang)},persianDigits:${lang === 'fa'},dense:true}))`);
    for (const [index, route] of routes.entries()) {
      await send('Page.navigate', { url: base + route });
      await until(() => evaluate(`location.pathname===${JSON.stringify(route.split('?')[0])} && document.documentElement.lang===${JSON.stringify(lang)} && !!document.querySelector('main h1,main h2,main h3') && !document.querySelector('main [role=progressbar]')`));
      await pause(600);
      const state = await evaluate(`({path:location.pathname,lang:document.documentElement.lang,dir:document.documentElement.dir,headings:[...document.querySelectorAll('main h1,main h2,main h3')].map(e=>e.textContent),text:document.querySelector('main').innerText})`);
      assert.equal(state.dir, lang === 'fa' ? 'rtl' : 'ltr');
      assert(state.text.length > 20); assert(!state.text.includes('Unexpected Application Error'));
      assert(!state.text.includes('Page not found')); assert(!state.text.includes('There is nothing at'));
      assert(!state.text.includes('BEGIN PRIVATE KEY')); assert(!state.text.includes(process.env.NGFW_BROWSER_PASSWORD));
      if (route === '/services?tab=lb') {
        assert(state.text.includes('browser-web'), 'real configured VIP missing from browser');
        const body = await send('Network.getResponseBody', {requestId:await until(() => lbRequests.at(-1))});
        const vip = JSON.parse(body.body).items.find(item => item.name === 'browser-web');
        assert(vip?.applied, 'real browser LB response lacks applied VIP');
        state.liveVip = {name:vip.name,applied:vip.applied};
      }
      const screenshot = await send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false });
      const file = `${lang}-${index}-${route.split('?')[0].replaceAll('/', '-').slice(1)}.png`;
      await writeFile(join(out, file), Buffer.from(screenshot.data, 'base64'));
      delete state.text; results.push({ ...state, route, screenshot: file });
    }
  }
  assert.equal(exceptions.length, 0, 'browser runtime exceptions');
  assert(!responses.some(r => r.status >= 500), 'unexpected API server failure in browser');
  assert(responses.some(r => r.path === '/api/v1/state/lb/vips' && r.status === 200));
  assert(responses.some(r => r.path === '/api/v1/state/management/tls' && r.status === 200));
  await writeFile(join(out, 'browser-results.json'), JSON.stringify({ results, responses, exceptions }, null, 2));
  console.log(`BROWSER_REAL_API_EN_FA_NAV=PASS routes=${results.length}`);
} catch (error) {
  console.error('BROWSER_ACCEPTANCE_ERROR=' + error.message);
  try { await captureFailure?.(); } catch {}
  throw error;
} finally {
  ws?.close();
  if (chrome.exitCode === null && chrome.signalCode === null) {
    chrome.kill('SIGTERM');
    await Promise.race([new Promise(resolve => chrome.once('exit', resolve)), pause(5000)]);
    if (chrome.exitCode === null && chrome.signalCode === null) { chrome.kill('SIGKILL'); await new Promise(resolve => chrome.once('exit', resolve)); }
  }
  await log.close(); // Parent owns profile removal after its entire child group stops.
}
