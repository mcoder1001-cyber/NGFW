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
const results = [], responses = [], exceptions = [], srvRequests = [];
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
      if (url.pathname === '/api/v1/state/srv6' && response.status === 200) srvRequests.push(message.params.requestId);
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
  const cases = [
    {route:'/routing/mpls?tab=interfaces',need:['loop1460']},
    {route:'/routing/mpls?tab=routes',need:['140016','140030','140040','10.14.40.0/24']},
    {route:'/routing/mpls?tab=tunnels',need:['browser-t1','140050']},
    {route:'/routing/mpls?tab=sr',need:['140100','140101','140102','10.14.60.0/24']},
    {route:'/vpn?tab=srv6',tab:'sids',need:['fd00:e:ff::1','fd00:e:ff::2','fd00:e:ff::a','fd00:e:ff::b','fd00:e:ff::c']},
    {route:'/vpn?tab=srv6',tab:'policies',need:['fd00:e:bb::1','fd00:e:bb::2','fd00:e:ee::1','fd00:e:ee::2']},
    {route:'/vpn?tab=srv6',tab:'steering',need:['10.14.160.0/24','fd00:e:160::/48','loop1461']},
    {route:'/vpn?tab=srv6',tab:'policies',editor:true,need:['fd00:e:ee::1','fd00:e:ee::a','fd00:e:ee::2']}];
  for (const lang of ['en','fa']) {
    await evaluate(`localStorage.setItem('ngfw.ui.settings',JSON.stringify({mode:'light',lang:${JSON.stringify(lang)},persianDigits:${lang==='fa'},dense:true}))`);
    for (const [index,item] of cases.entries()) {
      await send('Page.navigate',{url:base+item.route});
      await until(()=>evaluate(`location.pathname===${JSON.stringify(item.route.split('?')[0])} && document.documentElement.lang===${JSON.stringify(lang)} && !!document.querySelector('main h1,main h2,main h3') && !document.querySelector('main [role=progressbar]')`));
      if(item.tab){
        await until(()=>evaluate(`!!document.getElementById('srv6-tab-${item.tab}')`));
        await evaluate(`document.getElementById('srv6-tab-${item.tab}').click()`);
        await until(()=>evaluate(`document.getElementById('srv6-tab-${item.tab}').getAttribute('aria-selected')==='true'`));
      }
      if(item.editor){
        await until(()=>evaluate(`!!document.querySelector('[data-testid="srv6-policy-fd00:e:bb::1"] button')`));
        await evaluate(`document.querySelector('[data-testid="srv6-policy-fd00:e:bb::1"] button').click()`);
        await until(()=>evaluate(`!!document.querySelector('[role=dialog] [data-testid=srv6-sid-lists]')`));
      }
      // Page headings render before asynchronous real configuration arrives.
      // Wait for the same exact configured values subsequently asserted below.
      await until(()=>evaluate(`(() => { const root=document.querySelector('[role=dialog]')??document.querySelector('main'); const inputs=[...root.querySelectorAll('input,textarea,select')].map(e=>e.value); const text=(root.innerText+' '+inputs.join(' ')).replace(/[۰-۹]/g,c=>'۰۱۲۳۴۵۶۷۸۹'.indexOf(c)).replace(/[٠-٩]/g,c=>'٠١٢٣٤٥٦٧٨٩'.indexOf(c)); return ${JSON.stringify(item.need)}.every(need=>text.includes(need)); })()`));
      await pause(600);
      const state=await evaluate(`({path:location.pathname,lang:document.documentElement.lang,dir:document.documentElement.dir,text:(document.querySelector('[role=dialog]')??document.querySelector('main')).innerText,inputs:[...(document.querySelector('[role=dialog]')??document.querySelector('main')).querySelectorAll('input,textarea,select')].map(e=>e.value)})`);
      assert.equal(state.dir,lang==='fa'?'rtl':'ltr');
      const text=(state.text+' '+state.inputs.join(' ')).replace(/[۰-۹]/g,c=>'۰۱۲۳۴۵۶۷۸۹'.indexOf(c)).replace(/[٠-٩]/g,c=>'٠١٢٣٤٥٦٧٨٩'.indexOf(c));
      assert(!text.includes('Page not found')); assert(!text.includes('There is nothing at')); assert(!text.includes('Unexpected Application Error'));
      assert(!text.includes('BEGIN PRIVATE KEY')); assert(!text.includes(process.env.NGFW_BROWSER_PASSWORD));
      for(const need of item.need) assert(text.includes(need),`configured browser value missing: ${lang} ${item.tab??item.route} ${need}`);
      if(item.tab){
        const body=await send('Network.getResponseBody',{requestId:await until(()=>srvRequests.at(-1))});
        const live=JSON.parse(body.body);
        assert.equal(live.localSids.length,5); assert.equal(live.policies.length,2); assert.equal(live.steering.length,3);
        assert(live.localSids.every(x=>x.configured));
        state.nativeCounts={localSids:5,policies:2,steering:3};
        state.counters=live.localSids.map(x=>({sid:x.sid,goodPackets:x.goodPackets,goodBytes:x.goodBytes,badPackets:x.badPackets,badBytes:x.badBytes}));
      }
      const shot=await send('Page.captureScreenshot',{format:'png',captureBeyondViewport:false});
      const file=`${lang}-${index}-${item.tab??'mpls'}${item.editor?'-editor':''}.png`;
      await writeFile(join(out,file),Buffer.from(shot.data,'base64'));
      delete state.text; delete state.inputs; results.push({...state,...item,screenshot:file});
    }
  }
  assert.equal(exceptions.length, 0, 'browser runtime exceptions');
  assert(!responses.some(r => r.status >= 500), 'unexpected API server failure in browser');
  assert(responses.some(r => r.path === '/api/v1/state/srv6' && r.status === 200));
  await writeFile(join(out, 'browser-results.json'), JSON.stringify({ results, responses, exceptions }, null, 2));
  console.log(`BROWSER_MPLS_SRV6_CONFIGURED_EN_FA=PASS routes=${results.length}`);
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
