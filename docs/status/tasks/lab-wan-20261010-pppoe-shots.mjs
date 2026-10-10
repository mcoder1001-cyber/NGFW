import {createRequire} from 'node:module';
import {spawn} from 'node:child_process';
import {resolve} from 'node:path';
import {createServer} from 'node:net';
const require=createRequire(import.meta.url);
const {chromium}=require('/root/.npm/_npx/e41f203b7505f1fb/node_modules/playwright-core');
let input='';for await(const chunk of process.stdin)input+=chunk;
const {password}=JSON.parse(input);input='';
const root=resolve(''),output=process.env.NGFW_WAN_BROWSER_OUTPUT;
if(!output||!output.startsWith(root+'/docs/status/tasks/lab-wan-20261010-evidence/'))throw new Error('owned screenshot output required');
const webPort='15780',apiPort='12000';
const reservation=createServer();await new Promise((ok,fail)=>reservation.once('error',fail).listen(Number(webPort),'127.0.0.1',ok));await new Promise(ok=>reservation.close(ok));
const web=spawn('node',[root+'/apps/web/node_modules/vite/bin/vite.js','preview','--host','127.0.0.1','--port',webPort,'--strictPort'],{cwd:root+'/apps/web',env:{...process.env,NGFW_HTTP_PORT:apiPort,NGFW_WEB_PORT:webPort},stdio:'ignore'});
let browser;
try{
 for(let i=0;i<100;i++){try{if((await fetch('http://127.0.0.1:'+webPort+'/')).ok)break;}catch{}await new Promise(r=>setTimeout(r,200));}
 browser=await chromium.launch({executablePath:'/tmp/fbr-ux-chrome/chrome-headless-shell-linux64/chrome-headless-shell',headless:true,timeout:15000,args:['--no-sandbox'],env:{...process.env,LD_LIBRARY_PATH:'/tmp/fbr-ux-chrome/libroot/usr/lib/x86_64-linux-gnu'}});
 for(const lang of ['en','fa']){
  const context=await browser.newContext({viewport:{width:1440,height:1000}});
  await context.addInitScript(l=>localStorage.setItem('ngfw.ui.settings',JSON.stringify({lang:l,mode:'light',dense:true,persianDigits:false})),lang);
  const page=await context.newPage();page.setDefaultTimeout(10000);page.setDefaultNavigationTimeout(10000);const errors=[];page.on('pageerror',e=>errors.push(e.message));
  await page.goto('http://127.0.0.1:'+webPort+'/login');
  await page.locator('input[autocomplete="username"]').fill('admin');await page.locator('input[type="password"]').fill(password);
  await page.locator('button[type="submit"]').click();await page.waitForURL(url=>!url.pathname.includes('login'));
  await page.goto('http://127.0.0.1:'+webPort+'/interfaces');
  await page.getByRole('row').filter({has:page.getByText('w20ppp',{exact:true})}).click();
  const table=page.getByRole('table',{name:lang==='en'?'PPPoE session':'نشست PPPoE'});
  await table.getByText('100.64.20.10',{exact:true}).waitFor();
  await table.getByText('100.64.20.1',{exact:true}).waitFor();
  if(errors.length)throw new Error('actual UI errors: '+errors.join('\n'));
  await page.screenshot({path:output+'-'+lang+'.png',fullPage:true});await context.close();
 }
 console.log('REAL_PPPOE_DRAWER_NORMAL_LOGIN_EN_FA_PASS');
}finally{
 if(browser)await browser.close();
 const wait=ms=>Promise.race([new Promise(ok=>web.once('exit',()=>ok(true))),new Promise(ok=>setTimeout(()=>ok(false),ms))]);
 if(web.exitCode===null&&web.signalCode===null){web.kill('SIGTERM');if(!(await wait(5000))){web.kill('SIGKILL');await wait(2000);}}
 if(web.exitCode===null&&web.signalCode===null)throw new Error('owned web process did not exit');
 const check=createServer();await new Promise((ok,fail)=>check.once('error',fail).listen(Number(webPort),'127.0.0.1',ok));await new Promise(ok=>check.close(ok));
 console.log('OWNED_PPPOE_BROWSER_WEB_PORT_CLEANUP_PASS');
}
